package cmd

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"time"

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

	ip := getPublicIP()

	for i := 0; i < numUsers; i++ {
		port := basePort + i
		userID := i + 1
		go startSOCKS5(port, userID)
		fmt.Printf("  user %d: %s:%d\n", userID, ip, port)
	}

	fmt.Println()
	fmt.Println("proxy server running...")

	select {}
}

const socks5HandshakeTimeout = 10 * time.Second

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
		go handleSOCKS5(conn)
	}
}

func handleSOCKS5(conn net.Conn) {
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(socks5HandshakeTimeout))

	buf := make([]byte, 256)

	// 1. greeting
	n, err := conn.Read(buf)
	if err != nil || n < 2 || buf[0] != 0x05 {
		return
	}
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
		if domainLen == 0 || domainLen > 253 || n < 5+domainLen+2 {
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
	remote, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		conn.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	// success reply — clear handshake deadline
	conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	conn.SetDeadline(time.Time{})

	// relay with idle timeout, both directions auto-close
	bidirectionalRelay(conn, remote)
}
