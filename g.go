package g

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

type Level int

func (l Level) String() string {
	return levelStrings[l]
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
func GetCount(level Level) uint64                     { return defaultLogger.GetCount(level) }
func GetCountAll() [LevelLength]uint64                { return defaultLogger.GetCountAll() }
func IsEnabled(ctx context.Context, level Level) bool { return defaultLogger.IsEnabled(ctx, level) }

func WithLevel(ctx context.Context, level Level) context.Context {
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

var defaultLogger = NewLogger(Ldebug, os.Stdout)

var defaultTraceId uint64 = uint64(time.Now().Nanosecond() % 1000000)

func WithTraceId(ctx ...context.Context) context.Context {
	var xctx context.Context
	if len(ctx) == 0 {
		xctx = context.Background()
	} else {
		xctx = ctx[0]
		if _, ok := xctx.Value(traceIdKey{}).(string); ok {
			return xctx
		}
	}
	id := atomic.AddUint64(&defaultTraceId, 1)
	return context.WithValue(xctx, traceIdKey{}, strconv.FormatUint(id, 10))
}

func SetTraceId(ctx context.Context, traceId interface{}) context.Context {
	var s string
	switch tid := traceId.(type) {
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
