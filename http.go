// HTTP 传输：POST /mcp（streamable HTTP，请求即响应）+ legacy SSE（GET /sse 长连接 +
// POST /message?sessionId=）。两形态共用 handleRPC。
// 认证：env CMD5_HTTP_TOKEN 非空时要求 Authorization: Bearer <token>（HTTP 暴露在网络面，
// 裸奔等于把付费 API 借给别人刷）。
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
)

// sseSession 是一个 legacy SSE 会话：长连 + 该会话的应答通道。
type sseSession struct {
	ch chan map[string]any
}

var (
	sseMu       sync.Mutex
	sseSessions = map[string]*sseSession{}
)

// authOK 校验 Bearer token；CMD5_HTTP_TOKEN 为空时放行（本机/容器环回场景）。
func authOK(r *http.Request) bool {
	tok := strings.TrimSpace(envOr("CMD5_HTTP_TOKEN", ""))
	if tok == "" {
		return true
	}
	return r.Header.Get("Authorization") == "Bearer "+tok
}

func serveHTTP(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", handleStreamable) // streamable HTTP
	mux.HandleFunc("/sse", handleSSE)        // legacy SSE：长连
	mux.HandleFunc("/message", handleSSEMessage)
	log.Printf("[cmd5-mcp] HTTP 传输监听 %s（POST /mcp + GET /sse）；认证：%s",
		addr, map[bool]string{true: "Bearer（CMD5_HTTP_TOKEN）", false: "未设置 CMD5_HTTP_TOKEN，裸奔"}[envOr("CMD5_HTTP_TOKEN", "") != ""])
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[cmd5-mcp] HTTP 服务退出: %v", err)
	}
}

// handleStreamable：POST JSON-RPC → JSON 应答（无 SSE 流，同步请求/响应足够本工具）。
func handleStreamable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !authOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.ID) == 0 || string(req.ID) == "null" {
		w.WriteHeader(http.StatusAccepted) // 通知
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(handleRPC(req))
}

// handleSSE：legacy SSE 长连——建立会话，先发 endpoint 事件，应答经 message 事件推送。
func handleSSE(w http.ResponseWriter, r *http.Request) {
	if !authOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	sid := newSessionID()
	sess := &sseSession{ch: make(chan map[string]any, 16)}
	sseMu.Lock()
	sseSessions[sid] = sess
	sseMu.Unlock()
	defer func() {
		sseMu.Lock()
		delete(sseSessions, sid)
		sseMu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	fmt.Fprintf(w, "event: endpoint\ndata: /message?sessionId=%s\n\n", sid)
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-sess.ch:
			b, _ := json.Marshal(msg)
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", b)
			flusher.Flush()
		}
	}
}

// handleSSEMessage：legacy SSE 的写入端——收 JSON-RPC，应答推进对应会话的 SSE 流。
func handleSSEMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !authOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	sid := r.URL.Query().Get("sessionId")
	sseMu.Lock()
	sess, ok := sseSessions[sid]
	sseMu.Unlock()
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusAccepted) // 先回 202，应答走 SSE 流
	if len(req.ID) == 0 || string(req.ID) == "null" {
		return // 通知
	}
	sess.ch <- handleRPC(req)
}

func newSessionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
