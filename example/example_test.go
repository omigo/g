package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/omigo/g"
)

// HertzFullLogger 的调用点在 example 目录，自动跳过库内帧后应指向本文件。
// 这等价于 hertz 的 hlog.DefaultLogger().Info() 直接调用方式。
func TestHertzCallSite(t *testing.T) {
	var buf bytes.Buffer
	oldLevel := g.GetLevel()
	g.SetLevel(g.Ltrace)
	g.SetOutput(&buf)
	defer func() {
		g.SetLevel(oldLevel)
		g.SetOutput(os.Stdout)
	}()

	g.HertzFullLogger{}.Info("hertz-call-site")
	if !strings.Contains(buf.String(), "example_test.go") {
		t.Fatalf("hertz 日志调用点应指向 example_test.go，实际: %q", buf.String())
	}
}
