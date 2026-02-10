package cmd

import (
	"fmt"
	"os"

	"github.com/liao/hidexx/client"
	"github.com/liao/hidexx/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to hidexx",
	Run:   runLogin,
}

func init() {
	loginCmd.Flags().String("email", "", "email address")
	loginCmd.Flags().String("password", "", "password")
	_ = viper.BindPFlag("email", loginCmd.Flags().Lookup("email"))
	_ = viper.BindPFlag("password", loginCmd.Flags().Lookup("password"))

	rootCmd.AddCommand(loginCmd)
}

func runLogin(cmd *cobra.Command, args []string) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config error: %v\n", err)
		os.Exit(1)
	}

	if cfg.Email == "" || cfg.Password == "" {
		fmt.Fprintf(os.Stderr, "email and password are required\n")
		fmt.Fprintf(os.Stderr, "set via config file (%s), env (HIDEXX_EMAIL/HIDEXX_PASSWORD), or flags (--email/--password)\n", config.ConfigFilePath())
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
}
