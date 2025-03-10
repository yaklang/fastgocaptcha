package main

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"time"

	"github.com/yaklang/fastgocaptcha"
)

// 获取可用端口
func getAvailablePort() int {
	// 先尝试默认端口 8370
	if isPortAvailable(8370) {
		return 8370
	}

	// 如果默认端口被占用，则随机选择 8000-9000 之间的端口
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < 100; i++ { // 最多尝试100次
		port := rand.Intn(1000) + 8000
		if isPortAvailable(port) {
			return port
		}
	}
	return 8000 // 如果实在找不到可用端口，返回 8000
}

// 检查端口是否可用
func isPortAvailable(port int) bool {
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

func main() {
	// 设置日志格式
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	cap, err := fastgocaptcha.NewFastGoCaptcha()
	if err != nil {
		log.Fatalf("create fast go capture failed: %s", err)
	}

	http.Handle("/", cap.GetTestPageHTTPHandler())

	// 检查 8126 端口是否可用，如果被占用则获取随机端口
	port := 8126
	if !isPortAvailable(port) {
		port = getAvailablePort()
	}
	addr := fmt.Sprintf(":%d", port)
	log.Printf("Starting server at %s", addr)
	log.Printf("Access the application at http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, cap.Middleware(cap.GetTestPageHTTPHandler())))
}
