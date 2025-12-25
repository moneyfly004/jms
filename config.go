package main

import (
	"os"
	"strconv"
)

// Config 配置
type Config struct {
	GitHubToken    string
	GitHubRepo     string
	MaxConcurrency int
	TestTimeout    int // 秒
}

// LoadConfig 加载配置
func LoadConfig() *Config {
	config := &Config{
		GitHubToken:    os.Getenv("GITHUB_TOKEN"),
		GitHubRepo:     os.Getenv("GITHUB_REPO"),
		MaxConcurrency: 10,
		TestTimeout:    5,
	}

	// 从环境变量读取并发数
	if maxConcurrency := os.Getenv("MAX_CONCURRENCY"); maxConcurrency != "" {
		if n, err := parseInt(maxConcurrency); err == nil && n > 0 {
			config.MaxConcurrency = n
		}
	}

	// 从环境变量读取超时时间
	if timeout := os.Getenv("TEST_TIMEOUT"); timeout != "" {
		if n, err := parseInt(timeout); err == nil && n > 0 {
			config.TestTimeout = n
		}
	}

	return config
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}
