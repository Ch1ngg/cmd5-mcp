package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"fmt"
	"strings"
	"testing"
)

func callReq(method string) rpcRequest {
	return rpcRequest{ID: json.RawMessage("1"), Method: method}
}

func callTool(name string, args map[string]any) rpcRequest {
	r := callReq("tools/call")
	r.Params.Name = name
	r.Params.Arguments = args
	return r
}

// 协议面：initialize/tools/list/ping 的应答形状。
func TestHandleRPCProtocol(t *testing.T) {
	resp := handleRPC(callReq("initialize"))
	res := resp["result"].(map[string]any)
	if res["protocolVersion"] != protocolVersion {
		t.Fatalf("protocolVersion: %v", res["protocolVersion"])
	}
	info := res["serverInfo"].(map[string]any)
	if info["name"] != "cmd5" {
		t.Fatalf("serverInfo: %v", info)
	}

	resp = handleRPC(callReq("tools/list"))
	tools := resp["result"].(map[string]any)["tools"].([]map[string]any)
	if len(tools) != 1 || tools[0]["name"] != "cmd5_decrypt" {
		t.Fatalf("tools: %v", tools)
	}

	if resp = handleRPC(callReq("ping")); resp["result"] == nil {
		t.Fatal("ping 应有空 result")
	}

	resp = handleRPC(callReq("resources/list"))
	if resp["error"] == nil {
		t.Fatal("未知方法应回 error")
	}
}

// 凭据门：未配置凭据时调用返回友好中文提示（isError），且提示里不许泄露空凭据细节。
func TestCallWithoutCredentials(t *testing.T) {
	t.Setenv("CMD5_EMAIL", "")
	t.Setenv("CMD5_KEY", "")
	resp := handleRPC(callTool("cmd5_decrypt", map[string]any{"hashes": "5d41402abc4b2a76b9719d911017c592"}))
	res := resp["result"].(map[string]any)
	if res["isError"] != true {
		t.Fatal("无凭据调用应 isError")
	}
	text := res["content"].([]map[string]any)[0]["text"].(string)
	if !strings.Contains(text, "CMD5_EMAIL") {
		t.Fatalf("提示应指引配置凭据: %q", text)
	}
}

// 入参护栏：空 hashes 与 >100 条都在本地拦下（不烧 API 余额）。
func TestCallInputGuards(t *testing.T) {
	t.Setenv("CMD5_EMAIL", "a@b.c")
	t.Setenv("CMD5_KEY", "k")
	text := func(args map[string]any) string {
		resp := handleRPC(callTool("cmd5_decrypt", args))
		res := resp["result"].(map[string]any)
		if res["isError"] != true {
			t.Fatalf("应 isError: %v", res)
		}
		return res["content"].([]map[string]any)[0]["text"].(string)
	}
	if got := text(map[string]any{"hashes": "  "}); !strings.Contains(got, "为空") {
		t.Fatalf("空 hashes: %q", got)
	}
	many := strings.Repeat("abcdef,", 100) + "ff" // 101 条
	if got := text(map[string]any{"hashes": many}); !strings.Contains(got, "100") {
		t.Fatalf("超 100 条: %q", got)
	}
}

// 错误码映射：本地桩返回 CMD5-ERROR:-2 → 中文「余额不足」；成功体原样透传。
func TestQueryErrorMappingAndSuccess(t *testing.T) {
	t.Setenv("CMD5_EMAIL", "a@b.c")
	t.Setenv("CMD5_KEY", "k")
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("hash") == "ok" {
			fmt.Fprint(w, "明文结果")
			return
		}
		fmt.Fprint(w, "CMD5-ERROR:-2")
	}))
	defer stub.Close()
	old := apiURL
	apiURL = stub.URL
	defer func() { apiURL = old }()

	_, errText := cmd5Query("bad")
	if !strings.Contains(errText, "余额不足") {
		t.Fatalf("错误码未映射: %q", errText)
	}
	body, errText := cmd5Query("ok")
	if errText != "" || body != "明文结果" {
		t.Fatalf("成功路径: %q %q", body, errText)
	}
}

// HTTP 认证：设了 CMD5_HTTP_TOKEN 后，无 token/错 token 401，对的过。
func TestHTTPBearerAuth(t *testing.T) {
	t.Setenv("CMD5_HTTP_TOKEN", "tok123")
	ok := func(auth string) bool {
		req := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		return authOK(req)
	}
	if ok("") || ok("Bearer wrong") {
		t.Fatal("无/错 token 应拒")
	}
	if !ok("Bearer tok123") {
		t.Fatal("正确 token 应过")
	}
}

// HTTP 形态：/mcp 完整请求-响应；未设 token 时裸奔放行（本机环回场景）。
func TestHTTPStreamableEndpoint(t *testing.T) {
	t.Setenv("CMD5_HTTP_TOKEN", "")
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"tools/list"}`))
	w := httptest.NewRecorder()
	handleStreamable(w, req)
	if w.Code != 200 {
		t.Fatalf("status: %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["id"].(float64) != 7 || resp["result"] == nil {
		t.Fatalf("应答形状不对: %v", resp)
	}
	// GET 不允许
	req = httptest.NewRequest("GET", "/mcp", nil)
	w = httptest.NewRecorder()
	handleStreamable(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /mcp 应 405，得 %d", w.Code)
	}
}
