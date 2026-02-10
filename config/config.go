package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	BaseURL  string `mapstructure:"base_url"`
	Email    string `mapstructure:"email"`
	Password string `mapstructure:"password"`
}

const defaultBaseURL = "https://a.hidexx.com"

func Load() (*Config, error) {
	viper.SetDefault("base_url", defaultBaseURL)

	// 配置文件: ~/.hidexx.yaml
	home, err := os.UserHomeDir()
	if err == nil {
		viper.AddConfigPath(home)
		viper.SetConfigName(".hidexx")
		viper.SetConfigType("yaml")
	}

	// 当前目录也查找
	viper.AddConfigPath(".")
	viper.SetConfigName(".hidexx")

	// 环境变量
	viper.SetEnvPrefix("HIDEXX")
	viper.AutomaticEnv()

	// 读取配置文件（不存在不报错）
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}

	return &cfg, nil
}

// ConfigFilePath returns the default config file path.
func ConfigFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".hidexx.yaml"
	}
	return filepath.Join(home, ".hidexx.yaml")
}
