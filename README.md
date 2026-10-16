# cmd5-mcp

CMD5（cmd5.com）哈希在线解密的极简 MCP stdio 服务器。零依赖 Go 单二进制——不需要 Python、不需要 Node，下载即用。

English: a zero-dependency MCP stdio server that decrypts hashes (md5/sha1/sha256/sha512/mysql/mysql5) via the cmd5.com online API.

## 工具

**`cmd5_decrypt`** — 在线解密哈希，混合类型自动识别。

- 参数 `hashes`：逗号分隔多条，≤100 条
- 每条查询都扣费，重复查询重复扣费
- 不支持 md5(unix)/md5(phpbb3)/md5(wordpress)

## 配置

| 环境变量 | 说明 |
|---|---|
| `CMD5_EMAIL` | cmd5.com 账号邮箱 |
| `CMD5_KEY` | cmd5.com API Key（改密码后 key 会变） |
| `SOCKS5_PROXY` | 可选，`socks5://[user:pass@]host:port`——查询流量经 socks5 出口（隧道/VPS），不落本机 IP |
| `HTTP_PROXY` / `HTTPS_PROXY` | 可选，HTTP 代理 |
| `CMD5_HTTP_TOKEN` | HTTP 模式下的 Bearer 认证 token（网络面暴露时强烈建议设置） |

凭据留空也能挂载（工具列表可见），调用时才提示配置。

## 传输模式

**stdio（默认）**：MCP 客户端以子进程拉起，常规用法。

```json
{
  "mcpServers": {
    "cmd5": {
      "command": "/path/to/cmd5-mcp",
      "env": {
        "CMD5_EMAIL": "you@example.com",
        "CMD5_KEY": "your-api-key"
      }
    }
  }
}
```

**HTTP**：`cmd5-mcp -http :9100` 起 HTTP 服务，同时提供两种形态——

- `POST /mcp`：streamable HTTP（请求即响应）
- `GET /sse` + `POST /message?sessionId=…`：legacy SSE 长连

设了 `CMD5_HTTP_TOKEN` 后两种形态都要求 `Authorization: Bearer <token>`。

```bash
CMD5_HTTP_TOKEN=$(openssl rand -hex 24) cmd5-mcp -http 127.0.0.1:9100
```

MCP 客户端配置（HTTP 形态）：

```json
{
  "mcpServers": {
    "cmd5": {
      "url": "http://127.0.0.1:9100/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

## 安装

从 [Releases](https://github.com/Ch1ngg/cmd5-mcp/releases) 下载对应平台的单二进制（linux/darwin/windows × amd64/arm64），或：

```bash
go install github.com/Ch1ngg/cmd5-mcp@latest
```

## License

Apache-2.0
