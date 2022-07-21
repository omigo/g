package main

import (
	"context"

	"github.com/omigo/g"
)

func main() {
	g.SetLevelString("info")

	ctx := context.Background()
	ctx = g.WithTraceId(ctx, 123131231)

	g.Trace(ctx, 3)
	g.Debug(ctx, 3)
	g.Info(ctx, 3)

	// if matched, set level debug
	ctx = g.WithLevel(ctx, g.Ldebug)

	method1(ctx)
}

func method1(ctx context.Context) {
	g.Trace(ctx, 1)
	g.Debug(ctx, 1)
	g.Info(ctx, 1)
	method2(ctx)
}

func method2(ctx context.Context) {
	g.Trace(ctx, 2)
	g.Debug(ctx, 2)
	g.Info(ctx, 2)
	g.Warn(ctx, 2)
	g.Error(ctx, 2)
	g.Stack(ctx, 2)
	g.Fatal(ctx, 2)
}
