package cmd

import (
	"io"
	"net"
	"sync"
	"time"
)

const relayIdleTimeout = 60 * time.Second

// bidirectionalRelay copies data between two connections with idle timeout.
// When either direction fails or times out, both connections are closed.
func bidirectionalRelay(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	closeOnce := sync.Once{}
	closeAll := func() {
		closeOnce.Do(func() {
			a.Close()
			b.Close()
		})
	}

	go func() {
		defer wg.Done()
		relayWithTimeout(a, b)
		closeAll()
	}()
	go func() {
		defer wg.Done()
		relayWithTimeout(b, a)
		closeAll()
	}()

	wg.Wait()
}

// relayWithTimeout copies from src to dst, resetting a deadline on each successful read.
func relayWithTimeout(dst, src net.Conn) {
	buf := make([]byte, 32*1024)
	for {
		src.SetReadDeadline(time.Now().Add(relayIdleTimeout))
		n, err := src.Read(buf)
		if n > 0 {
			dst.SetWriteDeadline(time.Now().Add(relayIdleTimeout))
			if _, wErr := dst.Write(buf[:n]); wErr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// relayConn is a helper for relaying between a net.Conn and an io.ReadWriter (e.g. cipher stream).
func bidirectionalRelayRW(conn net.Conn, rw io.ReadWriter, remoteConn net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	closeOnce := sync.Once{}
	closeAll := func() {
		closeOnce.Do(func() {
			conn.Close()
			remoteConn.Close()
		})
	}

	go func() {
		defer wg.Done()
		relayRWWithTimeout(remoteConn, rw, conn) // remote -> cipher -> client
		closeAll()
	}()
	go func() {
		defer wg.Done()
		relayRWWithTimeout2(rw, remoteConn, conn) // client -> cipher -> remote
		closeAll()
	}()

	wg.Wait()
}

// relayRWWithTimeout: read from net.Conn (src), write to io.Writer (dst), with timeout on srcConn.
func relayRWWithTimeout(srcConn net.Conn, dst io.Writer, deadlineConn net.Conn) {
	buf := make([]byte, 32*1024)
	for {
		srcConn.SetReadDeadline(time.Now().Add(relayIdleTimeout))
		n, err := srcConn.Read(buf)
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

// relayRWWithTimeout2: read from io.Reader (src), write to net.Conn (dst), with timeout on deadlineConn.
func relayRWWithTimeout2(src io.Reader, dstConn net.Conn, deadlineConn net.Conn) {
	buf := make([]byte, 32*1024)
	for {
		deadlineConn.SetReadDeadline(time.Now().Add(relayIdleTimeout))
		n, err := src.Read(buf)
		if n > 0 {
			dstConn.SetWriteDeadline(time.Now().Add(relayIdleTimeout))
			if _, wErr := dstConn.Write(buf[:n]); wErr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}
