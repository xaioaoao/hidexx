package cmd

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/shadowsocks/go-shadowsocks2/core"
	"github.com/shadowsocks/go-shadowsocks2/socks"
	"github.com/spf13/cobra"
)

var ssCmd = &cobra.Command{
	Use:   "ss",
	Short: "Run Shadowsocks encrypted proxy server (one port per user)",
	Run:   runSS,
}

func init() {
	ssCmd.Flags().IntP("users", "n", 2, "number of proxy ports")
	ssCmd.Flags().IntP("port", "p", 51801, "starting port")
	ssCmd.Flags().String("http", "51800", "HTTP port for Clash YAML subscription")
	ssCmd.Flags().StringP("method", "m", "AEAD_AES_256_GCM", "encryption method")

	rootCmd.AddCommand(ssCmd)
}

func runSS(cmd *cobra.Command, args []string) {
	numUsers, _ := cmd.Flags().GetInt("users")
	basePort, _ := cmd.Flags().GetInt("port")
	method, _ := cmd.Flags().GetString("method")

	fmt.Println("=== hidexx Shadowsocks server ===")
	fmt.Println()

	localIP := getPublicIP()

	passwords := loadOrGeneratePasswords(numUsers)

	// Shadowrocket 用的 method 名
	ssMethod := "aes-256-gcm"

	for i := 0; i < numUsers; i++ {
		port := basePort + i
		userID := i + 1
		pw := passwords[i]
		go startSS(port, userID, method, pw)

		// 生成 ss:// 一键导入链接
		raw := fmt.Sprintf("%s:%s", ssMethod, pw)
		encoded := base64.StdEncoding.EncodeToString([]byte(raw))
		ssURL := fmt.Sprintf("ss://%s@%s:%d#hidexx-user%d", encoded, localIP, port, userID)

		fmt.Printf("  user %d:\n", userID)
		fmt.Printf("    one-click URL: %s\n", ssURL)
		fmt.Println()
	}

	// 启动 HTTP 服务，提供 Clash YAML 订阅
	httpPort, _ := cmd.Flags().GetString("http")
	publicIP := getPublicIP()

	mux := http.NewServeMux()
	for i := 0; i < numUsers; i++ {
		idx := i
		port := basePort + idx
		pw := passwords[idx]
		userID := idx + 1
		path := fmt.Sprintf("/%d/clash.yaml", userID)

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			yaml := fmt.Sprintf(`mixed-port: 7890
allow-lan: false
mode: rule
log-level: info

proxies:
  - name: hidexx-user%d
    type: ss
    server: %s
    port: %d
    cipher: aes-256-gcm
    password: "%s"

proxy-groups:
  - name: PROXY
    type: select
    proxies:
      - hidexx-user%d

rules:
  - GEOIP,CN,DIRECT
  - MATCH,PROXY
`, userID, publicIP, port, pw, userID)

			w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
			w.Write([]byte(yaml))
		})
	}

	go func() {
		addr := "0.0.0.0:" + httpPort
		log.Printf("Clash subscription HTTP server on %s", addr)
		http.ListenAndServe(addr, mux)
	}()

	fmt.Println("Clash subscription URLs (for Android Clash):")
	for i := 0; i < numUsers; i++ {
		fmt.Printf("  user %d: http://%s:%s/%d/clash.yaml\n", i+1, publicIP, httpPort, i+1)
	}
	fmt.Println()
	fmt.Println("server running...")

	select {}
}

func startSS(port, userID int, method, password string) {
	ciph, err := core.PickCipher(method, nil, password)
	if err != nil {
		log.Fatalf("[user %d] cipher error: %v", userID, err)
	}

	addr := "0.0.0.0:" + strconv.Itoa(port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[user %d] listen %s: %v", userID, addr, err)
	}
	log.Printf("[user %d] Shadowsocks (%s) listening on %s", userID, method, addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("[user %d] accept: %v", userID, err)
			continue
		}
		go func() {
			defer conn.Close()
			ssConn := ciph.StreamConn(conn)
			tgt, err := socks.ReadAddr(ssConn)
			if err != nil {
				return
			}
			remote, err := net.Dial("tcp", tgt.String())
			if err != nil {
				return
			}
			defer remote.Close()

			done := make(chan struct{}, 2)
			go func() { relay(remote, ssConn); done <- struct{}{} }()
			go func() { relay(ssConn, remote); done <- struct{}{} }()
			<-done
		}()
	}
}

func getPublicIP() string {
	resp, err := http.Get("https://api.ipify.org")
	if err != nil {
		return getOutboundIP()
	}
	defer resp.Body.Close()
	b := make([]byte, 64)
	n, _ := resp.Body.Read(b)
	ip := string(b[:n])
	if net.ParseIP(ip) != nil {
		return ip
	}
	return getOutboundIP()
}

const passwordFile = "/etc/hidexx/passwords.json"

func loadOrGeneratePasswords(n int) []string {
	// try load from file
	if data, err := os.ReadFile(passwordFile); err == nil {
		var pws []string
		if json.Unmarshal(data, &pws) == nil && len(pws) >= n {
			log.Printf("loaded %d passwords from %s", n, passwordFile)
			return pws[:n]
		}
	}

	// generate new
	pws := make([]string, n)
	for i := range pws {
		b := make([]byte, 32)
		rand.Read(b)
		pws[i] = base64.StdEncoding.EncodeToString(b)
	}

	// save to file
	os.MkdirAll(filepath.Dir(passwordFile), 0755)
	if data, err := json.Marshal(pws); err == nil {
		os.WriteFile(passwordFile, data, 0600)
		log.Printf("saved %d passwords to %s", n, passwordFile)
	}

	return pws
}
