# FastGoCaptcha

[English](#english) | [中文](#中文)

## English

FastGoCaptcha is a high-performance, easy-to-integrate sliding captcha solution for Go applications. It provides a modern, user-friendly interface with robust security features.

### Features

- High Performance: Optimized for speed and efficiency
- Modern UI: Clean and responsive design
- Security: Built-in anti-replay protection
- Easy Integration: Simple middleware implementation
- Multiple Content Types: Supports form-urlencoded, multipart/form-data, and JSON
- Flexible Storage: Customizable storage backend with default in-memory implementation
- Zero Configuration: No CDN or external resources required
- Built-in Assets: All required resources are embedded using Go's embed feature

### Dependencies

- Go 1.16 or later
- github.com/google/uuid: For generating unique captcha IDs
- github.com/wenlng/go-captcha: Core captcha generation library
- github.com/wenlng/go-captcha-assets: Captcha assets (images and tiles)

### Acknowledgments

This project is built upon the excellent work of several open-source projects:

- go-captcha by wenlng: The core captcha generation library
- go-captcha-assets: The assets package providing captcha images and tiles

### Quick Start

1. Install the package:
```bash
go get github.com/VillanCh/fastgocaptcha
```

2. Basic usage:
```go
package main

import (
    "github.com/VillanCh/fastgocaptcha"
    "net/http"
)

func main() {
    // Create a new captcha instance
    captcha, err := fastgocaptcha.NewFastGoCaptcha()
    if err != nil {
        panic(err)
    }

    // Use as middleware
    http.Handle("/", captcha.Middleware(yourHandler))
    
    // Or use the test page directly
    http.Handle("/", captcha.GetTestPageHTTPHandler())
}
```

### API Endpoints

- `GET /fastgocaptcha/captcha`: Generate a new captcha
- `POST /fastgocaptcha/verify`: Verify the captcha solution
- `GET /static/fastgocaptcha/gocaptcha.global.css`: Captcha CSS styles
- `GET /static/fastgocaptcha/gocaptcha.global.js`: Captcha JavaScript
- `GET /static/fastgocaptcha/fastgocaptcha.js`: FastGoCaptcha helper JavaScript

### Client-Side Integration

FastGoCaptcha provides a built-in JavaScript helper for easy client-side integration. The `fastgocaptcha.js` file is automatically embedded and served with the application.

#### Using showSlideCaptcha

The `showSlideCaptcha` function provides an easy way to display and handle the captcha in your web application:

```javascript
// Include the script in your HTML
// <script src="/static/fastgocaptcha/fastgocaptcha.js"></script>

// Basic usage
showSlideCaptcha({
    captchaUrl: '/fastgocaptcha/captcha',  // URL to fetch captcha data
    verifyUrl: '/fastgocaptcha/verify',    // URL to verify captcha
    onSuccess: function() {
        console.log('Verification successful');
        // Handle successful verification
    },
    onError: function(msg) {
        console.error('Verification failed:', msg);
        // Handle verification failure
    }
});

// Advanced options
showSlideCaptcha({
    captchaUrl: '/fastgocaptcha/captcha',
    verifyUrl: '/fastgocaptcha/verify',
    containerId: 'captcha-container',  // Custom container ID
    title: 'Security Verification',    // Custom title
    subtitle: 'Slide to verify',       // Custom subtitle
    extraData: {                       // Extra data to send with verification
        token: 'your-token-here',
        userId: 'user-id'
    },
    onSuccess: function() {
        // Success callback
    },
    onError: function(msg) {
        // Error callback
    }
});
```

The `showSlideCaptcha` function supports the following options:

| Option | Type | Description |
|--------|------|-------------|
| captchaUrl | string | URL to fetch captcha data (default: `/fastgocaptcha/captcha`) |
| verifyUrl | string | URL to verify captcha (default: `/fastgocaptcha/verify`) |
| containerId | string | ID of container element (default: auto-generated) |
| title | string | Title of captcha dialog |
| subtitle | string | Subtitle of captcha dialog |
| extraData | object | Additional data to send with verification request |
| onSuccess | function | Callback on successful verification |
| onError | function | Callback on verification error |

### Response Examples

1. Captcha Generation Response:
```json
{
    "fastgocaptcha_id": "550e8400-e29b-41d4-a716-446655440000",
    "fastgocaptcha_image_base64": "base64_encoded_image_data",
    "fastgocaptcha_thumb_base64": "base64_encoded_thumb_data",
    "fastgocaptcha_thumb_width": 40,
    "fastgocaptcha_thumb_height": 40,
    "fastgocaptcha_thumb_x": 100,
    "fastgocaptcha_thumb_y": 50
}
```

2. Verification Response (Success):
```json
{
    "success": true,
    "message": "Verification successful"
}
```

3. Verification Response (Failure):
```json
{
    "success": false,
    "message": "Verification failed"
}
```

### Complete Example

Here's a complete example showing how to use FastGoCaptcha in your application:

```go
package main

import (
    "fmt"
    "log"
    "net/http"

    "github.com/VillanCh/fastgocaptcha"
)

func main() {
    // Set up logging
    log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

    // Create a new captcha instance
    captcha, err := fastgocaptcha.NewFastGoCaptcha()
    if err != nil {
        log.Fatalf("Failed to create captcha: %v", err)
    }

    // Serve the test page
    http.Handle("/", captcha.GetTestPageHTTPHandler())

    // Start the server with middleware
    port := 8126
    addr := fmt.Sprintf(":%d", port)
    log.Printf("Starting server at %s", addr)
    log.Printf("Access the application at http://localhost%s", addr)
    
    // Use the middleware to handle captcha requests
    log.Fatal(http.ListenAndServe(addr, captcha.Middleware(captcha.GetTestPageHTTPHandler())))
}
```

The example demonstrates:
1. Basic server setup with logging
2. Captcha instance creation
3. Test page serving
4. Middleware integration
5. Server startup with custom port

You can run this example and access the test page at `http://localhost:8126` to see the captcha in action.

### Custom Storage

You can implement your own storage backend using the provided options:

```go
captcha, err := fastgocaptcha.NewFastGoCaptcha(
    fastgocaptcha.WithStoreGoCaptchaData(yourStoreFunc),
    fastgocaptcha.WithLoadGoCaptchaData(yourLoadFunc),
    fastgocaptcha.WithDeleteGoCaptchaData(yourDeleteFunc),
)
```

## 中文

FastGoCaptcha 是一个高性能、易于集成的滑动验证码解决方案，专为 Go 应用程序设计。它提供了现代化的用户界面和强大的安全特性。

### 特性

- 高性能：针对速度和效率进行了优化
- 现代化界面：清晰且响应式设计
- 安全性：内置防重放保护
- 易于集成：简单的中间件实现
- 多种内容类型：支持 form-urlencoded、multipart/form-data 和 JSON
- 灵活存储：可自定义存储后端，默认使用内存实现
- 零配置：无需配置 CDN 或外部资源
- 内置资源：使用 Go 的 embed 特性嵌入所有必要资源

### 依赖说明

- Go 1.16 或更高版本
- github.com/google/uuid：用于生成唯一的验证码 ID
- github.com/wenlng/go-captcha：核心验证码生成库
- github.com/wenlng/go-captcha-assets：验证码资源（图片和滑块）

### 致谢

本项目基于以下优秀的开源项目构建：

- go-captcha 作者 wenlng：核心验证码生成库
- go-captcha-assets：提供验证码图片和滑块的资源包

### 快速开始

1. 安装包：
```bash
go get github.com/VillanCh/fastgocaptcha
```

2. 基本用法：
```go
package main

import (
    "github.com/VillanCh/fastgocaptcha"
    "net/http"
)

func main() {
    // 创建新的验证码实例
    captcha, err := fastgocaptcha.NewFastGoCaptcha()
    if err != nil {
        panic(err)
    }

    // 作为中间件使用
    http.Handle("/", captcha.Middleware(yourHandler))
    
    // 或直接使用测试页面
    http.Handle("/", captcha.GetTestPageHTTPHandler())
}
```

### API 接口

- `GET /fastgocaptcha/captcha`：生成新的验证码
- `POST /fastgocaptcha/verify`：验证验证码答案
- `GET /static/fastgocaptcha/gocaptcha.global.css`：验证码 CSS 样式
- `GET /static/fastgocaptcha/gocaptcha.global.js`：验证码 JavaScript
- `GET /static/fastgocaptcha/fastgocaptcha.js`：FastGoCaptcha 辅助 JavaScript

### 客户端集成

FastGoCaptcha 提供了内置的 JavaScript 辅助工具，便于客户端集成。`fastgocaptcha.js` 文件自动嵌入并随应用程序一起提供。

#### 使用 showSlideCaptcha

`showSlideCaptcha` 函数提供了一种简单的方式在你的 Web 应用中显示和处理验证码：

```javascript
// 在 HTML 中引入脚本
// <script src="/static/fastgocaptcha/fastgocaptcha.js"></script>

// 基本用法
showSlideCaptcha({
    captchaUrl: '/fastgocaptcha/captcha',  // 获取验证码数据的 URL
    verifyUrl: '/fastgocaptcha/verify',    // 验证验证码的 URL
    onSuccess: function() {
        console.log('验证成功');
        // 处理验证成功的逻辑
    },
    onError: function(msg) {
        console.error('验证失败:', msg);
        // 处理验证失败的逻辑
    }
});

// 高级选项
showSlideCaptcha({
    captchaUrl: '/fastgocaptcha/captcha',
    verifyUrl: '/fastgocaptcha/verify',
    containerId: 'captcha-container',  // 自定义容器 ID
    title: '安全验证',                  // 自定义标题
    subtitle: '滑动验证',               // 自定义副标题
    extraData: {                       // 验证时发送的额外数据
        token: 'your-token-here',
        userId: 'user-id'
    },
    onSuccess: function() {
        // 成功回调
    },
    onError: function(msg) {
        // 错误回调
    }
});
```

`showSlideCaptcha` 函数支持以下选项：

| 选项 | 类型 | 描述 |
|------|------|------|
| captchaUrl | string | 获取验证码数据的 URL（默认：`/fastgocaptcha/captcha`） |
| verifyUrl | string | 验证验证码的 URL（默认：`/fastgocaptcha/verify`） |
| containerId | string | 容器元素的 ID（默认：自动生成） |
| title | string | 验证码对话框的标题 |
| subtitle | string | 验证码对话框的副标题 |
| extraData | object | 验证请求时发送的额外数据 |
| onSuccess | function | 验证成功时的回调函数 |
| onError | function | 验证错误时的回调函数 |

### 响应示例

1. 验证码生成响应：
```json
{
    "fastgocaptcha_id": "550e8400-e29b-41d4-a716-446655440000",
    "fastgocaptcha_image_base64": "base64_encoded_image_data",
    "fastgocaptcha_thumb_base64": "base64_encoded_thumb_data",
    "fastgocaptcha_thumb_width": 40,
    "fastgocaptcha_thumb_height": 40,
    "fastgocaptcha_thumb_x": 100,
    "fastgocaptcha_thumb_y": 50
}
```

2. 验证成功响应：
```json
{
    "success": true,
    "message": "Verification successful"
}
```

3. 验证失败响应：
```json
{
    "success": false,
    "message": "Verification failed"
}
```

### 完整示例

以下是一个完整的示例，展示如何在你的应用中使用 FastGoCaptcha：

```go
package main

import (
    "fmt"
    "log"
    "net/http"

    "github.com/VillanCh/fastgocaptcha"
)

func main() {
    // 设置日志格式
    log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

    // 创建新的验证码实例
    captcha, err := fastgocaptcha.NewFastGoCaptcha()
    if err != nil {
        log.Fatalf("创建验证码失败: %v", err)
    }

    // 提供测试页面
    http.Handle("/", captcha.GetTestPageHTTPHandler())

    // 启动服务器（使用中间件）
    port := 8126
    addr := fmt.Sprintf(":%d", port)
    log.Printf("服务器启动在 %s", addr)
    log.Printf("访问地址: http://localhost%s", addr)
    
    // 使用中间件处理验证码请求
    log.Fatal(http.ListenAndServe(addr, captcha.Middleware(captcha.GetTestPageHTTPHandler())))
}
```

这个示例展示了：
1. 基本的服务器设置和日志配置
2. 验证码实例的创建
3. 测试页面的提供
4. 中间件的集成
5. 自定义端口的服务器启动

你可以运行这个示例，然后访问 `http://localhost:8126` 来查看验证码的实际效果。

### 自定义存储

你可以使用提供的选项实现自己的存储后端：

```go
captcha, err := fastgocaptcha.NewFastGoCaptcha(
    fastgocaptcha.WithStoreGoCaptchaData(yourStoreFunc),
    fastgocaptcha.WithLoadGoCaptchaData(yourLoadFunc),
    fastgocaptcha.WithDeleteGoCaptchaData(yourDeleteFunc),
)
``` 