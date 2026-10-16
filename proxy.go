// 出口代理：默认认 HTTP_PROXY/HTTPS_PROXY；SOCKS5_PROXY（或 ALL_PROXY 为 socks5:// 时）
// 走 socks5（红队场景：查询流量经隧道/VPS 出口，不落本机 IP）。
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

// httpClient 构造出口 client：socks5 优先，其次 HTTP(S)_PROXY，都无则直连。
func httpClient() *http.Client {
	tr := &http.Transport{Proxy: http.ProxyFromEnvironment}
	raw := envOr("SOCKS5_PROXY", envOr("ALL_PROXY", ""))
	if strings.HasPrefix(raw, "socks5://") {
		u, err := url.Parse(raw)
		if err == nil {
			var auth *proxy.Auth
			if u.User != nil {
				pw, _ := u.User.Password()
				auth = &proxy.Auth{User: u.User.Username(), Password: pw}
			}
			if dialer, err := proxy.SOCKS5("tcp", u.Host, auth, &net.Dialer{Timeout: 15 * time.Second}); err == nil {
				tr.Proxy = nil
				tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
					return dialer.Dial(network, addr)
				}
			}
		}
	}
	return &http.Client{Transport: tr, Timeout: 30 * time.Second}
}
