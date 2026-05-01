package main

import (
	"net"
	"net/url"
	"strings"
)

// corsAllowDevLAN allows local/LAN browser origins when using credentials (Fiber lowercases Origin).
func corsAllowDevLAN(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return false
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "localhost" || host == "127.0.0.1" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
