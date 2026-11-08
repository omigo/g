package g

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestLogger(level Level) (*Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return NewLogger(level, &buf), &buf
}

// 级别过滤：全局级别与 context 级别。
func TestLevelFiltering(t *testing.T) {
	l, buf := newTestLogger(Linfo)
	ctx := context.Background()

	l.Debug(ctx, "debug-should-filter")
	if strings.Contains(buf.String(), "debug-should-filter") {
		t.Fatalf("Debug 在 INFO 级别下不应输出: %s", buf.String())
	}

	l.Info(ctx, "info-should-output")
	if !strings.Contains(buf.String(), "info-should-output") {
		t.Fatalf("Info 在 INFO 级别下应输出")
	}

	// context 级别更详细时，全局过滤被绕过。
	debugCtx := l.WithLevel(ctx, Ldebug)
	l.Debug(debugCtx, "debug-with-context")
	if !strings.Contains(buf.String(), "debug-with-context") {
		t.Fatalf("context 级别应允许 Debug 输出")
	}
}

// Fatal 必须始终输出，并且退出行为可被拦截。
func TestFatalAlwaysOutputAndExitHook(t *testing.T) {
	l, buf := newTestLogger(Lstack) // 最低级别，普通日志全部过滤
	var exitCode int
	var exitCalled bool
	l.SetExit(func(code int) {
		exitCalled = true
		exitCode = code
	})
	ctx := context.Background()

	l.Fatal(ctx, "fatal-must-output")
	if !strings.Contains(buf.String(), "fatal-must-output") {
		t.Fatalf("Fatal 不受级别过滤，必须输出: %q", buf.String())
	}
	if !exitCalled || exitCode != 99 {
		t.Fatalf("SetExit 应收到退出码 99, called=%v code=%d", exitCalled, exitCode)
	}
}

// Stack 日志不应写入缓冲区里的零字节，且只打当前 goroutine。
func TestStackNoNULAndCurrentGoroutineOnly(t *testing.T) {
	l, buf := newTestLogger(Ltrace)
	ctx := context.Background()

	started := make(chan struct{})
	release := make(chan struct{})
	go func() {
		close(started)
		<-release
	}()
	<-started
	defer close(release)

	l.Stack(ctx, "boom")
	out := buf.Bytes()
	if bytes.ContainsRune(out, 0) {
		t.Fatalf("Stack 日志包含 NUL 字节，长度 %d", len(out))
	}
	if !bytes.Contains(out, []byte("goroutine")) {
		t.Fatalf("Stack 日志缺少调用栈内容: %q", string(out))
	}
	if bytes.Contains(out, []byte("TestStackNoNULAndCurrentGoroutineOnly.func1")) {
		t.Fatalf("Stack 打印了其他 goroutine 的栈，应只打当前 goroutine")
	}
	if len(out) >= 8192+64 {
		t.Fatalf("Stack 日志长度异常: %d", len(out))
	}
}

// 级别越界不再 panic。
func TestLevelStringOutOfRange(t *testing.T) {
	cases := map[Level]string{
		Level(-1):   "LEVEL(-1)",
		LevelLength: "LEVEL(7)",
		Level(100):  "LEVEL(100)",
		Linfo:       "INFO",
	}
	for level, want := range cases {
		if got := level.String(); got != want {
			t.Fatalf("Level(%d).String() = %q, want %q", int(level), got, want)
		}
	}
}

func TestGetCountOutOfRange(t *testing.T) {
	l, _ := newTestLogger(Linfo)
	if got := l.GetCount(Level(100)); got != 0 {
		t.Fatalf("GetCount(100) = %d, want 0", got)
	}
	if got := l.GetCount(Level(-1)); got != 0 {
		t.Fatalf("GetCount(-1) = %d, want 0", got)
	}
}

// 非法级别保持原值，不静默关闭全部日志。
func TestSetLevelInvalidKeepsOld(t *testing.T) {
	l, buf := newTestLogger(Lwarn)
	l.SetLevel(Level(99))
	if got := l.GetLevel(); got != Lwarn {
		t.Fatalf("SetLevel(99) 后级别 = %s, want WARN（非法值应保持原值）", got)
	}
	l.Error(context.Background(), "still-works")
	if !strings.Contains(buf.String(), "still-works") {
		t.Fatalf("非法 SetLevel 后日志仍应正常工作")
	}
}

