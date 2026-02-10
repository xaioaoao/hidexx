package cmd

import (
	"fmt"
	"os"

	"github.com/liao/hidexx/client"
	"github.com/liao/hidexx/config"
	"github.com/spf13/cobra"
)

var claimCmd = &cobra.Command{
	Use:   "claim",
	Short: "Login and claim free one-day trial",
	Run:   runClaim,
}

func init() {
	claimCmd.Flags().String("line", "1", `line_id: "1" for 王者套餐, "11" for 青铜套餐`)

	rootCmd.AddCommand(claimCmd)
}

func runClaim(cmd *cobra.Command, args []string) {
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

	lineID, _ := cmd.Flags().GetString("line")
	fmt.Printf("claiming free trial (line_id=%s) ...\n", lineID)
	if err := c.ClaimFreeTrial(lineID); err != nil {
		fmt.Fprintf(os.Stderr, "claim error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("claim success! subscription will be delivered in ~5 seconds")
}
