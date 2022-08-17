package main

import (
	"context"
	"os"
	"time"

	"github.com/omigo/g"
)

func main() {
	g.SetLevelString("info")

	ctx := context.Background()
	ctx = g.WithTraceId(ctx, 123131231)

	g.Trace(ctx, g.GetLevel())
	g.Debug(ctx, 3)

	g.SetOutput(os.Stdout)
	if g.IsEnabled(g.Linfo) {
		g.Info(ctx, "info enabled, current level:", g.GetLevel())
	}

	g.SetLevel(g.Ldebug)
	g.Debugf(ctx, "%d", g.GetCount(g.Linfo))

	// if matched, set level debug
	ctx = g.WithLevel(ctx, g.Ldebug)

	method1(ctx)
	g.Infof(ctx, "%d", g.GetCountAll())
	g.Fatal(ctx, 3)
}

func method1(ctx context.Context) {
	defer g.Cost(ctx, "method1")()
	g.Trace(ctx, 1)
	g.Debug(ctx, 1)
	if g.IsEnabled(g.Linfo) {
		g.Info(ctx, "info enabled")
	}
	method2(ctx)
}

func method2(ctx context.Context) {
	defer g.Costf(ctx, "method%d", 2)()
	g.Trace(ctx, 2)
	g.Debug(ctx, 2)
	g.Info(ctx, 2)

	log := g.NewLogger(g.Lwarn, os.Stdout)

	log.Warn(ctx, "new logger")
	if log.IsEnabled(g.Lerror) {
		log.Error(ctx, "error enabled")
	}
	log.Stack(ctx, 2)
	time.Sleep(12345 * time.Microsecond)
}
