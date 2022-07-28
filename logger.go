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

type Logger struct {
	mutex sync.RWMutex

	Level  Level
	Buffer *bufio.Writer
	count  [LevelLength]uint64

	WithLevel   func(ctx context.Context, level Level) context.Context
	WithTraceId func(ctx context.Context, traceId interface{}) context.Context

	Fatal, Stack, Error, Warn, Info, Debug, Trace        func(ctx context.Context, msg ...interface{})
	Fatalf, Stackf, Errorf, Warnf, Infof, Debugf, Tracef func(ctx context.Context, format string, msg ...interface{})
}

func (l *Logger) GetCount(level Level) uint64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.count[level]
}
func (l *Logger) GetCountAll() [LevelLength]uint64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.count
}

func logout(logger *Logger, level Level) func(ctx context.Context, msg ...interface{}) {
	return func(ctx context.Context, msg ...interface{}) {
		if logger.check(ctx, level) {
			logger.output(ctx, level, "", msg...)
		}
	}
}

func logoutf(logger *Logger, level Level) func(ctx context.Context, format string, msg ...interface{}) {
	return func(ctx context.Context, format string, msg ...interface{}) {
		if logger.check(ctx, level) {
			logger.output(ctx, level, format, msg...)
		}
	}
}

func NewLogger(level Level, writer io.Writer) *Logger {
	newLogger := &Logger{
		Level:  level,
		Buffer: bufio.NewWriter(writer),
	}

	newLogger.WithLevel = WithLevel
	newLogger.WithTraceId = WithTraceId

	newLogger.Fatal = logout(newLogger, Lfatal)
	newLogger.Stack = logout(newLogger, Lstack)
	newLogger.Error = logout(newLogger, Lerror)
	newLogger.Warn = logout(newLogger, Lwarn)
	newLogger.Info = logout(newLogger, Linfo)
	newLogger.Debug = logout(newLogger, Ldebug)
	newLogger.Trace = logout(newLogger, Ltrace)

	newLogger.Fatalf = logoutf(newLogger, Lfatal)
	newLogger.Stackf = logoutf(newLogger, Lstack)
	newLogger.Errorf = logoutf(newLogger, Lerror)
	newLogger.Warnf = logoutf(newLogger, Lwarn)
	newLogger.Infof = logoutf(newLogger, Linfo)
	newLogger.Debugf = logoutf(newLogger, Ldebug)
	newLogger.Tracef = logoutf(newLogger, Ltrace)

	return newLogger
}

func (l *Logger) IsEnabled(level Level) bool {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.Level >= level
}

func (l *Logger) GetLevel() Level {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.Level
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
	if format == "" {
		for _, v := range msg {
			l.Buffer.WriteByte(whitespace)
			fmt.Fprint(l.Buffer, v)
		}
	} else {
		l.Buffer.WriteByte(whitespace)
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
	l.count[level]++
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
