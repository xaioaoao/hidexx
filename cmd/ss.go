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
	"time"

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

	publicIP := getPublicIP()
	passwords := loadOrGeneratePasswords(numUsers)

	ssMethod := "aes-256-gcm"

	for i := 0; i < numUsers; i++ {
		port := basePort + i
		userID := i + 1
		pw := passwords[i]
		go startSS(port, userID, method, pw)

		raw := fmt.Sprintf("%s:%s", ssMethod, pw)
		encoded := base64.StdEncoding.EncodeToString([]byte(raw))
		ssURL := fmt.Sprintf("ss://%s@%s:%d#hidexx-user%d", encoded, publicIP, port, userID)

		fmt.Printf("  user %d:\n", userID)
		fmt.Printf("    one-click URL: %s\n", ssURL)
		fmt.Println()
	}

	// Clash YAML HTTP 服务
	httpPort, _ := cmd.Flags().GetString("http")

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
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Fatalf("Clash HTTP server failed: %v", err)
		}
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
		go handleSS(conn, ciph)
	}
}

func handleSS(conn net.Conn, ciph core.Cipher) {
	defer conn.Close()

	ssConn := ciph.StreamConn(conn)
	tgt, err := socks.ReadAddr(ssConn)
	if err != nil {
		return
	}

	remote, err := net.DialTimeout("tcp", tgt.String(), 10*time.Second)
	if err != nil {
		return
	}
	defer remote.Close()

	bidirectionalRelayRW(conn, ssConn, remote)
}

// --- password persistence ---

const passwordFile = "/etc/hidexx/passwords.json"

func loadOrGeneratePasswords(n int) []string {
	// 尝试从文件加载
	if data, err := os.ReadFile(passwordFile); err == nil {
		var existing []string
		if json.Unmarshal(data, &existing) == nil {
			if len(existing) >= n {
				log.Printf("loaded %d passwords from %s", n, passwordFile)
				return existing[:n]
			}
			// 已有密码不够，保留已有的，追加新的
			log.Printf("loaded %d passwords, generating %d more", len(existing), n-len(existing))
			for len(existing) < n {
				existing = append(existing, generatePassword())
			}
			savePasswords(existing)
			return existing
		}
	}

	// 全新生成
	pws := make([]string, n)
	for i := range pws {
		pws[i] = generatePassword()
	}
	savePasswords(pws)
	return pws
}

func generatePassword() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("rand.Read failed: %v", err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func savePasswords(pws []string) {
	os.MkdirAll(filepath.Dir(passwordFile), 0755)
	data, err := json.Marshal(pws)
	if err != nil {
		log.Printf("marshal passwords failed: %v", err)
		return
	}
	if err := os.WriteFile(passwordFile, data, 0600); err != nil {
		log.Printf("save passwords failed: %v", err)
		return
	}
	log.Printf("saved %d passwords to %s", len(pws), passwordFile)
}
