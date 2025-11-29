package main

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 配置结构体，定义了服务的基本配置信息
// Port: 服务监听的端口号
// ApiKeys: API访问密钥列表，用于身份验证
// NoAuthRoutes: 无需身份验证的路由路径列表
// Routes: 路由配置列表，包含路径映射和转发目标
// GlobalRateLimit: 全局速率限制配置
// Timeout: 请求超时时间
type Config struct {
	Port            string                `yaml:"port"`
	ApiKeys         []string              `yaml:"apiKeys"`
	NoAuthRoutes    []string              `yaml:"noAuthRoutes"`
	Routes          []RouteConfig         `yaml:"routes"`
	GlobalRateLimit GlobalRateLimitConfig `yaml:"globalRateLimit"`
	Timeout         time.Duration         `yaml:"timeout"`
}

// RouteConfig 路由配置结构体，定义了单个路由的转发规则
// Path: 请求路径
// Target: 转发目标地址
// QPS: 每秒查询率限制
type RouteConfig struct {
	Path   string `yaml:"path"`
	Target string `yaml:"target"`
	QPS    int    `yaml:"qps"`
}

// GlobalRateLimitConfig 全局速率限制配置结构体
// Cap: 令牌桶容量
// Rate: 令牌生成速率
type GlobalRateLimitConfig struct {
	Cap  int `yaml:"cap"`
	Rate int `yaml:"rate"`
}

// loadConfig 从指定文件路径加载配置文件并解析为Config结构体
// filepath: 配置文件的路径
// 返回值: 解析后的Config指针和可能的错误信息
func loadConfig(filepath string) (*Config, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	// 填充默认值
	if config.Port == "" {
		config.Port = ":8082"
	}
	if config.GlobalRateLimit.Cap == 0 {
		config.GlobalRateLimit.Cap = 3
	}
	if config.GlobalRateLimit.Rate == 0 {
		config.GlobalRateLimit.Rate = 1
	}
	if config.Timeout == 0 {
		config.Timeout = 1 * time.Second
	}

	return &config, nil
}
