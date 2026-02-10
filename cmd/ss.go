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

	for i := 0; i < numUsers; i++ {
		port := basePort + i
		userID := i + 1
		pw := passwords[i]
		go startSS(port, userID, method, pw)
		fmt.Printf("  user %d:\n", userID)
		fmt.Printf("    address:  %s\n", localIP)
		fmt.Printf("    port:     %d\n", port)
		fmt.Printf("    method:   %s\n", method)
		fmt.Printf("    password: %s\n", pw)
		fmt.Println()
	}

	fmt.Println("Shadowrocket config:")
	fmt.Println("  Type: Shadowsocks")
	fmt.Println("  Algorithm: aes-256-gcm")
	fmt.Println("  Address/Port/Password: see above")
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
