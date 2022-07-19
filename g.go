package g

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Level int

func (l Level) String() string {
	return levelStrings[l]
}

const (
	Loff   Level = 0
	Lfatal       = 1
	Lstack       = 2
	Lerror       = 3
	Lwarn        = 4
	Linfo        = 5
	Ldebug       = 6
	Ltrace       = 7
)

var levelStrings = []string{"off", "fatal", "stack", "error", "warn", "info", "debug", "trace"}

func SetLevel(level Level) {
	defultLogger.SetLevel(level)
}
func SetLevelString(level string) {
	defultLogger.SetLevelString(level)
}
func SetOutput(writer io.Writer) {
	defultLogger.SetOutput(writer)
}

func WithLevel(ctx context.Context, level Level) context.Context {
	return context.WithValue(ctx, logLevelKey{}, level)
}
func WithTraceId(ctx context.Context, traceId interface{}) context.Context {
	var s string
	switch t := traceId.(type) {
	case string:
		s = t
	default:
		s = fmt.Sprint(traceId)
	}
	return context.WithValue(ctx, traceIdKey{}, s)
}

func genMethod(logger *Logger, level Level) func(ctx context.Context, msg ...interface{}) {
	return func(ctx context.Context, msg ...interface{}) {
		if logger.check(ctx, level) {
			logger.output(ctx, level, "", msg...)
		}
	}
}

func genFormatMethod(logger *Logger, level Level) func(ctx context.Context, format string, msg ...interface{}) {
	return func(ctx context.Context, format string, msg ...interface{}) {
		if logger.check(ctx, level) {
			logger.output(ctx, level, format, msg...)
		}
	}
}

var (
	Fatal = genMethod(defultLogger, Lfatal)
	Stack = genMethod(defultLogger, Lstack)
	Error = genMethod(defultLogger, Lerror)
	Warn  = genMethod(defultLogger, Lwarn)
	Info  = genMethod(defultLogger, Linfo)
	Debug = genMethod(defultLogger, Ldebug)
	Trace = genMethod(defultLogger, Ltrace)

	Fatalf = genFormatMethod(defultLogger, Lfatal)
	Stackf = genFormatMethod(defultLogger, Lstack)
	Errorf = genFormatMethod(defultLogger, Lerror)
	Warnf  = genFormatMethod(defultLogger, Lwarn)
	Infof  = genFormatMethod(defultLogger, Linfo)
	Debugf = genFormatMethod(defultLogger, Ldebug)
	Tracef = genFormatMethod(defultLogger, Ltrace)
)

var defultLogger = NewLogger(Linfo, os.Stdout)

type Logger struct {
	mutex sync.Mutex

	Level  Level
	Buffer *bufio.Writer

	WithLevel   func(ctx context.Context, level Level) context.Context
	WithTraceId func(ctx context.Context, traceId interface{}) context.Context

	Fatal, Stack, Error, Warn, Info, Debug, Trace        func(ctx context.Context, msg ...interface{})
	Fatalf, Stackf, Errorf, Warnf, Infof, Debugf, Tracef func(ctx context.Context, format string, msg ...interface{})
}

func NewLogger(level Level, writer io.Writer) *Logger {
	newLogger := &Logger{
		mutex:  sync.Mutex{},
		Level:  level,
		Buffer: bufio.NewWriter(writer),
	}

	newLogger.WithLevel = WithLevel
	newLogger.WithTraceId = WithTraceId

	newLogger.Fatal = genMethod(newLogger, Lfatal)
	newLogger.Stack = genMethod(newLogger, Lstack)
	newLogger.Error = genMethod(newLogger, Lerror)
	newLogger.Warn = genMethod(newLogger, Lwarn)
	newLogger.Info = genMethod(newLogger, Linfo)
	newLogger.Debug = genMethod(newLogger, Ldebug)
	newLogger.Trace = genMethod(newLogger, Ltrace)

	newLogger.Fatalf = genFormatMethod(newLogger, Lfatal)
	newLogger.Stackf = genFormatMethod(newLogger, Lstack)
	newLogger.Errorf = genFormatMethod(newLogger, Lerror)
	newLogger.Warnf = genFormatMethod(newLogger, Lwarn)
	newLogger.Infof = genFormatMethod(newLogger, Linfo)
	newLogger.Debugf = genFormatMethod(newLogger, Ldebug)
	newLogger.Tracef = genFormatMethod(newLogger, Ltrace)

	return newLogger
}

func (l *Logger) SetLevel(level Level) {
	l.mutex.Lock()
	l.Level = level
	l.mutex.Unlock()
}

func (l *Logger) SetLevelString(level string) {
	level = strings.ToLower(level)
	var lvl = -1
	for i, v := range levelStrings {
		if v == level {
			lvl = i
			break
		}
	}
	if lvl == -1 {
		Errorf(context.Background(), "error log level string: %s", level)
		return
	}
	l.SetLevel(Level(lvl))
}

func (l *Logger) SetOutput(writer io.Writer) {
	l.mutex.Lock()
	l.Buffer = bufio.NewWriter(writer)
	l.mutex.Unlock()
}

type logLevelKey struct{}
type traceIdKey struct{}

func (l *Logger) check(ctx context.Context, level Level) bool {
	if v := ctx.Value(logLevelKey{}); v != nil {
		if special, ok := v.(Level); ok {
			return special >= level
		}
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.Level >= level
}

const whitespace = ' '

func (l *Logger) output(ctx context.Context, level Level, format string, msg ...interface{}) {
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	traceId := getTraceId(ctx)
	file, line := getFileLine()

	l.mutex.Lock()
	l.Buffer.WriteString(ts)
	l.Buffer.WriteByte(whitespace)
	l.Buffer.WriteString(traceId)
	l.Buffer.WriteByte(whitespace)
	l.Buffer.WriteString(level.String())
	l.Buffer.WriteByte(whitespace)
	l.Buffer.WriteString(file)
	l.Buffer.WriteByte(':')
	l.Buffer.WriteString(strconv.Itoa(line))
	l.Buffer.WriteByte(whitespace)
	if format == "" {
		fmt.Fprint(l.Buffer, msg...)
	} else {
		fmt.Fprintf(l.Buffer, format, msg...)
	}
	l.Buffer.WriteByte('\n')

	if level == Lstack {
		stack := make([]byte, 4096)
		runtime.Stack(stack, true)
		l.Buffer.Write(stack)
		l.Buffer.WriteByte('\n')
	}

	l.Buffer.Flush()
	l.mutex.Unlock()

	if level == Lfatal {
		os.Exit(127)
	}
}

func getTraceId(ctx context.Context) string {
	if v := ctx.Value(traceIdKey{}); v != nil {
		if traceId, ok := v.(string); ok {
			return traceId
		}
	}
	return "-"
}

func getFileLine() (string, int) {
	_, path, line, ok := runtime.Caller(3) // expensive
	if ok {
		if i := strings.LastIndexByte(path, '/'); i > -1 {
			if j := strings.LastIndexByte(path[:i], '/'); j > -1 {
				path = path[j+1:]
			} else {
				path = path[i+1:]
			}
		}
	}
	return path, line
}
