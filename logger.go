package g

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Format 表示日志输出格式。
type Format int

const (
	// FormatProduction 生产格式：[时间] [文件:方法:行] [级别] [traceId] 消息
	FormatProduction Format = iota
	// FormatDevelop 开发格式：时间 traceId 级别 文件:行 方法 消息
	FormatDevelop
)

var (
	bufferPool    = sync.Pool{New: func() interface{} { return bytes.NewBuffer(make([]byte, 0, 1024)) }}
	defaultFormat = detectFormat()
)

// detectFormat 根据环境变量选择默认输出格式。
// 优先读 G_LOG_FORMAT，其次兼容旧的 ENVIRONMENT。
func detectFormat() Format {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("G_LOG_FORMAT")))
	if env == "" {
		env = strings.ToLower(strings.TrimSpace(os.Getenv("ENVIRONMENT")))
	}
	switch env {
	case "dev", "develop", "development":
		return FormatDevelop
	case "prod", "production":
		return FormatProduction
	}
	return FormatProduction
}

// normalizeCtx 把 nil context 归一化为 Background，避免日志调用因误传 nil 崩溃。
func normalizeCtx(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// Logger 是一个按 context 级别过滤的日志器。
//
// level、buffer 等状态字段不导出，避免外部绕过锁直接修改造成数据竞争；
// 请使用 GetLevel、SetLevel、SetOutput 等方法访问。
type Logger struct {
	stateMu  sync.RWMutex // 保护 level、formatter、exit
	outputMu sync.Mutex   // 保护 buffer、count；与 stateMu 分开，避免 Writer 回调 Logger 时死锁

	level     Level
	buffer    *bufio.Writer
	count     [LevelLength]uint64
	formatter formatter
	exit      func(int)

	WithLevel   func(ctx context.Context, level Level) context.Context
	WithTraceId func(ctx ...context.Context) context.Context
	SetTraceId  func(ctx context.Context, traceId interface{}) context.Context

	Fatal, Stack, Error, Warn, Info, Debug, Trace        func(ctx context.Context, msg ...interface{})
	Fatalf, Stackf, Errorf, Warnf, Infof, Debugf, Tracef func(ctx context.Context, format string, msg ...interface{})

	Cost  func(ctx context.Context, msg ...interface{}) func()
	Costf func(ctx context.Context, format string, msg ...interface{}) func()
}

func (l *Logger) GetCount(level Level) uint64 {
	if !validLevel(level) {
		return 0
	}
	l.outputMu.Lock()
	defer l.outputMu.Unlock()
	return l.count[level]
}
func (l *Logger) GetCountAll() [LevelLength]uint64 {
	l.outputMu.Lock()
	defer l.outputMu.Unlock()
	return l.count
}

func logout(logger *Logger, level Level) func(ctx context.Context, msg ...interface{}) {
	return func(ctx context.Context, msg ...interface{}) {
		if logger.check(ctx, level) {
			logger.output(ctx, level, 4, "", msg)
		}
	}
}

func logoutf(logger *Logger, level Level) func(ctx context.Context, format string, msg ...interface{}) {
	return func(ctx context.Context, format string, msg ...interface{}) {
		if logger.check(ctx, level) {
			logger.output(ctx, level, 4, format, msg)
		}
	}
}

// cost 暂不纳入级别控制，保持原有行为，见 REVIEW.md。
func cost(logger *Logger, level Level) func(ctx context.Context, msg ...interface{}) func() {
	return func(ctx context.Context, msg ...interface{}) func() {
		s := append(msg, "start...")
		logger.output(ctx, level, 4, "", s)
		start := time.Now()
		return func() {
			e := append(msg, "cost", time.Since(start).Truncate(time.Millisecond).String())
			logger.output(ctx, level, 4, "", e)
		}
	}
}

// costf 暂不纳入级别控制，保持原有行为，见 REVIEW.md。
func costf(logger *Logger, level Level) func(ctx context.Context, format string, msg ...interface{}) func() {
	return func(ctx context.Context, format string, msg ...interface{}) func() {
		logger.output(ctx, level, 4, format+" start...", msg)
		start := time.Now()
		return func() {
			logger.output(ctx, level, 4, format+" cost "+time.Since(start).Truncate(time.Millisecond).String(), msg)
		}
	}
}

func NewLogger(level Level, writer io.Writer) *Logger {
	if !validLevel(level) {
		fmt.Fprintf(os.Stderr, "g: invalid log level %d, fallback to %s\n", int(level), Linfo)
		level = Linfo
	}
	if writer == nil {
		writer = io.Discard
	}

	newLogger := &Logger{
		level:     level,
		buffer:    bufio.NewWriter(writer),
		formatter: formatterOf(defaultFormat),
		exit:      os.Exit,
	}

	newLogger.WithLevel = WithLevel
	newLogger.WithTraceId = WithTraceId
	newLogger.SetTraceId = SetTraceId

	newLogger.Fatal = logout(newLogger, Lfatal)
	newLogger.Stack = logout(newLogger, Lstack)
	newLogger.Error = logout(newLogger, Lerror)
	newLogger.Warn = logout(newLogger, Lwarn)
	newLogger.Info = logout(newLogger, Linfo)
	newLogger.Debug = logout(newLogger, Ldebug)
	newLogger.Trace = logout(newLogger, Ltrace)
	newLogger.Cost = cost(newLogger, Linfo)

	newLogger.Fatalf = logoutf(newLogger, Lfatal)
	newLogger.Stackf = logoutf(newLogger, Lstack)
	newLogger.Errorf = logoutf(newLogger, Lerror)
	newLogger.Warnf = logoutf(newLogger, Lwarn)
	newLogger.Infof = logoutf(newLogger, Linfo)
	newLogger.Debugf = logoutf(newLogger, Ldebug)
	newLogger.Tracef = logoutf(newLogger, Ltrace)
	newLogger.Costf = costf(newLogger, Linfo)

	return newLogger
}

func (l *Logger) IsEnabled(ctx context.Context, level Level) bool {
	return l.check(ctx, level)
}

func (l *Logger) GetLevel() Level {
	l.stateMu.RLock()
	defer l.stateMu.RUnlock()
	return l.level
}

// SetLevel 设置日志级别。非法级别会保持原值并输出警告。
func (l *Logger) SetLevel(level Level) {
	if !validLevel(level) {
		fmt.Fprintf(os.Stderr, "g: invalid log level %d, keep %s\n", int(level), l.GetLevel())
		return
	}
	l.stateMu.Lock()
	l.level = level
	l.stateMu.Unlock()
}

// SetLevelString 设置日志级别，大小写不敏感，自动去除首尾空白。
func (l *Logger) SetLevelString(level string) {
	level = strings.ToUpper(strings.TrimSpace(level))
	var lvl = -1
	for i, v := range levelStrings {
		if v == level {
			lvl = i
			break
		}
	}
	if lvl == -1 {
		fmt.Fprintf(os.Stderr, "g: invalid log level string %q, keep %s\n", level, l.GetLevel())
		return
	}
	l.SetLevel(Level(lvl))
}

// SetOutput 设置输出目标。传 nil 时将丢弃日志而不是崩溃。
func (l *Logger) SetOutput(writer io.Writer) {
	if writer == nil {
		writer = io.Discard
	}
	l.outputMu.Lock()
	l.buffer = bufio.NewWriter(writer)
	l.outputMu.Unlock()
}

// SetFormat 切换输出格式。
func (l *Logger) SetFormat(format Format) {
	l.stateMu.Lock()
	l.formatter = formatterOf(format)
	l.stateMu.Unlock()
}

// SetExit 设置 Fatal 之后的退出行为。默认是 os.Exit。
// 传入 nil 表示只打印日志、不退出进程；传空函数可以拦截退出。
// 注意：Fatal 不受日志级别过滤，只要调用就一定会输出。
func (l *Logger) SetExit(exit func(int)) {
	l.stateMu.Lock()
	l.exit = exit
	l.stateMu.Unlock()
}

type logLevelKey struct{}
type traceIdKey struct{}

func (l *Logger) check(ctx context.Context, level Level) bool {
	// Fatal 必须始终输出，避免级别配置错误导致致命错误被静默吞掉。
	if level == Lfatal {
		return true
	}
	ctx = normalizeCtx(ctx)
	if v := ctx.Value(logLevelKey{}); v != nil {
		if special, ok := v.(Level); ok {
			return special >= level
		}
	}
	l.stateMu.RLock()
	defer l.stateMu.RUnlock()
	return l.level >= level
}

func (l *Logger) output(ctx context.Context, level Level, skip int, format string, msg []interface{}) {
	ctx = normalizeCtx(ctx)

	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	l.stateMu.RLock()
	writeFn := l.formatter
	l.stateMu.RUnlock()
	writeFn(ctx, buf, level, skip, format, msg)

	// 输出锁用局部作用域 + defer 解锁：即使用户 Writer panic，锁也会释放，
	// 不会让后续所有日志永久阻塞。Writer 内部不要再回调同一个 Logger 打日志。
	func() {
		l.outputMu.Lock()
		defer l.outputMu.Unlock()
		_, _ = l.buffer.Write(buf.Bytes())
		_ = l.buffer.Flush()
		if validLevel(level) {
			l.count[level]++
		}
	}()

	// 退出动作放到锁外，避免持锁执行用户回调或 os.Exit。
	if level == Lfatal {
		l.stateMu.RLock()
		exit := l.exit
		l.stateMu.RUnlock()
		if exit != nil {
			exit(99)
		}
	}
}

const (
	whitespace   = ' '
	leftBracket  = '['
	rightBracket = ']'
)

// formatter 是日志格式化函数类型，直接以函数值保存到 Logger，
// 避免在 output 与具体实现之间多一层调用帧，导致文件行号偏移。
type formatter func(ctx context.Context, buf *bytes.Buffer, level Level, skip int, format string, msg []interface{})

func formatterOf(f Format) formatter {
	if f == FormatDevelop {
		return writeDevelop
	}
	return writeOther
}

func writeOther(ctx context.Context, buf *bytes.Buffer, level Level, skip int, format string, msg []interface{}) {
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	buf.WriteByte(leftBracket)
	buf.WriteString(ts)
	buf.WriteByte(rightBracket)

	buf.WriteByte(whitespace)

	file, line, method := callerInfo(skip)
	buf.WriteByte(leftBracket)
	buf.WriteString(file)
	buf.WriteByte(':')
	buf.WriteString(method)
	buf.WriteByte(':')
	buf.WriteString(strconv.Itoa(line))
	buf.WriteByte(rightBracket)

	buf.WriteByte(whitespace)

	buf.WriteByte(leftBracket)
	buf.WriteString(level.String())
	buf.WriteByte(rightBracket)

	buf.WriteByte(whitespace)

	traceId := getTraceId(ctx)
	buf.WriteByte(leftBracket)
	buf.WriteString(traceId)
	buf.WriteByte(rightBracket)

	buf.WriteByte(whitespace)
	writeMessage(buf, format, msg)

	if level == Lstack {
		writeStack(buf)
	}
}

func writeDevelop(ctx context.Context, buf *bytes.Buffer, level Level, skip int, format string, msg []interface{}) {
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	buf.WriteString(ts)
	buf.WriteByte(whitespace)

	traceId := getTraceId(ctx)
	buf.WriteString(traceId)
	buf.WriteByte(whitespace)

	buf.WriteString(level.String())
	buf.WriteByte(whitespace)

	file, line, method := callerInfo(skip)
	buf.WriteString(file)
	buf.WriteByte(':')
	buf.WriteString(strconv.Itoa(line))
	buf.WriteByte(whitespace)

	buf.WriteString(method)
	buf.WriteByte(whitespace)

	writeMessage(buf, format, msg)

	if level == Lstack {
		writeStack(buf)
	}
}

func writeMessage(buf *bytes.Buffer, format string, msg []interface{}) {
	if format == "" {
		for _, v := range msg {
			buf.WriteByte(whitespace)
			writeValue(buf, v)
		}
	} else {
		buf.WriteByte(whitespace)
		fmt.Fprintf(buf, format, msg...)
	}
	buf.WriteByte('\n')
}

// writeStack 只输出当前 goroutine 的调用栈，并按 runtime.Stack 返回的实际长度写入，
// 避免把缓冲区里未使用的零字节一并写进日志。
func writeStack(buf *bytes.Buffer) {
	const stackSize = 8192
	stack := make([]byte, stackSize)
	n := runtime.Stack(stack, false)
	buf.Write(stack[:n])
	buf.WriteByte('\n')
}

var lineSpaces = regexp.MustCompile(`\s*\n\s*`)

func writeValue(buf *bytes.Buffer, v interface{}) {
	if v == nil {
		buf.WriteString("nil")
		return
	}

	// typed nil（如 (*T)(nil)、nil map、nil slice）统一输出 nil，
	// 避免一部分输出 "null"、一部分输出 "nil" 造成歧义。
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		if rv.IsNil() {
			buf.WriteString("nil")
			return
		}
	}

	switch vv := v.(type) {
	case error:
		buf.WriteString(vv.Error())
	case []byte:
		buf.Write(vv)
	case string:
		buf.WriteString(lineSpaces.ReplaceAllString(vv, " "))
	case int:
		buf.WriteString(strconv.Itoa(vv))
	case int8:
		buf.WriteString(strconv.FormatInt(int64(vv), 10))
	case int16:
		buf.WriteString(strconv.FormatInt(int64(vv), 10))
	case int32:
		buf.WriteString(strconv.FormatInt(int64(vv), 10))
	case int64:
		buf.WriteString(strconv.FormatInt(vv, 10))
	case uint:
		buf.WriteString(strconv.FormatUint(uint64(vv), 10))
	case uint8:
		buf.WriteString(strconv.FormatUint(uint64(vv), 10))
	case uint16:
		buf.WriteString(strconv.FormatUint(uint64(vv), 10))
	case uint32:
		buf.WriteString(strconv.FormatUint(uint64(vv), 10))
	case uint64:
		buf.WriteString(strconv.FormatUint(vv, 10))
	case bool:
		buf.WriteString(strconv.FormatBool(vv))
	case float32:
		buf.WriteString(strconv.FormatFloat(float64(vv), 'g', 3, 32))
	case float64:
		buf.WriteString(strconv.FormatFloat(vv, 'g', 3, 64))
	case interface{ String() string }:
		buf.WriteString(vv.String())
	default:
		if js, err := json.Marshal(v); err != nil {
			fmt.Fprint(buf, v)
		} else {
			buf.Write(js)
		}
	}
}