func TestSetLevelStringTrimAndInvalid(t *testing.T) {
	l, _ := newTestLogger(Lwarn)
	l.SetLevelString(" info ")
	if got := l.GetLevel(); got != Linfo {
		t.Fatalf("SetLevelString(\" info \") 后级别 = %s, want INFO", got)
	}

	l.SetLevel(Ldebug)
	l.SetLevelString("NOT-A-LEVEL")
	if got := l.GetLevel(); got != Ldebug {
		t.Fatalf("非法字符串不应改变级别，当前 %s", got)
	}
}

// nil context 不应崩溃。
func TestNilContext(t *testing.T) {
	l, buf := newTestLogger(Linfo)
	var ctx context.Context
	l.Info(ctx, "nil-context")
	if !strings.Contains(buf.String(), "nil-context") {
		t.Fatalf("nil context 时应降级为 Background")
	}
	if !l.IsEnabled(ctx, Linfo) {
		t.Fatalf("nil context 时 IsEnabled 不应 panic")
	}
	if _, ok := l.WithLevel(ctx, Ldebug).Value(logLevelKey{}).(Level); !ok {
		t.Fatalf("WithLevel(nil) 应返回可用的 context")
	}
	if l.WithTraceId(ctx).Value(traceIdKey{}) == nil {
		t.Fatalf("WithTraceId(nil) 应生成 traceId")
	}
}

// 生产格式的时间戳必须是 [2006-01-02 15:04:05.000]，毫秒分隔符为点号。
func TestProductionTimestampFormat(t *testing.T) {
	l, buf := newTestLogger(Linfo)
	l.SetFormat(FormatProduction)
	l.Info(context.Background(), "hello")

	re := regexp.MustCompile(`^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}\] `)
	if !re.MatchString(buf.String()) {
		t.Fatalf("生产格式时间戳不正确: %q", buf.String())
	}
}

// typed nil 与 nil 统一输出 nil。
func TestTypedNil(t *testing.T) {
	l, buf := newTestLogger(Linfo)
	var p *int
	var m map[string]int
	var s []byte
	l.Info(context.Background(), nil, p, m, s)

	out := buf.String()
	if strings.Contains(out, "null") {
		t.Fatalf("typed nil 应输出 nil 而不是 null: %q", out)
	}
	if strings.Count(out, "nil") != 4 {
		t.Fatalf("应有 4 个 nil: %q", out)
	}
}

// SetOutput(nil) 不应导致后续日志崩溃。
func TestSetOutputNil(t *testing.T) {
	l, _ := newTestLogger(Linfo)
	l.SetOutput(nil)
	l.Info(context.Background(), "discard-me")
}

// 开发格式不带方括号。
func TestFormatSwitch(t *testing.T) {
	l, buf := newTestLogger(Linfo)
	l.SetFormat(FormatDevelop)
	l.Info(context.Background(), "dev")
	if strings.HasPrefix(buf.String(), "[") {
		t.Fatalf("开发格式不应以方括号开头: %q", buf.String())
	}
	buf.Reset()
	l.SetFormat(FormatProduction)
	l.Info(context.Background(), "prod")
	if !strings.HasPrefix(buf.String(), "[") {
		t.Fatalf("生产格式应以方括号开头: %q", buf.String())
	}
}

// 文件行号信息应指向真实调用点。
func TestGetFileLineMethod(t *testing.T) {
	file, line, method := getFileLineMethod(1)
	if !strings.Contains(file, "logger_test.go") {
		t.Fatalf("file = %q, 应包含 logger_test.go", file)
	}
	if line <= 0 {
		t.Fatalf("line = %d, 应为正数", line)
	}
	if !strings.Contains(method, "TestGetFileLineMethod") {
		t.Fatalf("method = %q, 应包含测试函数名", method)
	}
}

