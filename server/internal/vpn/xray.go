package vpn

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// BuildVMessURL builds vmess:// URL.
// JSON payload: {"v":"2","ps":"","add":"host","port":"443","id":"uuid","aid":"0","scy":"auto","net":"tcp","type":"none","host":"","path":"","tls":"tls","sni":"","alpn":""}
func BuildVMessURL(uuid, host, port, remark string) string {
	if port == "" {
		port = "443"
	}
	if remark == "" {
		remark = "messenger"
	}
	payload := map[string]interface{}{
		"v":   "2",
		"ps":  remark,
		"add": host,
		"port": port,
		"id":  uuid,
		"aid": "0",
		"scy": "auto",
		"net": "tcp",
		"type": "none",
		"host": "",
		"path": "",
		"tls":  "tls",
		"sni":  host,
		"alpn": "h2,http/1.1",
	}
	b, _ := json.Marshal(payload)
	return "vmess://" + base64.StdEncoding.EncodeToString(b)
}

// BuildVLESSURL builds vless://uuid@host:port?params URL (VLESS + XTLS Vision, recommended).
func BuildVLESSURL(uuid, host, port, remark string) string {
	if port == "" {
		port = "443"
	}
	params := url.Values{}
	params.Set("type", "tcp")
	params.Set("security", "xtls")
	params.Set("flow", "xtls-rprx-vision")
	params.Set("sni", host)
	params.Set("alpn", "h2,http/1.1")
	if remark != "" {
		params.Set("name", url.QueryEscape(remark))
	}
	return fmt.Sprintf("vless://%s@%s:%s?%s", uuid, host, port, params.Encode())
}

// BuildTrojanURL builds trojan://password@host:port?params URL.
func BuildTrojanURL(password, host, port, remark string) string {
	if port == "" {
		port = "443"
	}
	params := url.Values{}
	params.Set("security", "tls")
	params.Set("sni", host)
	params.Set("alpn", "h2,http/1.1")
	if remark != "" {
		params.Set("name", url.QueryEscape(remark))
	}
	return fmt.Sprintf("trojan://%s@%s:%s?%s", url.PathEscape(password), host, port, params.Encode())
}

// parseHostPort splits "host:port" and returns host, port. Default port 443.
func parseHostPort(endpoint string) (host, port string) {
	if endpoint == "" {
		return "", "443"
	}
	// Handle [::1]:443
	if strings.HasPrefix(endpoint, "[") {
		if idx := strings.Index(endpoint, "]:"); idx > 0 {
			return endpoint[:idx+1], endpoint[idx+2:]
		}
		return endpoint, "443"
	}
	if idx := strings.LastIndex(endpoint, ":"); idx > 0 {
		return endpoint[:idx], endpoint[idx+1:]
	}
	return endpoint, "443"
}
