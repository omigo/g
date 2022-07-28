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

func IsEnabled(level Level) bool       { return defultLogger.IsEnabled(level) }
func GetLevel() Level                  { return defultLogger.GetLevel() }
func SetLevel(level Level)             { defultLogger.SetLevel(level) }
func SetLevelString(level string)      { defultLogger.SetLevelString(level) }
func SetOutput(writer io.Writer)       { defultLogger.SetOutput(writer) }
func GetCount(level Level) uint64      { return defultLogger.GetCount(level) }
func GetCountAll() [LevelLength]uint64 { return defultLogger.GetCountAll() }

func WithLevel(ctx context.Context, level Level) context.Context {
	return context.WithValue(ctx, logLevelKey{}, level)
}

var (
	Fatal = logout(defultLogger, Lfatal)
	Stack = logout(defultLogger, Lstack)
	Error = logout(defultLogger, Lerror)
	Warn  = logout(defultLogger, Lwarn)
	Info  = logout(defultLogger, Linfo)
	Debug = logout(defultLogger, Ldebug)
	Trace = logout(defultLogger, Ltrace)

	Fatalf = logoutf(defultLogger, Lfatal)
	Stackf = logoutf(defultLogger, Lstack)
	Errorf = logoutf(defultLogger, Lerror)
	Warnf  = logoutf(defultLogger, Lwarn)
	Infof  = logoutf(defultLogger, Linfo)
	Debugf = logoutf(defultLogger, Ldebug)
	Tracef = logoutf(defultLogger, Ltrace)
)

var defultLogger = NewLogger(Ldebug, os.Stdout)

func WithTraceId(ctx context.Context, traceId interface{}) context.Context {
	var s string
	switch traceId.(type) {
	case string:
		s = traceId.(string)
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
	default:
		s = fmt.Sprint(traceId)
	}
	return context.WithValue(ctx, traceIdKey{}, s)
}