// 并发写日志：行数完整、无数据竞争。
func TestConcurrentLogging(t *testing.T) {
	l, buf := newTestLogger(Ltrace)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				l.Infof(ctx, "g%d-j%d", i, j)
			}
		}(i)
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1600 {
		t.Fatalf("期望 1600 行, 实际 %d 行", len(lines))
	}
	for _, line := range lines {
		if !strings.Contains(line, "g") {
			t.Fatalf("日志行损坏: %q", line)
		}
	}
}

// 并发切换级别不应产生数据竞争。
func TestConcurrentSetLevel(t *testing.T) {
	l, _ := newTestLogger(Linfo)
	ctx := context.Background()
	var stop int32
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		for atomic.LoadInt32(&stop) == 0 {
			l.SetLevel(Ldebug)
			l.SetLevel(Linfo)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			l.Info(ctx, "x")
			l.IsEnabled(ctx, Ldebug)
		}
		atomic.StoreInt32(&stop, 1)
	}()
	wg.Wait()
}

// WithLevel 传入非法级别时忽略 context 级别。
func TestWithLevelInvalid(t *testing.T) {
	l, buf := newTestLogger(Lerror)
	ctx := l.WithLevel(context.Background(), Level(99))
	// 非法级别被忽略，因此 INFO 仍被全局 ERROR 级别过滤。
	l.Info(ctx, "should-filter")
	if strings.Contains(buf.String(), "should-filter") {
		t.Fatalf("非法 context 级别应被忽略: %q", buf.String())
	}
}

// hlog 级别映射必须逐项正确。
func TestHertzLevelMap(t *testing.T) {
	want := []Level{Ltrace, Ldebug, Linfo, Lwarn, Lwarn, Lerror, Lfatal}
	origin := defaultLogger.GetLevel()
	defer defaultLogger.SetLevel(origin)

	for level := 0; level < len(want); level++ {
		defaultLogger.SetLevel(Linfo)
		HertzFullLogger{}.SetLevel(level)
		if got := defaultLogger.GetLevel(); got != want[level] {
			t.Fatalf("hlog level %d 映射为 %s, want %s", level, got, want[level])
		}
	}

	// 越界值收敛到边界，不 panic。
	HertzFullLogger{}.SetLevel(-1)
	if got := defaultLogger.GetLevel(); got != Ltrace {
		t.Fatalf("hlog level -1 应收敛到 TRACE, got %s", got)
	}
	HertzFullLogger{}.SetLevel(99)
	if got := defaultLogger.GetLevel(); got != Lfatal {
		t.Fatalf("hlog level 99 应收敛到 FATAL, got %s", got)
	}
}

// 计数只统计实际输出的日志。
func TestCount(t *testing.T) {
	l, _ := newTestLogger(Lwarn)
	ctx := context.Background()
	l.Info(ctx, "filtered")
	l.Warn(ctx, "kept")
	l.Error(ctx, "kept")

	if got := l.GetCount(Linfo); got != 0 {
		t.Fatalf("被过滤的 INFO 计数应为 0, got %d", got)
	}
	if got := l.GetCount(Lwarn); got != 1 {
		t.Fatalf("WARN 计数应为 1, got %d", got)
	}
	if got := l.GetCount(Lerror); got != 1 {
		t.Fatalf("ERROR 计数应为 1, got %d", got)
	}
}

// 输出侧基准，便于后续对比性能变化。
func BenchmarkFilteredLog(b *testing.B) {
	l := NewLogger(Linfo, &bytes.Buffer{})
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Debug(ctx, "filtered")
	}
}

func BenchmarkOutputLog(b *testing.B) {
	l := NewLogger(Linfo, &bytes.Buffer{})
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Infof(ctx, "hello %d", i)
	}
}

// 端到端调用点：输出里的文件名必须是调用方文件，而不是 logger.go 内部。
func TestLogCallSite(t *testing.T) {
	l, buf := newTestLogger(Linfo)
	l.SetFormat(FormatProduction)
	_, _, line, _ := runtime.Caller(0)
	l.Info(context.Background(), "site-check")

	re := regexp.MustCompile(`\[([^:]+):[^:]+:(\d+)\]`)
	m := re.FindStringSubmatch(buf.String())
	if m == nil {
		t.Fatalf("无法解析调用点: %q", buf.String())
	}
	if !strings.Contains(m[1], "logger_test.go") {
		t.Fatalf("调用点文件 = %q, 应为 logger_test.go", m[1])
	}
	gotLine, _ := strconv.Atoi(m[2])
	if gotLine != line+1 {
		t.Fatalf("调用点行号 = %d, want %d", gotLine, line+1)
	}
}

