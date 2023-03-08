package g

import (
	"context"
	"io"
)

type HertzFullLogger struct{}

func wrapper(ctx context.Context, level Level, format string, v []interface{}) {
	if defaultLogger.check(ctx, level) {
		defaultLogger.output(ctx, level, 6, format, v)
	}
}
func (HertzFullLogger) Trace(v ...interface{})  { wrapper(context.TODO(), Ltrace, "", v) }
func (HertzFullLogger) Debug(v ...interface{})  { wrapper(context.TODO(), Ltrace, "", v) }
func (HertzFullLogger) Info(v ...interface{})   { wrapper(context.TODO(), Ldebug, "", v) }
func (HertzFullLogger) Notice(v ...interface{}) { wrapper(context.TODO(), Linfo, "", v) }
func (HertzFullLogger) Warn(v ...interface{})   { wrapper(context.TODO(), Lwarn, "", v) }
func (HertzFullLogger) Error(v ...interface{})  { wrapper(context.TODO(), Lerror, "", v) }
func (HertzFullLogger) Fatal(v ...interface{})  { wrapper(context.TODO(), Lfatal, "", v) }

func (HertzFullLogger) Tracef(format string, v ...interface{}) {
	wrapper(context.TODO(), Ltrace, format, v)
}
func (HertzFullLogger) Debugf(format string, v ...interface{}) {
	wrapper(context.TODO(), Ltrace, format, v)
}
func (HertzFullLogger) Infof(format string, v ...interface{}) {
	wrapper(context.TODO(), Ldebug, format, v)
}
func (HertzFullLogger) Noticef(format string, v ...interface{}) {
	wrapper(context.TODO(), Linfo, format, v)
}
func (HertzFullLogger) Warnf(format string, v ...interface{}) {
	wrapper(context.TODO(), Lwarn, format, v)
}
func (HertzFullLogger) Errorf(format string, v ...interface{}) {
	wrapper(context.TODO(), Lerror, format, v)
}
func (HertzFullLogger) Fatalf(format string, v ...interface{}) {
	wrapper(context.TODO(), Lfatal, format, v)
}

func (HertzFullLogger) CtxTracef(ctx context.Context, format string, v ...interface{}) {
	wrapper(ctx, Ltrace, format, v)
}
func (HertzFullLogger) CtxDebugf(ctx context.Context, format string, v ...interface{}) {
	wrapper(ctx, Ltrace, format, v)
}
func (HertzFullLogger) CtxInfof(ctx context.Context, format string, v ...interface{}) {
	wrapper(ctx, Ldebug, format, v)
}
func (HertzFullLogger) CtxNoticef(ctx context.Context, format string, v ...interface{}) {
	wrapper(ctx, Linfo, format, v)
}
func (HertzFullLogger) CtxWarnf(ctx context.Context, format string, v ...interface{}) {
	wrapper(ctx, Lwarn, format, v)
}
func (HertzFullLogger) CtxErrorf(ctx context.Context, format string, v ...interface{}) {
	wrapper(ctx, Lerror, format, v)
}
func (HertzFullLogger) CtxFatalf(ctx context.Context, format string, v ...interface{}) {
	wrapper(ctx, Lfatal, format, v)
}

// func (HertzFullLogger) SetLevel(level hlog.Level) {
func (HertzFullLogger) SetLevel(level int) {
	if level == 0 {
		level = 1
	}
	lvl := 7 - level // hlog 等级 刚好反过来
	defaultLogger.SetLevel(Level(lvl))
}

func (HertzFullLogger) SetOutput(w io.Writer) {
	defaultLogger.SetOutput(w)
}

/*

type MyLogger struct {
	g.HertzFullLogger
}

func (l MyLogger) SetLevel(level hlog.Level) {
	l.HertzFullLogger.SetLevel(int(level))
}

func init() {
	hlog.SetLogger(&MyLogger{})
}

*/
