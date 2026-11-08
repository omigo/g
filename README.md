G - Log for Golang
-------

垂直打印日志，不同于 Java 配置某个类日志级别这种水平切面的日志组件，`g` 的日志级别是跟着上下文的，根据规则从入口到出口打印一次请求内所产生的日志。

例如，在 level=info 的生产环境中，把 ID=123 的用户行为日志 level 设置成 debug，从 HTTP Request 到 Service 再到 DAO，所有 debug 日志全部打出来，以便调试或分析用户行为。

*** 实现方式

在入口处，根据规则，把 日志级别 设置在 Context 中，打印日志时，优先判断 Context 中的日志级别，决定是否输出该条日志。

## 安装

```bash
go get github.com/omigo/g
```

要求 Go 1.18 及以上。

## 快速开始

```go
package main

import (
	"context"

	"github.com/omigo/g"
)

func main() {
	ctx := g.WithTraceId(context.Background()) // 生成 traceId 并放进 ctx

	g.Info(ctx, "服务启动", 8080)                 // 普通日志
	g.Debugf(ctx, "调试信息 %d", 42)
	doWork(ctx)
}

func doWork(ctx context.Context) {
	defer g.Cost(ctx, "doWork")() // 打印 start 与 cost

	// 只对这一次请求打开 DEBUG
	ctx = g.WithLevel(ctx, g.Ldebug)
	g.Debug(ctx, "这条日志在全局 INFO 下也会输出")
}
```

## 级别

| 常量 | 值 | 说明 |
|---|---|---|
| Lstack | 0 | 打印堆栈 |
| Lfatal | 1 | 致命错误 |
| Lerror | 2 | 错误 |
| Lwarn | 3 | 警告 |
| Linfo | 4 | 常规信息 |
| Ldebug | 5 | 调试 |
| Ltrace | 6 | 最详细 |

注意：数值越大表示越详细，与 zap、slog 等库的方向相反。比较级别时不要想当然。

默认级别是 INFO。用法：

```go
g.SetLevel(g.Ldebug)       // 代码设置
g.SetLevelString("debug")  // 字符串设置，大小写不敏感，自动去首尾空白
```

非法级别不会生效，会保持原值并向 stderr 输出一条警告。

## 按上下文调整级别

`WithLevel` 把级别写进 context，优先级高于全局级别；`ctx` 中的级别只影响该请求链路：

```go
ctx = g.WithLevel(ctx, g.Ldebug)
```

## traceId

```go
ctx := g.WithTraceId(ctx)              // 没有则生成，已有则沿用
ctx = g.SetTraceId(ctx, "request-123") // 使用调用方生成的 ID
```

`WithTraceId` 的 traceId 起点使用随机数，进程内单调递增。跨进程唯一性由入口（网关或调用方）通过 `SetTraceId` 保证。

## 耗时统计

```go
defer g.Cost(ctx, "method", "arg")()
defer g.Costf(ctx, "method%d", 2)()
```

`Cost` 与 `Costf` 目前不受级别过滤控制，无论级别如何都会输出，这一点与普通日志不同，后续版本会修正。

## 输出格式

默认由环境变量决定：优先读 `G_LOG_FORMAT`，没有设置时兼容旧的 `ENVIRONMENT`。值为 dev、develop、development 时使用开发格式，prod、production 或未设置时使用生产格式。也可以在运行时切换：

```go
g.SetFormat(g.FormatDevelop)
g.SetFormat(g.FormatProduction)
```

生产格式示例：

```
[2026-11-08 16:21:50.050] [g/example/main.go:main:22] [INFO] [14077769] 消息
```

开发格式示例：

```
2026-11-08 16:21:50.050 14077769 INFO g/example/main.go:22 main  消息
```

## hertz 接入

```go
type MyLogger struct {
	g.HertzFullLogger
}

func (l MyLogger) SetLevel(level hlog.Level) {
	l.HertzFullLogger.SetLevel(int(level))
}

func init() {
	hlog.SetLogger(&MyLogger{})
}
```

说明：

- 不引入 hertz 依赖，所以 `HertzFullLogger.SetLevel` 接收 int，直接注册需要上面的薄包装。
- `Trace`、`Debug` 等不带 ctx 的方法无法带 traceId，日志中显示为 `-`；需要链路 traceId 时请使用 `CtxXxxf` 系列。
- `HertzFullLogger` 的调用点会自动跳过库内帧与 hlog 适配层，`hlog.Info` 包级函数和 `hlog.DefaultLogger().Info()` 两种调用方式都能定位到用户代码。
- 本包没有 NOTICE 档，hlog 的 notice 归入 warn，以保证 hlog 级别设为 Notice 时 Info 日志被正确过滤；副作用是 Warn 阈值下 Notice 仍会输出。
- `HertzFullLogger.SetLevel` 直接修改本包默认 Logger，`hlog.SetLevel` 与 `g.SetLevel` 会互相影响，接入后请只在一处设置级别。
- hlog 默认级别是 Trace，接入后会受本包默认 INFO 级别过滤，需要调试日志请显式调低级别。

## 行为约定

- 默认级别是 INFO。
- `Fatal` 一定输出，不受级别过滤；默认在输出后执行 `os.Exit(99)`。
- 不希望库退出进程时，可以替换退出行为：

```go
g.SetExit(nil)                 // 只打印，不退出
g.SetExit(func(code int) {})   // 拦截退出码，自行处理
```

- `Stack` 只打印当前 goroutine 的调用栈，并按实际长度写入，不会带出缓冲区的零字节。
- 传入 nil context 会按 `context.Background()` 处理，不会 panic。
- 非法级别不会导致 panic，`Level.String()` 返回 LEVEL(n)。
- 输出目标传 nil 时日志会被丢弃，不会崩溃。
- typed nil（nil 指针、nil map、nil slice 等）统一输出 nil；`SetTraceId(ctx, nil)` 把 traceId 记为 `-`。
- 自定义 Writer 不要在 Write 内部回调同一个 Logger 打日志（会自死锁）；读取 `GetLevel` 等状态方法是安全的。

## 从 v0.5.4 升级

- `Logger.Level`、`Logger.Buffer` 字段已私有化，改用 `GetLevel`、`SetLevel`、`SetOutput`。
- 默认级别从 DEBUG 改为 INFO，生产环境需要 DEBUG 时请显式设置。
- `Fatal` 默认在输出后执行 `os.Exit(99)`；需要拦截时使用 `SetExit`。
- `Stack` 不再打印所有 goroutine，只打印当前 goroutine。
- hertz 级别映射修正为逐项对应：之前 debug、info 两档存在错位，notice 现在归入 warn。
- 输出格式可以通过 `SetFormat` 在运行时切换；环境变量优先读 `G_LOG_FORMAT`。
- 日志中的级别名固定为大写（STACK、FATAL、ERROR、WARN、INFO、DEBUG、TRACE），`SetLevelString` 输入大小写不敏感。
- typed nil 统一输出 nil（旧版本输出 null 或 <nil>）。
- 非法级别、非法格式串等配置错误会输出 stderr 警告，不再静默。
