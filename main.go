// cmd5-mcp：CMD5 哈希在线解密的极简 MCP stdio 服务器（零依赖，仅 Go 标准库）。
//
// 协议：MCP stdio（换行分隔的 JSON-RPC 2.0），实现 initialize/tools/list/tools/call/ping。
// 配置：env CMD5_EMAIL / CMD5_KEY（cmd5.com 后台获取；改密码后 key 会变）。
// 留空调用时返回友好提示，挂载工具列表不需要凭据。
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	apiURL          = "http://www.cmd5.com/api.ashx"
	serverName      = "cmd5"
	serverVersion   = "1.1.0"
	protocolVersion = "2024-11-05"
)

// cmd5.com 错误码 → 中文提示
var errText = map[string]string{
	"0":    "查询失败（未收录该哈希明文）",
	"-1":   "无效的用户名密码（CMD5_EMAIL/CMD5_KEY 不对；cmd5 改密码后 key 会变）",
	"-2":   "余额不足",
	"-22":  "有效期到期（充值用户 API 自首次使用起 10 天后禁用）",
	"-3":   "查询服务器故障",
	"-4":   "不识别的哈希（不支持 md5(unix)/md5(phpbb3)/md5(wordpress) 等类型）",
	"-7":   "不支持的类型（仅 Md5/Sha1/Sha256/Sha512/mysql/mysql5 等）",
	"-8":   "api 权限被禁止",
	"-9":   "条数超过 100 条",
	"-999": "其它错误",
}

// cmd5Tool 是唯一的工具定义。
var cmd5Tool = map[string]any{
	"name": "cmd5_decrypt",
	"description": "CMD5 在线解密哈希（md5/sha1/sha256/sha512/mysql/mysql5 混合类型自动识别）。" +
		"参数 hashes 支持逗号分隔多条（≤100 条；每条查询都扣费，重复查询重复扣费）。" +
		"不支持 md5(unix)/md5(phpbb3)/md5(wordpress)。" +
		"需在 MCP env 配置 CMD5_EMAIL/CMD5_KEY（cmd5.com 账号）。",
	"inputSchema": map[string]any{
		"type": "object",
		"properties": map[string]any{
			"hashes": map[string]any{"type": "string", "description": "要解密的哈希，多条用英文逗号分隔（≤100 条）"},
		},
		"required": []string{"hashes"},
	},
}

// rpcRequest 是入站 JSON-RPC 消息（只取需要的字段）。
type rpcRequest struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"params"`
}

var out = bufio.NewWriter(os.Stdout)

func send(msg map[string]any) {
	b, _ := json.Marshal(msg)
	out.Write(b)
	out.WriteByte('\n')
	out.Flush()
}

func result(id json.RawMessage, payload any) {
	send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": payload})
}

func rpcError(id json.RawMessage, code int, message string) {
	send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id),
		"error": map[string]any{"code": code, "message": message}})
}

func textResult(id json.RawMessage, text string, isError bool) {
	payload := map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}
	if isError {
		payload["isError"] = true
	}
	result(id, payload)
}

// cmd5Query 调 cmd5.com 在线解密。凭据空调用才校验——挂载工具列表不需要凭据。
func cmd5Query(hashes string) (string, string) {
	email := strings.TrimSpace(os.Getenv("CMD5_EMAIL"))
	key := strings.TrimSpace(os.Getenv("CMD5_KEY"))
	if email == "" || key == "" {
		return "", "未配置 CMD5 凭据：在 MCP 的 env 里填 CMD5_EMAIL 和 CMD5_KEY（cmd5.com 登录后可见）。"
	}
	hashes = strings.TrimSpace(hashes)
	if hashes == "" {
		return "", "hashes 参数为空。"
	}
	if strings.Count(hashes, ",") >= 100 {
		return "", "条数超过 100 条上限（CMD5-ERROR:-9）。"
	}
	qs := url.Values{"email": {email}, "key": {key}, "hash": {hashes}}.Encode()
	client := &http.Client{Timeout: 30 * time.Second} // Transport 默认认 HTTP(S)_PROXY 环境变量
	resp, err := client.Get(apiURL + "?" + qs)
	if err != nil {
		return "", fmt.Sprintf("请求 cmd5.com 失败: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Sprintf("读取 cmd5.com 响应失败: %v", err)
	}
	body := strings.TrimSpace(string(raw))
	if strings.HasPrefix(body, "CMD5-ERROR:") {
		code := strings.TrimSpace(strings.TrimPrefix(body, "CMD5-ERROR:"))
		text, ok := errText[code]
		if !ok {
			text = "未知错误码"
		}
		return "", fmt.Sprintf("%s（%s）", body, text)
	}
	return body, ""
}

func main() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req rpcRequest
		if json.Unmarshal([]byte(line), &req) != nil {
			continue
		}
		if len(req.ID) == 0 || string(req.ID) == "null" {
			continue // 通知（如 notifications/initialized）无需应答
		}
		switch req.Method {
		case "initialize":
			result(req.ID, map[string]any{
				"protocolVersion": protocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": serverName, "version": serverVersion},
			})
		case "ping":
			result(req.ID, map[string]any{})
		case "tools/list":
			result(req.ID, map[string]any{"tools": []map[string]any{cmd5Tool}})
		case "tools/call":
			if req.Params.Name != "cmd5_decrypt" {
				rpcError(req.ID, -32602, fmt.Sprintf("unknown tool: %s", req.Params.Name))
				continue
			}
			hashes, _ := req.Params.Arguments["hashes"].(string)
			body, errText := cmd5Query(hashes)
			if errText != "" {
				textResult(req.ID, errText, true)
			} else {
				textResult(req.ID, body, false)
			}
		default:
			rpcError(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
		}
	}
}
