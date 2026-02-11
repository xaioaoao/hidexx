package cmd

import (
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// getPublicIP returns the server's public IP via external API.
// Falls back to getLocalIP on failure.
func getPublicIP() string {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		return getLocalIP()
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return getLocalIP()
	}
	ip := strings.TrimSpace(string(b))
	if net.ParseIP(ip) != nil && !net.ParseIP(ip).IsLoopback() && !net.ParseIP(ip).IsPrivate() {
		return ip
	}
	return getLocalIP()
}

// getLocalIP returns the first non-loopback IPv4 address.
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			return ipNet.IP.String()
		}
	}
	return "127.0.0.1"
}
