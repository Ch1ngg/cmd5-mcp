// 出口代理：只认 CMD5_PROXY（刻意不复用 HTTP_PROXY 等系统代理变量，避免误走系统代理）。
// 支持 socks5://（红队场景：查询流量经隧道/VPS 出口，不落本机 IP）与 http(s)://，留空直连。
package main

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// httpClient 构造出口 client：CMD5_PROXY 为 socks5:// 走 socks5，为 http(s):// 走 HTTP 代理，空则直连。
func httpClient() *http.Client {
	tr := &http.Transport{}
	raw := strings.TrimSpace(os.Getenv("CMD5_PROXY"))
	if raw == "" {
		return &http.Client{Transport: tr, Timeout: 30 * time.Second}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return &http.Client{Transport: tr, Timeout: 30 * time.Second}
	}
	switch strings.ToLower(u.Scheme) {
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if u.User != nil {
			pw, _ := u.User.Password()
			auth = &proxy.Auth{User: u.User.Username(), Password: pw}
		}
		if dialer, err := proxy.SOCKS5("tcp", u.Host, auth, &net.Dialer{Timeout: 15 * time.Second}); err == nil {
			tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			}
		}
	default:
		tr.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Transport: tr, Timeout: 30 * time.Second}
}
