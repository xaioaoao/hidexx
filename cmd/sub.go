package cmd

import (
	"fmt"
	"os"

	"github.com/liao/hidexx/client"
	"github.com/liao/hidexx/config"
	"github.com/spf13/cobra"
)

var subCmd = &cobra.Command{
	Use:   "sub",
	Short: "Login and show subscription links",
	Run:   runSub,
}

func init() {
	rootCmd.AddCommand(subCmd)
}

func runSub(cmd *cobra.Command, args []string) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config error: %v\n", err)
		os.Exit(1)
	}

	if cfg.Email == "" || cfg.Password == "" {
		fmt.Fprintf(os.Stderr, "email and password are required\n")
		os.Exit(1)
	}

	c, err := client.New(cfg.BaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create client error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("logging in as %s ...\n", cfg.Email)
	if err := c.Login(cfg.Email, cfg.Password); err != nil {
		fmt.Fprintf(os.Stderr, "login error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("login success")

	subs, err := c.GetSubscriptions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "get subscriptions error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nfound %d subscription link(s):\n\n", len(subs))
	for i, s := range subs {
		if s.Label != "" {
			fmt.Printf("  [%d] %s\n      %s\n\n", i+1, s.Label, s.URL)
		} else {
			fmt.Printf("  [%d] %s\n\n", i+1, s.URL)
		}
	}

	fmt.Println("Shadowrocket usage:")
	fmt.Println("  1. Copy any subscription URL above")
	fmt.Println("  2. Open Shadowrocket -> Settings (bottom tab)")
	fmt.Println("  3. Tap 'Subscribe' -> '+' (top right)")
	fmt.Println("  4. Paste the URL -> Save")
	fmt.Println("  5. Tap the subscription to update nodes")
}