// NewLogger 收到非法级别时回退到 INFO，而不是静默关闭日志。
func TestNewLoggerInvalidLevelFallback(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(Level(99), &buf)
	if got := l.GetLevel(); got != Linfo {
		t.Fatalf("非法级别应回退到 INFO, got %s", got)
	}
	l.Info(context.Background(), "fallback-works")
	if !strings.Contains(buf.String(), "fallback-works") {
		t.Fatalf("回退后日志应输出")
	}
}

// SetExit(nil) 表示 Fatal 只打印、不退出。
func TestFatalNoExitHook(t *testing.T) {
	l, buf := newTestLogger(Linfo)
	l.SetExit(nil)
	l.Fatal(context.Background(), "no-exit")
	if !strings.Contains(buf.String(), "no-exit") {
		t.Fatalf("Fatal 应输出")
	}
}

// 非法 Format 值回退到生产格式，不 panic。
func TestFormatInvalidFallback(t *testing.T) {
	l, buf := newTestLogger(Linfo)
	l.SetFormat(Format(99))
	l.Info(context.Background(), "x")
	if !strings.HasPrefix(buf.String(), "[") {
		t.Fatalf("非法 Format 应回退生产格式: %q", buf.String())
	}
}

// SetTraceId 支持多种整数与字符串类型。
func TestSetTraceIdTypes(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		in   interface{}
		want string
	}{
		{uint64(7), "7"},
		{uint32(8), "8"},
		{int64(-9), "-9"},
		{int(10), "10"},
		{int32(11), "11"},
		{"abc", "abc"},
	}
	for _, c := range cases {
		got, _ := SetTraceId(ctx, c.in).Value(traceIdKey{}).(string)
		if got != c.want {
			t.Fatalf("SetTraceId(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// WithTraceId 发现已有 traceId 时保持原值。
func TestWithTraceIdKeepsExisting(t *testing.T) {
	ctx := SetTraceId(context.Background(), "keep-me")
	got, _ := WithTraceId(ctx).Value(traceIdKey{}).(string)
	if got != "keep-me" {
		t.Fatalf("已有 traceId 不应被覆盖, got %q", got)
	}
}

// 调用栈信息获取失败时返回兜底值，不 panic。
func TestGetFileLineMethodTooLargeSkip(t *testing.T) {
	file, line, method := getFileLineMethod(1000)
	if file != "?" || line != 0 || method != "?" {
		t.Fatalf("skip 过大应返回兜底值, got %q %d %q", file, line, method)
	}
}

// 开发格式下 Stack 同样不写 NUL。
func TestDevelopStackNoNUL(t *testing.T) {
	l, buf := newTestLogger(Ltrace)
	l.SetFormat(FormatDevelop)
	l.Stack(context.Background(), "dev-stack")
	if bytes.ContainsRune(buf.Bytes(), 0) {
		t.Fatalf("开发格式 Stack 含 NUL")
	}
}

// 包级 GetCountAll 返回按级别的计数数组。
func TestGetCountAll(t *testing.T) {
	l, _ := newTestLogger(Ltrace)
	ctx := context.Background()
	l.Info(ctx, "a")
	l.Error(ctx, "b")
	all := l.GetCountAll()
	if all[Linfo] != 1 || all[Lerror] != 1 {
		t.Fatalf("GetCountAll 计数不正确: %v", all)
	}
}

// ---------- 复核补充的回归测试 ----------

type panicWriter struct{ panicked int32 }

func (w *panicWriter) Write(p []byte) (int, error) {
	if atomic.CompareAndSwapInt32(&w.panicked, 0, 1) {
		panic("writer panic")
	}
	return len(p), nil
}

// Writer panic 后不能把锁永久泄漏，否则后续日志全部阻塞。
func TestWriterPanicReleasesLock(t *testing.T) {
	l := NewLogger(Linfo, &panicWriter{})
	func() {
		defer func() { _ = recover() }()
		l.Info(context.Background(), "boom")
	}()

	done := make(chan struct{})
	go func() {
		l.Info(context.Background(), "after")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Writer panic 后锁未释放，后续日志阻塞")
	}
}

type levelWriter struct {
	l   *Logger
	buf bytes.Buffer
}

func (w *levelWriter) Write(p []byte) (int, error) {
	_ = w.l.GetLevel() // 模拟 Writer 回调 Logger 的读方法
	return w.buf.Write(p)
}

// Writer 回调 Logger 的读方法不应死锁（状态锁与输出锁必须分开）。
func TestWriterCallbackGetLevelNoDeadlock(t *testing.T) {
	w := &levelWriter{}
	l := NewLogger(Linfo, w)
	w.l = l

	done := make(chan struct{})
	go func() {
		l.Info(context.Background(), "callback")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Writer 回调 GetLevel 死锁")
	}
}

// 并发切换格式化器、输出与退出行为不应有数据竞争。
func TestConcurrentSetFormatOutputExit(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(Linfo, &buf)
	ctx := context.Background()
	var stop int32
	var wg sync.WaitGroup

	wg.Add(4)
	go func() {
		defer wg.Done()
		for atomic.LoadInt32(&stop) == 0 {
			l.SetFormat(FormatProduction)
			l.SetFormat(FormatDevelop)
		}
	}()
	go func() {
		defer wg.Done()
		for atomic.LoadInt32(&stop) == 0 {
			l.SetOutput(&buf)
		}
	}()
	go func() {
		defer wg.Done()
		for atomic.LoadInt32(&stop) == 0 {
			l.SetExit(nil)
			l.SetExit(func(int) {})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			l.Info(ctx, "x")
		}
		atomic.StoreInt32(&stop, 1)
	}()
	wg.Wait()
}

// Cost/Costf 的调用点也应指向调用方（skip 的第二条路径）。
func TestCostCallSite(t *testing.T) {
	l, buf := newTestLogger(Linfo)
	l.SetFormat(FormatProduction)
	done := l.Cost(context.Background(), "cost-check")
	done()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("Cost 应输出 start 与 cost 两行, 实际 %d 行: %q", len(lines), buf.String())
	}
	for _, line := range lines {
		if !strings.Contains(line, "logger_test.go") {
			t.Fatalf("Cost 调用点应为 logger_test.go, 实际: %q", line)
		}
	}
}

type myErr struct{}

func (*myErr) Error() string { return "myErr" }

// typed nil error 统一输出 nil；真实 error 仍输出消息。
func TestTypedNilError(t *testing.T) {
	l, buf := newTestLogger(Linfo)
	var e *myErr
	var err error = e
	l.Info(context.Background(), err)
	if !strings.Contains(buf.String(), "nil") {
		t.Fatalf("typed nil error 应输出 nil: %q", buf.String())
	}

	buf.Reset()
	l.Info(context.Background(), &myErr{})
	if !strings.Contains(buf.String(), "myErr") {
		t.Fatalf("真实 error 应输出消息: %q", buf.String())
	}
}

// float32 使用 32 位精度格式化。
func TestFloat32Format(t *testing.T) {
	l, buf := newTestLogger(Linfo)
	l.Info(context.Background(), float32(1.1))
	if !strings.Contains(buf.String(), "1.1") {
		t.Fatalf("float32 输出异常: %q", buf.String())
	}
}

// Fatal 默认行为：输出后再以 99 退出。用子进程验证，避免测试进程真的退出。
func TestFatalDefaultExitCode(t *testing.T) {
	if os.Getenv("G_FATAL_HELPER") == "1" {
		l := NewLogger(Linfo, os.Stdout)
		l.Fatal(context.Background(), "fatal-marker")
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestFatalDefaultExitCode")
	cmd.Env = append(os.Environ(), "G_FATAL_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("Fatal 应导致进程退出，实际正常返回: %s", out)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("Fatal 未以退出码结束: %v", err)
	}
	if exitErr.ExitCode() != 99 {
		t.Fatalf("退出码 = %d, want 99", exitErr.ExitCode())
	}
	if !strings.Contains(string(out), "fatal-marker") {
		t.Fatalf("Fatal 输出缺失: %s", out)
	}
}