func getTraceId(ctx context.Context) string {
	ctx = normalizeCtx(ctx)
	if v := ctx.Value(traceIdKey{}); v != nil {
		if traceId, ok := v.(string); ok {
			return traceId
		}
	}

	return "-"
}

// skipAuto 表示自动跳过日志库自身以及 hertz hlog 适配层的栈帧。
const skipAuto = -1

// libraryDir 是日志库自身所在目录，用于识别内部栈帧。
var libraryDir = func() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}()

const hertzHlogPkg = "cloudwego/hertz/pkg/common/hlog"

func callerInfo(skip int) (string, int, string) {
	if skip == skipAuto {
		return getCallerOutsidePackage()
	}
	// +1 补偿 callerInfo 自身这一层，保持调用方传入的 skip 语义不变。
	return getFileLineMethod(skip + 1)
}

func getFileLineMethod(skip int) (string, int, string) {
	pc, path, line, ok := runtime.Caller(skip)
	if !ok {
		return "?", 0, "?"
	}
	return formatCaller(pc, path, line)
}

// getCallerOutsidePackage 从调用栈中找到第一个不属于本库、也不属于 hlog 适配层的帧，
// 因此不依赖固定的 skip 层数；hlog 包级函数与 hlog.DefaultLogger() 两种调用方式都能定位到用户代码。
func getCallerOutsidePackage() (string, int, string) {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(2, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if filepath.Dir(frame.File) == libraryDir || strings.Contains(frame.Function, hertzHlogPkg) {
			if !more {
				break
			}
			continue
		}
		return formatCaller(frame.PC, frame.File, frame.Line)
	}
	return "?", 0, "?"
}

func formatCaller(pc uintptr, path string, line int) (string, int, string) {
	path = filepath.ToSlash(path)
	if parts := strings.Split(path, "/"); len(parts) > 3 {
		path = strings.Join(parts[len(parts)-3:], "/")
	}

	method := "?"
	if fn := runtime.FuncForPC(pc); fn != nil {
		method = fn.Name()
		if i := strings.LastIndex(method, "."); i > -1 {
			method = method[i+1:]
		}
	}
	return path, line, method
}
