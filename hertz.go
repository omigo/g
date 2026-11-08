package g

import (
	"context"
	"io"
)

// HertzFullLogger 把 hertz hlog 的调用转发到本包的默认 Logger。
//
// 注意：hlog.FullLogger 要求 SetLevel(hlog.Level)，这里为了不引入 hertz 依赖
// 使用 SetLevel(int)，直接注册给 hlog 时需要一层薄包装：
//
//	type MyLogger struct{ g.HertzFullLogger }
//
//	func (l MyLogger) SetLevel(level hlog.Level) { l.HertzFullLogger.SetLevel(int(level)) }
//
//	func init() { hlog.SetLogger(&MyLogger{}) }
//
// 另外 non-Context 系列方法没有 ctx，traceId 会输出 "-"；需要链路 traceId 时
// 请使用 CtxXxxf 系列，或在入口处用 g.WithTraceId 把 traceId 放进 ctx。
type HertzFullLogger struct{}

// hertzLevels 把 hlog.Level 映射到本包级别。
// hlog 定义：trace=0 debug=1 info=2 notice=3 warn=4 error=5 fatal=6。
//
// 本包没有 NOTICE 档，notice 归入 warn：这样 hlog 级别设为 Notice 时，
// Info 日志会被正确过滤（若归入 info 则会泄漏）。副作用是 Warn 阈值下
// Notice 仍会输出，与 hlog 原生语义略有差异，见 README。
var hertzLevels = [...]Level{
	Ltrace, // 0 trace
	Ldebug, // 1 debug
	Linfo,  // 2 info
	Lwarn,  // 3 notice（折叠进 warn，保证 Notice 阈值下 Info 被过滤）
	Lwarn,  // 4 warn
	Lerror, // 5 error
	Lfatal, // 6 fatal
}

// wrapper 使用 skipAuto，由 getCallerOutsidePackage 自动跳过本库与 hlog 适配层的栈帧，
// 不依赖固定调用层数：hlog 包级函数与 hlog.DefaultLogger() 两种调用路径都能定位到用户代码。
func wrapper(ctx context.Context, level Level, format string, v []interface{}) {
	if defaultLogger.check(ctx, level) {
		defaultLogger.output(ctx, level, skipAuto, format, v)
	}
}

func (HertzFullLogger) Trace(v ...interface{})  { wrapper(context.TODO(), Ltrace, "", v) }
func (HertzFullLogger) Debug(v ...interface{})  { wrapper(context.TODO(), Ldebug, "", v) }
func (HertzFullLogger) Info(v ...interface{})   { wrapper(context.TODO(), Linfo, "", v) }
func (HertzFullLogger) Notice(v ...interface{}) { wrapper(context.TODO(), Linfo, "", v) }
func (HertzFullLogger) Warn(v ...interface{})   { wrapper(context.TODO(), Lwarn, "", v) }
func (HertzFullLogger) Error(v ...interface{})  { wrapper(context.TODO(), Lerror, "", v) }
func (HertzFullLogger) Fatal(v ...interface{})  { wrapper(context.TODO(), Lfatal, "", v) }

func (HertzFullLogger) Tracef(format string, v ...interface{}) {
	wrapper(context.TODO(), Ltrace, format, v)
}
func (HertzFullLogger) Debugf(format string, v ...interface{}) {
	wrapper(context.TODO(), Ldebug, format, v)
}
func (HertzFullLogger) Infof(format string, v ...interface{}) {
	wrapper(context.TODO(), Linfo, format, v)
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
	wrapper(ctx, Ldebug, format, v)
}
func (HertzFullLogger) CtxInfof(ctx context.Context, format string, v ...interface{}) {
	wrapper(ctx, Linfo, format, v)
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

// SetLevel 把 hlog 的数值级别映射为本包级别，非法值会被收敛到边界。
func (HertzFullLogger) SetLevel(level int) {
	if level < 0 {
		level = 0
	}
	if level >= len(hertzLevels) {
		level = len(hertzLevels) - 1
	}
	defaultLogger.SetLevel(hertzLevels[level])
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
