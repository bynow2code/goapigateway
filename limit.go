package main

import (
	"log"
	"net/http"
	"sync"
	"time"
)

// TokenBucket 令牌桶结构体，用于实现限流功能。
// Cap: 桶的最大令牌容量。
// Rate: 每秒生成的令牌数量。
// interval: 令牌生成的时间间隔（默认为1秒）。
// Tokens: 当前桶中剩余的令牌数。
// LastCheck: 上一次检查并补充令牌的时间点。
// mu: 互斥锁，确保并发安全。
type TokenBucket struct {
	Cap       int           // 桶最大容量
	Rate      int           // 生成速率
	interval  time.Duration // 生成间隔
	Tokens    int           // 当前令牌数
	LastCheck time.Time     // 上次生成令牌的时间
	mu        sync.Mutex    // 保证并发安全
}

// NewTokenBucket 创建一个新的令牌桶实例。
// 参数 cap 表示桶的最大令牌容量。
// 参数 rate 表示每秒新增的令牌数量。
// 返回一个初始化完成的 TokenBucket 实例指针。
func NewTokenBucket(cap int, rate int) *TokenBucket {
	return &TokenBucket{
		Cap:       cap,
		Rate:      rate,
		interval:  1 * time.Second,
		Tokens:    cap,
		LastCheck: time.Now(),
		mu:        sync.Mutex{},
	}
}

// Allow 判断是否允许通过请求，并扣除一个令牌。
// 若当前桶中有足够令牌则返回 true 并减少一个令牌；
// 否则返回 false，表示被限流。
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// 根据时间差计算需要补充的令牌数
	now := time.Now()
	duration := now.Sub(tb.LastCheck)
	tokens := int(duration.Seconds() * float64(tb.Rate) / tb.interval.Seconds())

	// 补充令牌，但不超过桶的最大容量
	if tokens > 0 {
		tb.Tokens = min(tb.Tokens+tokens, tb.Cap)
		tb.LastCheck = now
	}

	// 尝试获取一个令牌
	if tb.Tokens > 0 {
		tb.Tokens--
		return true
	}
	return false
}

// 全局默认限流器：容量为1，速率为1 QPS
var globalLimiter = NewTokenBucket(1, 1)

// RateLimitMiddleware 是一个中间件工厂函数，根据路由配置应用不同的限流策略。
// 参数 routes 包含各个路径对应的 QPS 配置信息。
// 返回一个包装后的 http.HandlerFunc 处理器。
func RateLimitMiddleware(rotes []Route) Middleware {
	// 提前给路由构建好各自的限流器
	routeLimiters := make(map[string]*TokenBucket)
	for _, route := range rotes {
		if route.QPS > 0 {
			routeLimiters[route.Path] = NewTokenBucket(route.QPS*2, route.QPS)
			break
		}
	}

	// 返回实际的中间件处理逻辑
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			var limiter *TokenBucket

			// 查找该请求路径是否有专用的限流器
			for path, tb := range routeLimiters {
				if r.URL.Path == path {
					limiter = tb
					break
				}
			}

			// 如果没有找到专用限流器，则使用全局默认限流器
			if limiter == nil {
				limiter = globalLimiter
			}

			// 执行限流判断
			if limiter.Allow() {
				next(w, r)
			} else {
				// 触发限流时返回 429 状态码和提示信息
				w.Header().Set("Retry-After", "1")
				http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
				log.Printf("[限流] %s %s | 超过QPS限制", r.Method, r.URL.Path)
				return
			}
		}
	}
}
