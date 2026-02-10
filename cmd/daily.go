package cmd

import (
	"fmt"
	"os"

	"github.com/liao/hidexx/client"
	"github.com/spf13/cobra"
)

var dailyCmd = &cobra.Command{
	Use:   "daily",
	Short: "One-click: register new account -> claim free trial -> show subscription links",
	Run:   runDaily,
}

func init() {
	dailyCmd.Flags().String("line", "1", `line_id: "1" for 王者套餐, "11" for 青铜套餐`)

	rootCmd.AddCommand(dailyCmd)
}

func runDaily(cmd *cobra.Command, args []string) {
	baseURL := "https://a.hidexx.com"

	c, err := client.New(baseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create client error: %v\n", err)
		os.Exit(1)
	}

	// 1. 生成随机账号
	email, password := client.GenerateRandomAccount()
	fmt.Printf("[1/4] registering %s ...\n", email)

	if err := c.Register(email, password); err != nil {
		fmt.Fprintf(os.Stderr, "register error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("register success (auto-logged in)")

	// 2. 领取免费试用
	lineID, _ := cmd.Flags().GetString("line")
	fmt.Printf("[2/4] claiming free trial (line_id=%s) ...\n", lineID)

	if err := c.ClaimFreeTrial(lineID); err != nil {
		fmt.Fprintf(os.Stderr, "claim error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("claim success")

	// 3. 获取订阅链接
	fmt.Println("[3/4] fetching subscription links ...")

	subs, err := c.GetSubscriptions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "get subscriptions error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n[4/4] done! %d subscription link(s):\n\n", len(subs))
	for i, s := range subs {
		if s.Label != "" {
			fmt.Printf("  [%d] %s\n      %s\n\n", i+1, s.Label, s.URL)
		} else {
			fmt.Printf("  [%d] %s\n\n", i+1, s.URL)
		}
	}

	fmt.Println("--- account credentials (save if needed) ---")
	fmt.Printf("  email:    %s\n", email)
	fmt.Printf("  password: %s\n", password)
}
