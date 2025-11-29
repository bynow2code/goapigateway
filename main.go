package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// ResponseWriterWrapper 包装 http.ResponseWriter 以捕获状态码
type ResponseWriterWrapper struct {
	http.ResponseWriter
	StatusCode int // 响应的状态码
}

// WriteHeader 实现 http.ResponseWriter 接口，并记录状态码
func (w *ResponseWriterWrapper) WriteHeader(statusCode int) {
	w.StatusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// Middleware 是中间件类型定义，用于包装 HTTP 处理函数
type Middleware func(handlerFunc http.HandlerFunc) http.HandlerFunc

// main 函数是程序入口点，初始化路由、中间件并启动 HTTP 服务器
func main() {
	config, err := loadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 初始化中间件链
	middlewares := []Middleware{
		AuthMiddleware(config),
		RateLimitMiddleware(config),
		CORSAMiddleware(),
		TimeoutMiddleware(3 * time.Second),
		LogMiddleware(),
	}

	// 构建处理函数并注册到根路径
	handler := ChainMiddleware(proxyHandler(config.Routes), middlewares...)
	http.HandleFunc("/", handler)

	fmt.Printf("服务已启动：[%s]\n", config.Port)

	// 启动 HTTP 服务器监听在端口 8082 上
	err = http.ListenAndServe(config.Port, nil)
	if err != nil {
		fmt.Println("Error starting server:", err)
		return
	}
}

func AuthMiddleware(config *Config) Middleware {
	apiKeysSet := make(map[string]struct{})
	for _, v := range config.ApiKeys {
		apiKeysSet[v] = struct{}{}
	}

	noAuthSet := make(map[string]struct{})
	for _, v := range config.NoAuthRoutes {
		noAuthSet[v] = struct{}{}
	}

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if _, ok := noAuthSet[r.URL.Path]; ok {
				next(w, r)
				return
			}

			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				w.Header().Set("WWW-Authenticate", "X-API-Key") // 提示客户端需要携带API Key
				http.Error(w, "401 Unauthorized: Missing X-API-Key", http.StatusUnauthorized)
				log.Printf("[认证失败] %s %s | 未携带API Key", r.Method, r.URL.Path)
				return
			}

			if _, ok := apiKeysSet[apiKey]; ok {
				log.Printf("[认证成功] %s %s | API Key: %s", r.Method, r.URL.Path, apiKey)
				next(w, r)
			} else {
				http.Error(w, "401 Unauthorized: Invalid X-API-Key", http.StatusUnauthorized)
				log.Printf("[认证失败] %s %s | 非法API Key: %s", r.Method, r.URL.Path, apiKey)
				return
			}
		}
	}
}

// LogMiddleware 返回一个日志记录中间件，用于记录请求方法、路径、响应状态码及耗时
//
// 参数:
//   - next: 下一个处理函数
//
// 返回值:
//   - http.HandlerFunc: 包含日志功能的处理函数
func LogMiddleware() Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			method := r.Method
			path := r.URL.Path

			// 使用包装器来获取响应状态码
			wrappedWriter := &ResponseWriterWrapper{
				ResponseWriter: w,
			}

			next(w, r)

			// 计算请求耗时（毫秒）
			cost := time.Since(start).Seconds() * 1000
			log.Printf("[%s] %s | 状态码：%d | 耗时：%.2fs", method, path, wrappedWriter.StatusCode, cost)
		}
	}
}

// TimeoutMiddleware 返回一个超时控制中间件，在指定时间内未完成请求则返回超时错误
//
// 参数:
//   - timeout: 超时持续时间
//
// 返回值:
//   - Middleware: 包含超时控制逻辑的中间件
func TimeoutMiddleware(timeout time.Duration) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// 创建带超时的上下文
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			// 将新上下文附加到请求中
			r = r.WithContext(ctx)

			// 创建通道用于通知请求是否已完成
			done := make(chan struct{})

			// 在 goroutine 中执行下一个处理器
			go func() {
				next(w, r)
				close(done)
			}()

			// 等待请求完成或超时
			select {
			case <-done:
				// 正常结束
			case <-ctx.Done():
				// 超时处理
				http.Error(w, "Request timeout", http.StatusGatewayTimeout)
				log.Printf("[超时] %s %s | 超时时间：%.2f", r.Method, r.URL.Path, timeout.Seconds()*1000)
			}
		}
	}
}

// CORSAMiddleware 返回一个跨域资源共享(CORS)支持的中间件
//
// 参数:
//   - next: 下一个处理函数
//
// 返回值:
//   - http.HandlerFunc: 支持 CORS 的处理函数
func CORSAMiddleware() Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// 设置允许所有来源访问
			w.Header().Set("Access-Control-Allow-Origin", "*")
			// 允许常用的 HTTP 方法
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			// 允许常见的头部字段
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			// 对于预检请求直接返回成功状态
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}

			next(w, r)
		}
	}
}

// ChainMiddleware 按照从右到左的顺序组合多个中间件形成调用链
//
// 参数:
//   - handler: 最终要被调用的处理函数
//   - middlewares: 中间件列表
//
// 返回值:
//   - http.HandlerFunc: 经过中间件层层包裹后的最终处理函数
func ChainMiddleware(handler http.HandlerFunc, middlewares ...Middleware) http.HandlerFunc {
	// 逆序应用中间件，确保最左边的中间件最先执行
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// proxyHandler 根据请求路径将请求代理转发至对应的目标地址
//
// 参数:
//   - routes: 路由规则数组，包含源路径与目标地址的映射
//
// 返回值:
//   - http.HandlerFunc: 反向代理处理函数
func proxyHandler(routes []RouteConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var target string
		// 查找匹配的路由规则
		for _, route := range routes {
			if route.Path == r.URL.Path {
				target = route.Target
			}
		}

		// 若没有找到对应的路由规则，则返回 404 错误
		if target == "" {
			http.Error(w, "404 Route Not Found", http.StatusNotFound)
			return
		}

		// 构造新的请求对象，携带原始请求的方法、URL 和 Body
		req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
		if err != nil {
			http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
			return
		}

		// 复制原请求的所有 Header 到新请求中
		for k, v := range r.Header {
			req.Header[k] = v
		}

		// 发起请求并将结果回传给客户端
		client := http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "Failed to forward request", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// 回写响应头信息
		for k, v := range resp.Header {
			w.Header()[k] = v
		}

		// 写入响应状态码
		w.WriteHeader(resp.StatusCode)

		// 将远程响应体复制到当前响应流中
		_, err = io.Copy(w, resp.Body)
		if err != nil {
			http.Error(w, "Failed to copy response body", http.StatusBadGateway)
			return
		}
	}
}
