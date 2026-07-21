# HTML 提取库

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://golang.org)
[![GoDoc](https://pkg.go.dev/badge/github.com/cybergodev/html.svg)](https://pkg.go.dev/github.com/cybergodev/html)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Security](https://img.shields.io/badge/security-policy-blue.svg)](docs/SECURITY.md)
[![Thread Safe](https://img.shields.io/badge/thread%20safe-yes-brightgreen.svg)](#-线程安全)

**一个高性能的 Go 库，用于智能 HTML 内容提取**，基于 `golang.org/x/net/html` 构建。

**[English Documentation](README.md)** | **[www.cybergo.dev/html](https://www.cybergo.dev/html)**

---

## 🎯 特性

| 特性 | 描述 |
|------|------|
| 🚀 **一行代码提取** | 单个函数调用即可从 HTML 提取纯净文本 |
| 🔍 **智能文章检测** | 使用评分算法识别主要内容 |
| 🌐 **自动编码检测** | 支持 UTF-8、Windows-1252、GBK、Shift_JIS 等 |
| 🔄 **批量处理** | 支持 Worker Pool 和 Context 的并行提取 |
| 📦 **多种输出格式** | 文本、Markdown、JSON |
| 🛡️ **安全优先** | HTML 净化、XSS 防护、审计日志 |
| 🧵 **线程安全** | 无需外部同步即可并发使用 |
| 🔗 **基于 golang.org/x/net/html** | 内部使用标准 HTML 解析器 |

---

## 🌐 应用场景

- **新闻聚合器**：从新闻网站提取文章内容
- **网页爬虫**：从 HTML 页面获取结构化数据
- **内容管理**：将 HTML 转换为 Markdown 或其他格式
- **搜索引擎**：索引主要内容，排除导航和广告
- **数据分析**：大规模提取和分析网页内容
- **RSS 订阅生成器**：提取内容用于订阅源创建
- **归档工具**：保存网页内容

---

## 📦 安装

```bash
go get github.com/cybergodev/html
```

**系统要求**：Go 1.25+

---

## ⚡ 30 秒快速入门

```go
package main

import (
    "fmt"
    "log"
    "github.com/cybergodev/html"
)

func main() {
    // 一行代码：从 HTML 中提取纯净文本
    htmlBytes := []byte(`
        <html>
            <nav>导航栏</nav>
            <article><h1>你好世界</h1><p>这里是内容...</p></article>
            <footer>页脚</footer>
        </html>
    `)

    text, err := html.ExtractText(htmlBytes)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(text)
    // 输出: "你好世界\n\n这里是内容..."
}
```

**自动完成的工作：**
- ✅ 移除导航、页脚、广告、脚本
- ✅ 使用评分算法检测主要内容
- ✅ 处理字符编码（UTF-8、Windows-1252、GBK 等）
- ✅ 清理空白字符

---

## 🚀 使用指南

### 1️⃣ 包级函数（最简单）

对于单次提取，使用包级函数：

```go
package main

import (
    "fmt"
    "github.com/cybergodev/html"
)

func main() {
    htmlBytes := []byte(`<html><body><h1>标题</h1><p>内容在这里...</p></body></html>`)

    // 仅提取文本
    text, _ := html.ExtractText(htmlBytes)

    // 提取所有内容（含元数据）
    result, _ := html.Extract(htmlBytes)
    fmt.Println(result.Title)     // "标题"
    fmt.Println(result.Text)      // "标题\n\n内容在这里..."
    fmt.Println(result.WordCount) // 2

    // 提取所有资源链接
    links, _ := html.ExtractAllLinks(htmlBytes)

    // 格式转换
    markdown, _ := html.ExtractToMarkdown(htmlBytes)
    jsonData, _ := html.ExtractToJSON(htmlBytes)
}
```

---

### 2️⃣ Processor 使用（推荐用于多次提取）

对于多次提取，创建 Processor 以利用缓存：

```go
package main

import (
    "fmt"
    "log"
    "github.com/cybergodev/html"
)

func main() {
    // 使用默认配置创建 Processor
    processor, err := html.New()
    if err != nil {
        log.Fatal(err)
    }
    defer processor.Close()

    htmlBytes := []byte(`<html><body><h1>标题</h1><p>内容</p></body></html>`)

    // 使用默认配置提取
    result, _ := processor.Extract(htmlBytes)

    // 从文件提取
    result, _ = processor.ExtractFromFile("page.html")

    // 批量处理
    htmlContents := [][]byte{htmlBytes, htmlBytes, htmlBytes}
    batchResult := processor.ExtractBatch(htmlContents)

    fmt.Printf("已处理 %d 个文档\n", len(batchResult.Results))
}
```

---

### 3️⃣ 自定义配置

```go
package main

import (
    "fmt"
    "github.com/cybergodev/html"
)

func main() {
    htmlBytes := []byte(`<html><body><h1>标题</h1><img src="img.jpg"><p>内容</p></body></html>`)

    // 从 DefaultConfig 开始并自定义
    config := html.DefaultConfig()
    config.PreserveVideos = false       // 跳过视频
    config.PreserveAudios = false       // 跳过音频
    config.InlineImageFormat = "none"   // 选项: "none", "markdown", "html", "placeholder"
    config.InlineLinkFormat = "none"    // 选项: "none", "markdown", "html"
    config.TableFormat = "markdown"     // 选项: "markdown", "html"

    processor, _ := html.New(config)
    defer processor.Close()

    result, _ := processor.Extract(htmlBytes)
    fmt.Printf("找到 %d 张图片\n", len(result.Images))
}
```

---

### 4️⃣ 预设配置

```go
// 仅文本 - 不保留媒体
processor, _ := html.New(html.TextOnlyConfig())

// Markdown 输出 - 图片格式化为 markdown
processor, _ := html.New(html.MarkdownConfig())

// 默认 - 启用所有功能
processor, _ := html.New(html.DefaultConfig())

// 高安全 - 对不受信任的输入使用更严格的限制
processor, _ := html.New(html.HighSecurityConfig())
```

---

### 5️⃣ 高级配置

```go
package main

import (
    "time"
    "github.com/cybergodev/html"
)

func main() {
    config := html.Config{
        MaxInputSize:       10 * 1024 * 1024, // 10MB 限制
        ProcessingTimeout:  30 * time.Second,
        MaxCacheEntries:    500,
        CacheTTL:           30 * time.Minute,
        CacheCleanup:       5 * time.Minute,  // 后台清理间隔
        WorkerPoolSize:     8,
        EnableSanitization: true,  // 移除 <script>, <style> 标签
        MaxDepth:           50,    // 防止深度嵌套攻击
    }
    processor, _ := html.New(config)
    defer processor.Close()
}
```

---

## 📖 常用示例

### 提取文章文本（纯净）

```go
text, _ := html.ExtractText(htmlBytes)
// 返回不含导航/广告的纯净文本
```

### 从文件提取

```go
// 从文件提取文本
text, _ := html.ExtractTextFromFile("page.html")

// 从文件提取完整结果
result, _ := html.ExtractFromFile("page.html")

// 将文件转换为 Markdown
markdown, _ := html.ExtractToMarkdownFromFile("page.html")

// 将文件转换为 JSON
jsonData, _ := html.ExtractToJSONFromFile("page.html")
```

### 提取内容与图片

```go
result, _ := html.Extract(htmlBytes)
for _, img := range result.Images {
    fmt.Printf("图片: %s (alt: %s)\n", img.URL, img.Alt)
}
```

### 提取所有链接

```go
processor, _ := html.New()
defer processor.Close()

links, _ := processor.ExtractAllLinks(htmlBytes)
for _, link := range links {
    fmt.Printf("%s: %s\n", link.Type, link.URL)
}

// 按类型分组
byType := html.GroupLinksByType(links)
cssLinks := byType["css"]
jsLinks := byType["js"]
images := byType["image"]
```

### 获取阅读时间

```go
result, _ := html.Extract(htmlBytes)
minutes := result.ReadingTime.Minutes()
fmt.Printf("阅读时间: %.1f 分钟", minutes)
```

### 带 Context 的批处理（支持取消）

```go
package main

import (
    "context"
    "time"
    "github.com/cybergodev/html"
)

func main() {
    processor, _ := html.New()
    defer processor.Close()

    files := []string{"page1.html", "page2.html", "page3.html"}

    // 创建带超时的 context
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    // 支持取消的处理
    result := processor.ExtractBatchFilesWithContext(ctx, files)

    fmt.Printf("成功: %d, 失败: %d, 取消: %d\n",
        result.Success, result.Failed, result.Cancelled)
}
```

### 缓存与统计

```go
processor, _ := html.New()
defer processor.Close()

htmlBytes := []byte(`<html><body><p>内容</p></body></html>`)

// 自动启用缓存
processor.Extract(htmlBytes)
processor.Extract(htmlBytes) // 缓存命中！

// 查看性能统计
stats := processor.GetStatistics()
fmt.Printf("缓存命中: %d/%d\n", stats.CacheHits, stats.TotalProcessed)

// 清除缓存（保留统计数据）
processor.ClearCache()

// 重置统计（保留缓存条目）
processor.ResetStatistics()
```

---

## 🔧 API 快速参考

### 包级函数

```go
// 提取（从字节）
html.Extract(htmlBytes []byte, cfg ...Config) (*Result, error)
html.ExtractText(htmlBytes []byte, cfg ...Config) (string, error)

// 提取（从文件）
html.ExtractFromFile(filePath string, cfg ...Config) (*Result, error)
html.ExtractTextFromFile(filePath string, cfg ...Config) (string, error)

// 格式转换（从字节）
html.ExtractToMarkdown(htmlBytes []byte, cfg ...Config) (string, error)
html.ExtractToJSON(htmlBytes []byte, cfg ...Config) ([]byte, error)

// 格式转换（从文件）
html.ExtractToMarkdownFromFile(filePath string, cfg ...Config) (string, error)
html.ExtractToJSONFromFile(filePath string, cfg ...Config) ([]byte, error)

// 链接
html.ExtractAllLinks(htmlBytes []byte, cfg ...Config) ([]LinkResource, error)
html.ExtractAllLinksFromFile(filePath string, cfg ...Config) ([]LinkResource, error)
html.ExtractAllLinksWithContext(ctx context.Context, htmlBytes []byte, cfg ...Config) ([]LinkResource, error)
html.ExtractAllLinksFromFileWithContext(ctx context.Context, filePath string, cfg ...Config) ([]LinkResource, error)
html.GroupLinksByType(links []LinkResource) map[string][]LinkResource

// 批处理
html.ExtractBatch(htmlContents [][]byte, cfg ...Config) *BatchResult
html.ExtractBatchWithContext(ctx context.Context, htmlContents [][]byte, cfg ...Config) *BatchResult
html.ExtractBatchFiles(filePaths []string, cfg ...Config) *BatchResult
html.ExtractBatchFilesWithContext(ctx context.Context, filePaths []string, cfg ...Config) *BatchResult

// Context 感知提取
html.ExtractWithContext(ctx context.Context, htmlBytes []byte, cfg ...Config) (*Result, error)
html.ExtractFromFileWithContext(ctx context.Context, filePath string, cfg ...Config) (*Result, error)
html.ExtractTextWithContext(ctx context.Context, htmlBytes []byte, cfg ...Config) (string, error)
html.ExtractTextFromFileWithContext(ctx context.Context, filePath string, cfg ...Config) (string, error)
```

### Processor 方法

```go
// 创建
processor, err := html.New()
processor, err := html.New(config)
processor, err := html.New(html.HighSecurityConfig())
processor, err := html.New(html.TextOnlyConfig())
processor, err := html.New(html.MarkdownConfig())
defer processor.Close()

// 提取（从字节）
processor.Extract(htmlBytes []byte) (*Result, error)
processor.ExtractText(htmlBytes []byte) (string, error)
processor.ExtractWithContext(ctx context.Context, htmlBytes []byte) (*Result, error)
processor.ExtractTextWithContext(ctx context.Context, htmlBytes []byte) (string, error)

// 提取（从文件）
processor.ExtractFromFile(filePath string) (*Result, error)
processor.ExtractTextFromFile(filePath string) (string, error)
processor.ExtractFromFileWithContext(ctx context.Context, filePath string) (*Result, error)
processor.ExtractTextFromFileWithContext(ctx context.Context, filePath string) (string, error)

// 格式转换
processor.ExtractToMarkdown(htmlBytes []byte) (string, error)
processor.ExtractToJSON(htmlBytes []byte) ([]byte, error)
processor.ExtractToMarkdownFromFile(filePath string) (string, error)
processor.ExtractToJSONFromFile(filePath string) ([]byte, error)
processor.ExtractToMarkdownWithContext(ctx context.Context, htmlBytes []byte) (string, error)
processor.ExtractToMarkdownFromFileWithContext(ctx context.Context, filePath string) (string, error)
processor.ExtractToJSONWithContext(ctx context.Context, htmlBytes []byte) ([]byte, error)
processor.ExtractToJSONFromFileWithContext(ctx context.Context, filePath string) ([]byte, error)

// 链接
processor.ExtractAllLinks(htmlBytes []byte) ([]LinkResource, error)
processor.ExtractAllLinksFromFile(filePath string) ([]LinkResource, error)
processor.ExtractAllLinksWithContext(ctx context.Context, htmlBytes []byte) ([]LinkResource, error)
processor.ExtractAllLinksFromFileWithContext(ctx context.Context, filePath string) ([]LinkResource, error)

// 批处理
processor.ExtractBatch(htmlContents [][]byte) *BatchResult
processor.ExtractBatchWithContext(ctx context.Context, htmlContents [][]byte) *BatchResult
processor.ExtractBatchFiles(filePaths []string) *BatchResult
processor.ExtractBatchFilesWithContext(ctx context.Context, filePaths []string) *BatchResult

// 监控
processor.GetStatistics() Statistics
processor.ClearCache()
processor.ResetStatistics()
processor.GetAuditLog() []AuditEntry
processor.ClearAuditLog()
```

### 配置预设

```go
html.DefaultConfig() Config                  // 标准配置
html.HighSecurityConfig() Config             // 安全优化配置
html.TextOnlyConfig() Config                 // 仅文本（无媒体）
html.MarkdownConfig() Config                 // Markdown 图片格式
html.DefaultAuditConfig() AuditConfig        // 标准审计配置
html.HighSecurityAuditConfig() AuditConfig   // 安全优化审计配置
```

---

## 📋 结果结构

```go
type Result struct {
    Text           string        `json:"text"`
    Title          string        `json:"title"`
    Images         []ImageInfo   `json:"images,omitempty"`
    Links          []LinkInfo    `json:"links,omitempty"`
    Videos         []VideoInfo   `json:"videos,omitempty"`
    Audios         []AudioInfo   `json:"audios,omitempty"`
    ProcessingTime time.Duration `json:"-"` // 通过 MarshalJSON 序列化为 "processing_time_ms"
    WordCount      int           `json:"word_count"`
    ReadingTime    time.Duration `json:"-"` // 通过 MarshalJSON 序列化为 "reading_time_ms"
}

type ImageInfo struct {
    URL          string `json:"url"`
    Alt          string `json:"alt"`
    Title        string `json:"title"`
    Width        string `json:"width"`
    Height       string `json:"height"`
    IsDecorative bool   `json:"is_decorative"`
    Position     int    `json:"position"`
}

type LinkInfo struct {
    URL        string `json:"url"`
    Text       string `json:"text"`
    Title      string `json:"title"`
    IsExternal bool   `json:"is_external"`
    IsNoFollow bool   `json:"is_nofollow"`
    Position   int    `json:"position"`
}

type VideoInfo struct {
    URL      string `json:"url"`
    Type     string `json:"type"`
    Poster   string `json:"poster"`
    Width    string `json:"width"`
    Height   string `json:"height"`
    Duration string `json:"duration"`
}

type AudioInfo struct {
    URL      string `json:"url"`
    Type     string `json:"type"`
    Duration string `json:"duration"`
}

type LinkResource struct {
    URL   string
    Title string
    Type  string // "link", "image", "video", "audio", "css", "js", "icon", "media"
}

type NodeAttr struct {
    Key   string
    Value string
}

type BatchResult struct {
    Results    []*Result
    Errors     []error
    Success    int
    Failed     int
    Cancelled  int
}

type Statistics struct {
    TotalProcessed     int64
    CacheHits          int64
    CacheMisses        int64
    ErrorCount         int64
    AverageProcessTime time.Duration
}
```

---

## ⚙️ 配置参考

### Config 结构体

```go
type Config struct {
    // === 资源管理 ===
    MaxInputSize      int           // 最大 HTML 输入大小（默认：50MB）
    MaxCacheEntries   int           // 最大缓存条目数（默认：2000，0=禁用）
    CacheTTL          time.Duration // 缓存生存时间（默认：1 小时）
    CacheCleanup      time.Duration // 后台清理间隔（默认：5 分钟）
    WorkerPoolSize    int           // 批处理并发数（默认：4）
    ProcessingTimeout time.Duration // 最大处理时间（默认：30s，0=无超时）

    // === 安全 ===
    EnableSanitization bool        // HTML 净化（默认：true）
    MaxDepth           int         // 最大 HTML 嵌套深度（默认：500）
    AllowedBaseDir     string      // 限制文件读取到此目录（默认：空 = 不限制）
    Audit              AuditConfig // 安全审计日志

    // === 内容提取 ===
    ExtractArticle bool // 启用文章检测（默认：true）
    PreserveImages bool // 提取图片（默认：true）
    PreserveLinks  bool // 提取链接（默认：true）
    PreserveVideos bool // 提取视频（默认：true）
    PreserveAudios bool // 提取音频（默认：true）

    // === 输出格式 ===
    InlineImageFormat string // "none", "markdown", "html", "placeholder"
    InlineLinkFormat  string // "none", "markdown", "html"
    TableFormat       string // "markdown", "html"
    Encoding          string // 输入编码（空=自动检测）

    // === 链接提取 ===
    ResolveRelativeURLs  bool   // 解析相对 URL（默认：true）
    BaseURL              string // 解析基准 URL
    IncludeImages        bool   // 包含图片 URL（默认：true）
    IncludeVideos        bool   // 包含视频 URL（默认：true）
    IncludeAudios        bool   // 包含音频 URL（默认：true）
    IncludeCSS           bool   // 包含 CSS URL（默认：true）
    IncludeJS            bool   // 包含 JS URL（默认：true）
    IncludeContentLinks  bool   // 包含锚点链接（默认：true）
    IncludeExternalLinks bool   // 包含外部链接（默认：true）
    IncludeIcons         bool   // 包含图标 URL（默认：true）

    // === 扩展 ===
    Scorer Scorer // 可选的自定义内容评分器
}
```

### 默认配置值对比

| 设置 | 默认值 | 高安全配置 |
|------|--------|-----------|
| MaxInputSize | 50 MB | 10 MB |
| MaxCacheEntries | 2000 | 500 |
| CacheTTL | 1 小时 | 30 分钟 |
| CacheCleanup | 5 分钟 | 1 分钟 |
| WorkerPoolSize | 4 | 2 |
| ProcessingTimeout | 30s | 10s |
| MaxDepth | 500 | 100 |
| Audit | 禁用 | 启用 |

---

## 🔒 安全特性

### HTML 净化
- **危险标签移除**：`<script>`、`<style>`、`<noscript>`、`<iframe>`、`<embed>`、`<object>`、`<input>`、`<button>`、`<svg>`、`<math>`（`<form>` 容器会特意保留 —— 仅移除其中的表单控件）
- **事件处理器移除**：所有 `on*` 属性（onclick、onerror、onload 等）
- **危险协议阻止**：`javascript:`、`vbscript:`、`data:`（安全媒体类型除外）
- **XSS 防护**：全面的净化以防止跨站脚本攻击

### 输入验证
- **大小限制**：可配置的 `MaxInputSize` 防止内存耗尽
- **深度限制**：`MaxDepth` 防止深度嵌套 HTML 导致栈溢出
- **超时保护**：`ProcessingTimeout` 防止格式错误输入导致挂起
- **路径遍历防护**：`ExtractFromFile` 验证文件路径
- **基目录锁定**：`AllowedBaseDir` 将文件读取限制在受信任的目录内

### Data URL 安全
- **允许**：`data:image/*`、`data:font/*`、`data:application/pdf`
- **阻止**：`data:text/html`、`data:text/javascript`、`data:text/plain`

---

## 🔍 审计日志

启用审计日志以满足安全合规要求：

```go
// 方法 1：使用 HighSecurityConfig（默认启用审计）
processor, _ := html.New(html.HighSecurityConfig())

// 方法 2：自定义配置并启用审计
config := html.DefaultConfig()
config.Audit = html.AuditConfig{
    Enabled:            true,
    LogBlockedTags:     true,
    LogBlockedAttrs:    true,
    LogBlockedURLs:     true,
    LogInputViolations: true,
    LogDepthViolations: true,
    LogTimeouts:        true,
    LogEncodingIssues:  true,
    LogPathTraversal:   true,
}
processor, _ := html.New(config)

// 获取审计日志
entries := processor.GetAuditLog()
for _, entry := range entries {
    fmt.Printf("[%s] %s: %s\n", entry.Level, entry.EventType, entry.Message)
}
```

### 自定义审计输出目标

```go
// 将审计日志写入文件
file, _ := os.Create("audit.log")
fileSink := html.NewWriterAuditSink(file)

// 仅过滤关键事件
filteredSink := html.NewLevelFilteredSink(fileSink, html.AuditLevelCritical)

// 在配置中使用自定义输出目标
config := html.DefaultConfig()
config.Audit = html.AuditConfig{
    Enabled: true,
    Sink:    filteredSink,
}
processor, _ := html.New(config)
```

### 内置审计输出目标

| 输出目标 | 描述 |
|---------|------|
| `NewLoggerAuditSink()` | 写入 stderr，带 `[AUDIT]` 前缀 |
| `NewLoggerAuditSinkWithWriter(w)` | 写入自定义 io.Writer |
| `NewWriterAuditSink(w)` | 以 JSON 行格式写入 io.Writer |
| `NewChannelAuditSink(bufferSize)` | 发送到通道供外部处理 |
| `NewMultiSink(sinks...)` | 组合多个输出目标 |
| `NewFilteredSink(sink, filter)` | 写入前过滤条目 |
| `NewLevelFilteredSink(sink, level)` | 仅输出指定级别及以上的条目 |

---

## 📁 示例代码

完整的可运行示例请参见 [examples/](examples)：

| 示例 | 描述 |
|------|------|
| [01_quick_start.go](examples/01_quick_start.go) | 快速入门指南 |
| [02_content_extraction.go](examples/02_content_extraction.go) | 内容提取选项与输出格式 |
| [03_links_media.go](examples/03_links_media.go) | 链接与媒体提取 |
| [04_performance.go](examples/04_performance.go) | 性能优化与批处理 |
| [05_http_integration.go](examples/05_http_integration.go) | HTTP 集成模式 |
| [06_advanced_usage.go](examples/06_advanced_usage.go) | 自定义评分器、审计日志、安全配置 |
| [07_error_handling.go](examples/07_error_handling.go) | 错误处理模式 |
| [08_real_world.go](examples/08_real_world.go) | 实际应用案例 |
| [09_context_cancellation.go](examples/09_context_cancellation.go) | 上下文取消与超时 |
| [10_secure_file_processing.go](examples/10_secure_file_processing.go) | AllowedBaseDir 沙箱与安全文件读取 |
| [11_encoding.go](examples/11_encoding.go) | 字符编码检测与覆盖 |

---

## 🔄 兼容性

本库内部使用 `golang.org/x/net/html` 进行 HTML 解析，但**不**重新导出其类型或函数。它不是 `golang.org/x/net/html` 的直接替代品，而是提供更高级的内容提取 API。

```go
import "github.com/cybergodev/html"

// 内容提取 API
processor, _ := html.New(html.DefaultConfig())
defer processor.Close()

result, _ := processor.Extract(htmlBytes)
fmt.Println(result.Text)
```

详见 [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md) 获取完整的 API 参考和迁移指南。

---

## 🧵 线程安全

`Processor` 可安全并发使用：

```go
processor, _ := html.New()
defer processor.Close()

htmlBytes := []byte(`<html><body><p>内容</p></body></html>`)

var wg sync.WaitGroup
for i := 0; i < 100; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        processor.Extract(htmlBytes)
    }()
}
wg.Wait()
```

---

## 🔌 接口

本库提供接口，便于依赖注入和测试：

```go
// Extractor 组合了所有提取功能
type Extractor interface {
    // 内容提取（从字节和文件，支持可选 Context）
    Extract(htmlBytes []byte) (*Result, error)
    ExtractWithContext(ctx context.Context, htmlBytes []byte) (*Result, error)
    ExtractFromFile(filePath string) (*Result, error)
    ExtractFromFileWithContext(ctx context.Context, filePath string) (*Result, error)

    // 文本提取
    ExtractText(htmlBytes []byte) (string, error)
    ExtractTextFromFile(filePath string) (string, error)
    ExtractTextWithContext(ctx context.Context, htmlBytes []byte) (string, error)
    ExtractTextFromFileWithContext(ctx context.Context, filePath string) (string, error)

    // 输出格式
    ExtractToMarkdown(htmlBytes []byte) (string, error)
    ExtractToMarkdownFromFile(filePath string) (string, error)
    ExtractToJSON(htmlBytes []byte) ([]byte, error)
    ExtractToJSONFromFile(filePath string) ([]byte, error)
    ExtractToMarkdownWithContext(ctx context.Context, htmlBytes []byte) (string, error)
    ExtractToMarkdownFromFileWithContext(ctx context.Context, filePath string) (string, error)
    ExtractToJSONWithContext(ctx context.Context, htmlBytes []byte) ([]byte, error)
    ExtractToJSONFromFileWithContext(ctx context.Context, filePath string) ([]byte, error)

    // 批处理
    ExtractBatch(htmlContents [][]byte) *BatchResult
    ExtractBatchWithContext(ctx context.Context, htmlContents [][]byte) *BatchResult
    ExtractBatchFiles(filePaths []string) *BatchResult
    ExtractBatchFilesWithContext(ctx context.Context, filePaths []string) *BatchResult

    // 链接提取
    ExtractAllLinks(htmlBytes []byte) ([]LinkResource, error)
    ExtractAllLinksFromFile(filePath string) ([]LinkResource, error)
    ExtractAllLinksWithContext(ctx context.Context, htmlBytes []byte) ([]LinkResource, error)
    ExtractAllLinksFromFileWithContext(ctx context.Context, filePath string) ([]LinkResource, error)

    // 资源清理
    Close() error
}

// StatsProvider 用于监控和缓存管理
type StatsProvider interface {
    GetStatistics() Statistics
    ClearCache()
    ResetStatistics()
}

// Scorer 用于自定义内容评分算法
type Scorer interface {
    Score(node ContentNode) int
    ShouldRemove(node ContentNode) bool
}

// ContentNode 抽象 HTML 节点，用于自定义 Scorer
type ContentNode interface {
    Type() string
    Data() string
    AttrValue(key string) string
    Attrs() []NodeAttr
    FirstChild() ContentNode
    NextSibling() ContentNode
    Parent() ContentNode
}
```

`Processor` 在编译时实现了 `Extractor` 和 `StatsProvider` 接口。

---

## ❌ 错误处理

所有错误均可通过 `errors.Is()` 检查：

```go
result, err := html.Extract(htmlBytes)
if err != nil {
    switch {
    case errors.Is(err, html.ErrInputTooLarge):
        // 输入超过 MaxInputSize
    case errors.Is(err, html.ErrInvalidHTML):
        // HTML 格式错误
    case errors.Is(err, html.ErrProcessingTimeout):
        // 处理超时
    case errors.Is(err, html.ErrMaxDepthExceeded):
        // 嵌套深度超限
    case errors.Is(err, html.ErrFileNotFound):
        // 文件不存在
    case errors.Is(err, html.ErrInvalidFilePath):
        // 无效文件路径
    case errors.Is(err, html.ErrProcessorClosed):
        // Processor 已关闭
    case errors.Is(err, html.ErrInvalidConfig):
        // 无效配置
    case errors.Is(err, html.ErrMultipleConfigs):
        // 提供了多个 Config
    case errors.Is(err, html.ErrInternalPanic):
        // 内部 panic 已恢复
    }
}
```

---

## 📄 许可证

MIT 许可证 - 详见 [LICENSE](LICENSE) 文件。

---

如果这个项目对你有帮助，请给一个 Star! ⭐
