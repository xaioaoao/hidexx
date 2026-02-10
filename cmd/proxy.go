package cmd

import (
	"fmt"
	"log"
	"net"
	"strconv"

	"github.com/spf13/cobra"
)

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Run SOCKS5 proxy server (one port per user)",
	Run:   runProxy,
}

func init() {
	proxyCmd.Flags().IntP("users", "n", 2, "number of proxy ports")
	proxyCmd.Flags().IntP("port", "p", 51801, "starting port (user1=port, user2=port+1, ...)")

	rootCmd.AddCommand(proxyCmd)
}

func runProxy(cmd *cobra.Command, args []string) {
	numUsers, _ := cmd.Flags().GetInt("users")
	basePort, _ := cmd.Flags().GetInt("port")

	fmt.Println("=== hidexx SOCKS5 proxy server ===")
	fmt.Println()

	localIP := getOutboundIP()

	for i := 0; i < numUsers; i++ {
		port := basePort + i
		userID := i + 1
		go startSOCKS5(port, userID)
		fmt.Printf("  user %d: %s:%d\n", userID, localIP, port)
	}

	fmt.Println()
	fmt.Println("Shadowrocket config:")
	fmt.Println("  Type: SOCKS5")
	fmt.Printf("  Address: %s\n", localIP)
	fmt.Println("  Port: (see above)")
	fmt.Println()
	fmt.Println("proxy server running...")

	// block forever
	select {}
}

func startSOCKS5(port, userID int) {
	addr := "0.0.0.0:" + strconv.Itoa(port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[user %d] listen %s: %v", userID, addr, err)
	}
	log.Printf("[user %d] SOCKS5 listening on %s", userID, addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("[user %d] accept: %v", userID, err)
			continue
		}
		go handleSOCKS5(conn, userID)
	}
}

func handleSOCKS5(conn net.Conn, userID int) {
	defer conn.Close()

	// SOCKS5 handshake
	buf := make([]byte, 256)

	// 1. greeting
	n, err := conn.Read(buf)
	if err != nil || n < 2 || buf[0] != 0x05 {
		return
	}
	// no auth required
	conn.Write([]byte{0x05, 0x00})

	// 2. request
	n, err = conn.Read(buf)
	if err != nil || n < 7 || buf[0] != 0x05 || buf[1] != 0x01 {
		conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	// parse target address
	var targetAddr string
	switch buf[3] {
	case 0x01: // IPv4
		if n < 10 {
			return
		}
		targetAddr = fmt.Sprintf("%d.%d.%d.%d:%d", buf[4], buf[5], buf[6], buf[7], int(buf[8])<<8|int(buf[9]))
	case 0x03: // domain
		domainLen := int(buf[4])
		if n < 5+domainLen+2 {
			return
		}
		domain := string(buf[5 : 5+domainLen])
		port := int(buf[5+domainLen])<<8 | int(buf[5+domainLen+1])
		targetAddr = fmt.Sprintf("%s:%d", domain, port)
	case 0x04: // IPv6
		if n < 22 {
			return
		}
		ip := net.IP(buf[4:20])
		port := int(buf[20])<<8 | int(buf[21])
		targetAddr = fmt.Sprintf("[%s]:%d", ip.String(), port)
	default:
		conn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	// connect to target
	remote, err := net.Dial("tcp", targetAddr)
	if err != nil {
		conn.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer remote.Close()

	// success reply
	conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	// bidirectional relay
	done := make(chan struct{}, 2)
	go func() { relay(remote, conn); done <- struct{}{} }()
	go func() { relay(conn, remote); done <- struct{}{} }()
	<-done
}

func relay(dst, src net.Conn) {
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, wErr := dst.Write(buf[:n]); wErr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return getLocalIP()
	}
	defer conn.Close()
	addr := conn.LocalAddr().(*net.UDPAddr)
	return addr.IP.String()
}

func init() {
	// ensure getLocalIP is available from serve.go
	_ = getLocalIP
}
