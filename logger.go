package g

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
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
	WithTraceId func(ctx ...context.Context) context.Context
	SetTraceId  func(ctx context.Context, traceId interface{}) context.Context

	Fatal, Stack, Error, Warn, Info, Debug, Trace        func(ctx context.Context, msg ...interface{})
	Fatalf, Stackf, Errorf, Warnf, Infof, Debugf, Tracef func(ctx context.Context, format string, msg ...interface{})

	Cost  func(ctx context.Context, msg ...interface{}) func()
	Costf func(ctx context.Context, format string, msg ...interface{}) func()
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
			logger.output(ctx, level, "", msg)
		}
	}
}

func logoutf(logger *Logger, level Level) func(ctx context.Context, format string, msg ...interface{}) {
	return func(ctx context.Context, format string, msg ...interface{}) {
		if logger.check(ctx, level) {
			logger.output(ctx, level, format, msg)
		}
	}
}

func cost(logger *Logger, level Level) func(ctx context.Context, msg ...interface{}) func() {
	return func(ctx context.Context, msg ...interface{}) func() {
		s := append(msg, "start...")
		logger.output(ctx, level, "", s)
		start := time.Now()
		return func() {
			e := append(msg, "cost", time.Since(start).Truncate(time.Millisecond).String())
			logger.output(ctx, level, "", e)
		}
	}
}

func costf(logger *Logger, level Level) func(ctx context.Context, format string, msg ...interface{}) func() {
	return func(ctx context.Context, format string, msg ...interface{}) func() {
		logger.output(ctx, level, format+" start...", msg)
		start := time.Now()
		return func() {
			logger.output(ctx, level, format+" cost "+time.Since(start).Truncate(time.Millisecond).String(), msg)
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
	level = strings.ToUpper(level)
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

func (l *Logger) output(ctx context.Context, level Level, format string, msg []interface{}) {
	buf := bytes.NewBuffer(make([]byte, 1024))
	write(ctx, buf, level, format, msg)

	l.mutex.Lock()
	defer l.mutex.Unlock()

	io.Copy(l.Buffer, buf)
	l.Buffer.Flush()

	l.count[level]++

	if level == Lfatal {
		os.Exit(99)
	}
}

const (
	whitespace   = ' '
	leftBracket  = '['
	rightBracket = ']'
)

func write(ctx context.Context, buf *bytes.Buffer, level Level, format string, msg []interface{}) {
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	buf.WriteByte(leftBracket)
	buf.WriteString(ts[:19])
	buf.WriteByte(whitespace)
	buf.WriteString(ts[20:])
	buf.WriteByte(rightBracket)

	buf.WriteByte(whitespace)

	file, line, method := getFileLineMethod()
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

	if level == Lstack {
		stack := make([]byte, 4096)
		runtime.Stack(stack, true)
		buf.Write(stack)
		buf.WriteByte('\n')
	}
}

func writeValue(buf *bytes.Buffer, v interface{}) {
	if v == nil {
		buf.WriteString("nil")
		return
	}

	switch vv := v.(type) {
	case error:
		buf.WriteString(vv.Error())
	case []byte:
		buf.Write(vv)
	case string:
		buf.WriteString(vv)
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
		buf.WriteString(strconv.FormatFloat(float64(vv), 'g', 3, 64))
	case float64:
		buf.WriteString(strconv.FormatFloat(vv, 'g', 3, 64))
	case interface{ String() string }:
		buf.WriteString(vv.String())
	default:
		switch reflect.TypeOf(v).Kind() {
		case reflect.Array, reflect.Map, reflect.Slice, reflect.Struct, reflect.Pointer:
			if js, err := json.Marshal(v); err != nil {
				fmt.Fprint(buf, v)
			} else {
				buf.Write(js)
			}
		default:
			fmt.Fprint(buf, v)
		}
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

func getFileLineMethod() (string, int, string) {
	pc, path, line, ok := runtime.Caller(4) // expensive
	if ok {
		if i := strings.LastIndexByte(path, '/'); i > -1 {
			if j := strings.LastIndexByte(path[:i], '/'); j > -1 {
				if k := strings.LastIndexByte(path[:j], '/'); k > -1 {
					path = path[k+1:]
				} else {
					path = path[j+1:]
				}
			} else {
				path = path[i+1:]
			}
		}
	}
	_, method, _ := strings.Cut(runtime.FuncForPC(pc).Name(), ".")
	return strings.TrimSuffix(path, ".go"), line, method
}
