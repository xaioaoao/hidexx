package cmd

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
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
	serveCmd.Flags().StringP("port", "p", "51991", "HTTP server listen port")
	serveCmd.Flags().String("line", "1", `line_id: "1" for 王者套餐, "11" for 青铜套餐`)
	serveCmd.Flags().IntP("users", "n", 1, "number of users (each gets an independent subscription)")

	rootCmd.AddCommand(serveCmd)
}

type subStore struct {
	mu    sync.RWMutex
	slots [][]byte // slots[0] = user 1, slots[1] = user 2, ...
}

func newSubStore(n int) *subStore {
	return &subStore{slots: make([][]byte, n)}
}

func (s *subStore) Set(index int, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.slots[index] = data
}

func (s *subStore) Get(index int) []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if index < 0 || index >= len(s.slots) {
		return nil
	}
	return s.slots[index]
}

func (s *subStore) Len() int {
	return len(s.slots)
}

func runServe(cmd *cobra.Command, args []string) {
	port, _ := cmd.Flags().GetString("port")
	lineID, _ := cmd.Flags().GetString("line")
	numUsers, _ := cmd.Flags().GetInt("users")
	if numUsers < 1 {
		numUsers = 1
	}

	store := newSubStore(numUsers)

	// 启动时立即执行
	refreshAll(store, lineID)

	// 后台定时刷新
	go func() {
		for {
			interval := nextRefreshInterval(store)
			time.Sleep(interval)
			refreshAll(store, lineID)
		}
	}()

	// HTTP 服务
	mux := http.NewServeMux()

	// 每个用户一个独立 endpoint: /1/sub.yaml, /2/sub.yaml, ...
	for i := 0; i < numUsers; i++ {
		idx := i
		userID := i + 1
		path := fmt.Sprintf("/%d/sub.yaml", userID)

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			log.Printf("[access] %s %s from %s - UA: %s", r.Method, r.URL.Path, r.RemoteAddr, r.UserAgent())
			data := store.Get(idx)
			if data == nil {
				http.Error(w, "subscription not ready yet, try again later", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=hidexx-%d.yaml", userID))
			w.Write(data)
			log.Printf("[access] served %d bytes to %s (user %d)", len(data), r.RemoteAddr, userID)
		})
	}

	// 兼容旧的单用户路径，指向 user 1
	mux.HandleFunc("/sub.yaml", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[access] %s %s from %s - UA: %s", r.Method, r.URL.Path, r.RemoteAddr, r.UserAgent())
		data := store.Get(0)
		if data == nil {
			http.Error(w, "subscription not ready yet, try again later", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=hidexx.yaml")
		w.Write(data)
	})

	// 状态页
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[access] %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		fmt.Fprintln(w, "hidexx subscription server")
		fmt.Fprintf(w, "users: %d\n\n", numUsers)
		for i := 0; i < numUsers; i++ {
			data := store.Get(i)
			status := "not ready"
			if data != nil {
				status = fmt.Sprintf("OK (%d bytes)", len(data))
			}
			fmt.Fprintf(w, "  user %d: %s  ->  /%d/sub.yaml\n", i+1, status, i+1)
		}
	})

	localIP := getLocalIP()
	addr := "0.0.0.0:" + port
	fmt.Println("=== hidexx subscription server ===")
	fmt.Println()
	fmt.Printf("listening on %s\n", addr)
	fmt.Printf("users: %d\n", numUsers)
	fmt.Println()
	fmt.Println("subscription URLs (one per person, configure once):")
	for i := 0; i < numUsers; i++ {
		fmt.Printf("  user %d: http://%s:%s/%d/sub.yaml\n", i+1, localIP, port, i+1)
	}
	fmt.Println()
	fmt.Println("subscription will auto-renew every ~20 hours.")

	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintf(os.Stderr, "http server error: %v\n", err)
		os.Exit(1)
	}
}

func refreshAll(store *subStore, lineID string) {
	for i := 0; i < store.Len(); i++ {
		userID := i + 1
		log.Printf("[user %d] starting daily renewal...", userID)
		if err := refreshOne(store, i, lineID); err != nil {
			log.Printf("[user %d] refresh failed: %v, will retry next cycle", userID, err)
		}
		if i < store.Len()-1 {
			time.Sleep(5 * time.Second)
		}
	}
}

func nextRefreshInterval(store *subStore) time.Duration {
	for i := 0; i < store.Len(); i++ {
		if store.Get(i) == nil {
			return 1 * time.Hour
		}
	}
	return 20 * time.Hour
}

func refreshOne(store *subStore, index int, lineID string) error {
	userID := index + 1
	tag := "[user " + strconv.Itoa(userID) + "]"

	c, err := client.New("https://a.hidexx.com")
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	email, password := client.GenerateRandomAccount()
	log.Printf("%s registering %s ...", tag, email)
	if err := c.Register(email, password); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	log.Printf("%s register success", tag)

	log.Printf("%s claiming free trial ...", tag)
	if err := c.ClaimFreeTrial(lineID); err != nil {
		return fmt.Errorf("claim: %w", err)
	}
	log.Printf("%s claim success, waiting 10s for provisioning...", tag)
	time.Sleep(10 * time.Second)

	subs, err := c.GetSubscriptions()
	if err != nil {
		return fmt.Errorf("get subscriptions: %w", err)
	}
	if len(subs) == 0 {
		return fmt.Errorf("no subscription links found")
	}

	subURL := subs[0].URL
	log.Printf("%s downloading subscription: %s", tag, subURL)
	data, err := client.DownloadSubscriptionYAML(subURL)
	if err != nil {
		return fmt.Errorf("download yaml: %w", err)
	}

	store.Set(index, data)
	log.Printf("%s done! serving %d bytes. account: %s / %s", tag, len(data), email, password)
	return nil
}
