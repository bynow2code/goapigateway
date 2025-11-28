package main

import (
	"fmt"
	"io"
	"net/http"
)

type Route struct {
	Path   string
	Target string
}

func main() {
	routes := []Route{
		{Path: "/api/baidu", Target: "https://www.baidu.com"},
		{Path: "/api/github", Target: "https://api.github.com"},
		{Path: "/api/local", Target: "http://localhost:8081"}, // 可对接你之前写的文件服务器
	}
	http.HandleFunc("/", proxyHandler(routes))
	err := http.ListenAndServe(":8082", nil)
	if err != nil {
		fmt.Println("Error starting server:", err)
		return
	}
}

func proxyHandler(routes []Route) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var target string
		for _, route := range routes {
			if route.Path == r.URL.Path {
				target = route.Target
			}
		}

		if target == "" {
			http.Error(w, "404 Route Not Found", http.StatusNotFound)
			return
		}

		req, err := http.NewRequest(r.Method, target, r.Body)
		if err != nil {
			http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
			return
		}

		for k, v := range r.Header {
			req.Header[k] = v
		}

		client := http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "Failed to forward request", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		for k, v := range resp.Header {
			w.Header()[k] = v
		}

		w.WriteHeader(resp.StatusCode)
		_, err = io.Copy(w, resp.Body)
		if err != nil {
			http.Error(w, "Failed to copy response body", http.StatusBadGateway)
			return
		}
	}
}
