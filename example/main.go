package main

import (
	"context"
	"os"
	"time"

	"github.com/omigo/g"
)

type TestStruct struct {
	A int
	B string
}

func main() {
	g.SetLevelString("trace")

	ctx := context.Background()
	ctx = g.WithTraceId(ctx)

	g.Trace(ctx, g.GetLevel())

	g.Debug(ctx, 3)

	g.SetOutput(os.Stdout)
	if g.IsEnabled(ctx, g.Linfo) {
		g.Info(ctx, "info enabled, current level:", g.GetLevel())
	}

	g.SetLevel(g.Ldebug)
	g.Debugf(ctx, "%d", g.GetCount(g.Linfo))
	g.Info(ctx, &TestStruct{1, "abc"})

	// if matched, set level debug
	ctx = g.WithLevel(ctx, g.Ldebug)
	ctx = g.SetTraceId(ctx, 3)

	method1(ctx)
	g.Infof(ctx, "%d", g.GetCountAll())
	g.Fatal(ctx, 3)
}

func method1(ctx context.Context) {
	defer g.Cost(ctx, "method1", "enable test")()
	g.Trace(ctx, 1)
	g.Debug(ctx, 1)
	if g.IsEnabled(ctx, g.Linfo) {
		g.Info(ctx, "info enabled")
	}
	method2(ctx)
}

func method2(ctx context.Context) {
	defer g.Costf(ctx, "method%d %s", 2, "stack test")()
	g.Trace(ctx, 2)
	g.Debug(ctx, 2)
	g.Info(ctx, 2)

	log := g.NewLogger(g.Lwarn, os.Stdout)

	log.Warn(ctx, "new logger")
	if log.IsEnabled(ctx, g.Lerror) {
		log.Error(ctx, "error enabled")
	}
	log.Stack(ctx, 2)
	time.Sleep(12345 * time.Microsecond)
}
