package cmd

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/liao/hidexx/client"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Auto-renew daily trial and serve subscription via HTTP (configure once in Shadowrocket)",
	Run:   runServe,
}

func init() {
	serveCmd.Flags().StringP("port", "p", "9191", "HTTP server listen port")
	serveCmd.Flags().String("line", "1", `line_id: "1" for 王者套餐, "11" for 青铜套餐`)

	rootCmd.AddCommand(serveCmd)
}

type subStore struct {
	mu   sync.RWMutex
	yaml []byte
}

func (s *subStore) Set(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.yaml = data
}

func (s *subStore) Get() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.yaml
}

func runServe(cmd *cobra.Command, args []string) {
	port, _ := cmd.Flags().GetString("port")
	lineID, _ := cmd.Flags().GetString("line")

	store := &subStore{}

	// 启动时立即执行一次
	if err := refreshSubscription(store, lineID); err != nil {
		log.Printf("initial refresh failed: %v", err)
		log.Println("will retry in 1 hour...")
	}

	// 后台定时刷新
	go func() {
		// 首次失败则 1 小时后重试，之后每 20 小时刷新（留余量，避免正好 24 小时过期）
		retryInterval := 1 * time.Hour
		normalInterval := 20 * time.Hour

		interval := normalInterval
		if store.Get() == nil {
			interval = retryInterval
		}

		for {
			time.Sleep(interval)
			if err := refreshSubscription(store, lineID); err != nil {
				log.Printf("refresh failed: %v", err)
				interval = retryInterval
			} else {
				interval = normalInterval
			}
		}
	}()

	// HTTP 服务
	mux := http.NewServeMux()
	mux.HandleFunc("/sub.yaml", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[access] %s %s from %s - UA: %s", r.Method, r.URL.Path, r.RemoteAddr, r.UserAgent())
		data := store.Get()
		if data == nil {
			http.Error(w, "subscription not ready yet, try again later", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=hidexx.yaml")
		w.Write(data)
		log.Printf("[access] served %d bytes to %s", len(data), r.RemoteAddr)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[access] %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		data := store.Get()
		status := "not ready"
		if data != nil {
			status = fmt.Sprintf("OK (%d bytes)", len(data))
		}
		fmt.Fprintf(w, "hidexx subscription server\nstatus: %s\nendpoint: /sub.yaml\n", status)
	})

	localIP := getLocalIP()
	addr := "0.0.0.0:" + port
	fmt.Println("=== hidexx subscription server ===")
	fmt.Println()
	fmt.Printf("listening on %s\n", addr)
	fmt.Println()
	fmt.Println("Shadowrocket config (one-time setup):")
	fmt.Printf("  URL: http://%s:%s/sub.yaml\n", localIP, port)
	fmt.Println()
	fmt.Println("  Shadowrocket -> bottom tab 'Config' -> '+' (top right)")
	fmt.Println("  -> paste the URL above -> Download -> Done")
	fmt.Println()
	fmt.Println("subscription will auto-renew every ~20 hours.")

	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintf(os.Stderr, "http server error: %v\n", err)
		os.Exit(1)
	}
}

func refreshSubscription(store *subStore, lineID string) error {
	log.Println("[refresh] starting daily renewal...")

	c, err := client.New("https://a.hidexx.com")
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	// 注册
	email, password := client.GenerateRandomAccount()
	log.Printf("[refresh] registering %s ...", email)
	if err := c.Register(email, password); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	log.Println("[refresh] register success")

	// 领取
	log.Println("[refresh] claiming free trial ...")
	if err := c.ClaimFreeTrial(lineID); err != nil {
		return fmt.Errorf("claim: %w", err)
	}
	log.Println("[refresh] claim success")

	// 获取订阅链接
	subs, err := c.GetSubscriptions()
	if err != nil {
		return fmt.Errorf("get subscriptions: %w", err)
	}
	if len(subs) == 0 {
		return fmt.Errorf("no subscription links found")
	}

	// 下载第一个订阅 YAML
	subURL := subs[0].URL
	log.Printf("[refresh] downloading subscription: %s", subURL)
	data, err := client.DownloadSubscriptionYAML(subURL)
	if err != nil {
		return fmt.Errorf("download yaml: %w", err)
	}

	store.Set(data)
	log.Printf("[refresh] done! serving %d bytes. account: %s", len(data), email)
	return nil
}

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
