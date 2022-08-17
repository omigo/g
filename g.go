package g

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
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

var levelStrings = []string{"stack", "fatal", "error", "warn", "info", "debug", "trace"}

func IsEnabled(level Level) bool       { return defaultLogger.IsEnabled(level) }
func GetLevel() Level                  { return defaultLogger.GetLevel() }
func SetLevel(level Level)             { defaultLogger.SetLevel(level) }
func SetLevelString(level string)      { defaultLogger.SetLevelString(level) }
func SetOutput(writer io.Writer)       { defaultLogger.SetOutput(writer) }
func GetCount(level Level) uint64      { return defaultLogger.GetCount(level) }
func GetCountAll() [LevelLength]uint64 { return defaultLogger.GetCountAll() }

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

func WithTraceId(ctx context.Context, traceId interface{}) context.Context {
	var s string
	switch traceId.(type) {
	case uint64:
		s = strconv.FormatUint(traceId.(uint64), 10)
	case uint32:
		s = strconv.FormatUint(uint64(traceId.(uint32)), 10)
	case int64:
		s = strconv.FormatInt(traceId.(int64), 10)
	case int:
		s = strconv.Itoa(traceId.(int))
	case int32:
		s = strconv.FormatInt(int64(traceId.(int32)), 10)
	case string:
		s = traceId.(string)
	default:
		s = fmt.Sprint(traceId)
	}
	return context.WithValue(ctx, traceIdKey{}, s)
}
