package cmd

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net"
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
	ssCmd.Flags().StringP("method", "m", "AEAD_AES_256_GCM", "encryption method")

	rootCmd.AddCommand(ssCmd)
}

func runSS(cmd *cobra.Command, args []string) {
	numUsers, _ := cmd.Flags().GetInt("users")
	basePort, _ := cmd.Flags().GetInt("port")
	method, _ := cmd.Flags().GetString("method")

	fmt.Println("=== hidexx Shadowsocks server ===")
	fmt.Println()

	localIP := getOutboundIP()

	passwords := generatePasswords(numUsers)

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

	fmt.Println("usage: copy the URL above, open in Safari/browser on phone")
	fmt.Println("       Shadowrocket will auto-import the config")
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

func generatePasswords(n int) []string {
	pws := make([]string, n)
	for i := range pws {
		b := make([]byte, 32)
		rand.Read(b)
		pws[i] = base64.StdEncoding.EncodeToString(b)
	}
	return pws
}
