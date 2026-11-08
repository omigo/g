package g

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

type Level int

// String 返回级别名称。非法级别不会 panic，而是返回 LEVEL(n)。
func (l Level) String() string {
	if !validLevel(l) {
		return "LEVEL(" + strconv.Itoa(int(l)) + ")"
	}
	return levelStrings[l]
}

// validLevel 判断级别是否在合法范围内。
func validLevel(level Level) bool {
	return level >= Lstack && level < LevelLength
}

const (
	Lstack Level = iota
	Lfatal
	Lerror
	Lwarn
	Linfo
	Ldebug
	Ltrace
	LevelLength
)

var levelStrings = []string{"STACK", "FATAL", "ERROR", "WARN", "INFO", "DEBUG", "TRACE"}

func GetLevel() Level                                 { return defaultLogger.GetLevel() }
func SetLevel(level Level)                            { defaultLogger.SetLevel(level) }
func SetLevelString(level string)                     { defaultLogger.SetLevelString(level) }
func SetOutput(writer io.Writer)                      { defaultLogger.SetOutput(writer) }
func SetFormat(format Format)                         { defaultLogger.SetFormat(format) }
func SetExit(exit func(int))                          { defaultLogger.SetExit(exit) }
func GetCount(level Level) uint64                     { return defaultLogger.GetCount(level) }
func GetCountAll() [LevelLength]uint64                { return defaultLogger.GetCountAll() }
func IsEnabled(ctx context.Context, level Level) bool { return defaultLogger.IsEnabled(ctx, level) }

// WithLevel 把日志级别写入 context。非法级别会被忽略并输出警告。
func WithLevel(ctx context.Context, level Level) context.Context {
	ctx = normalizeCtx(ctx)
	if !validLevel(level) {
		fmt.Fprintf(os.Stderr, "g: invalid log level %d, ignore context level\n", int(level))
		return ctx
	}
	return context.WithValue(ctx, logLevelKey{}, level)
}

var (
	Fatal = logout(defaultLogger, Lfatal)
	Stack = logout(defaultLogger, Lstack)
	Error = logout(defaultLogger, Lerror)
	Warn  = logout(defaultLogger, Lwarn)
	Info  = logout(defaultLogger, Linfo)
	Debug = logout(defaultLogger, Ldebug)
	Trace = logout(defaultLogger, Ltrace)
	Cost  = cost(defaultLogger, Linfo)

	Fatalf = logoutf(defaultLogger, Lfatal)
	Stackf = logoutf(defaultLogger, Lstack)
	Errorf = logoutf(defaultLogger, Lerror)
	Warnf  = logoutf(defaultLogger, Lwarn)
	Infof  = logoutf(defaultLogger, Linfo)
	Debugf = logoutf(defaultLogger, Ldebug)
	Tracef = logoutf(defaultLogger, Ltrace)
	Costf  = costf(defaultLogger, Linfo)
)

// 默认只输出 INFO 及以上级别，避免生产环境忘记配置时打出 DEBUG 日志。
var defaultLogger = NewLogger(Linfo, os.Stdout)

// defaultTraceId 使用随机数作为起点，降低多进程重启后 traceId 碰撞的概率。
var defaultTraceId = func() uint64 {
	var b [4]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		return uint64(time.Now().UnixNano())
	}
	return uint64(binary.BigEndian.Uint32(b[:]))
}()

// WithTraceId 为 context 生成 traceId；如果 context 中已有 traceId 则直接返回。
// 不传 context 时使用 context.Background()。
func WithTraceId(ctx ...context.Context) context.Context {
	xctx := context.Background()
	if len(ctx) > 0 && ctx[0] != nil {
		xctx = ctx[0]
		if _, ok := xctx.Value(traceIdKey{}).(string); ok {
			return xctx
		}
	}
	id := atomic.AddUint64(&defaultTraceId, 1)
	return context.WithValue(xctx, traceIdKey{}, strconv.FormatUint(id, 10))
}

// SetTraceId 把调用方生成的 traceId 写入 context，支持常见整数与字符串类型。
func SetTraceId(ctx context.Context, traceId interface{}) context.Context {
	ctx = normalizeCtx(ctx)
	var s string
	switch tid := traceId.(type) {
	case nil:
		s = "-"
	case uint64:
		s = strconv.FormatUint(tid, 10)
	case uint32:
		s = strconv.FormatUint(uint64(tid), 10)
	case int64:
		s = strconv.FormatInt(tid, 10)
	case int:
		s = strconv.Itoa(tid)
	case int32:
		s = strconv.FormatInt(int64(tid), 10)
	case string:
		s = tid
	default:
		s = fmt.Sprint(traceId)
	}
	return context.WithValue(ctx, traceIdKey{}, s)
}
