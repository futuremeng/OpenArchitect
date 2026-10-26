// Package ciguard 守住 .github/workflows/ci.yaml 里的依赖守卫（spec §5.1
// R1/R2、§4.3 DoD-6）。它读 YAML 原文而不是重新执行命令：命令本身也在断言范围
// 里，一条被删掉的 grep 不会自己报错。
package ciguard

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const workflowPath = "../../.github/workflows/ci.yaml"

// minJobTimeout 是 YAML 里 timeout-minutes 的下限，不是它的值。推导：Task 3 的
// PoC-3 门槛是单仓解析 ≤ 60 s，CI 跑的是全模块单测 + 三个 fixture 的扫描，
// Step 4 会量出本地总耗时；15 分钟 ≈ 那个数字的三倍余量。YAML 写 20。
const minJobTimeout = 15

func readWorkflow(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read %s: %v（Task 23 的 YAML 还没建，或路径变了）", workflowPath, err)
	}
	return string(raw)
}

// normalize 吃掉 shell 的续行（反斜杠 + 换行）并把空白压成单空格，这样比对
// 只关心命令内容，不关心 YAML 里的缩进和折行写法。
func normalize(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\\\n", " ")), " ")
}

const uiStackPattern = `'github.com/egoist/mygo|github.com/egoist/mygo/ui|github.com/oneo-me/onevra[^ ]*'`

// 守卫查直接 import（-f '{{join .Imports}}'）而不是 -deps：plugins/sqlite 自己
// 直接 import mygo root（sqlite.go:27、library.go:18），root 又带整套 UI 栈，
// 而 spec §5.1 要求 scanner/oac 收 *store.DB（必然 import store，store 必然
// import 被授权的驱动）——-deps 闭包在任何合法架构下都必然命中。spec 禁的是
// 「import」，命令必须与语义一致。plan Task 23 的 -deps 写法与 plan 自己的
// 架构互斥，这是对它的定稿修正，YAML 注释里带同一份证据。
const importsFormat = `-f '{{join .Imports "\n"}}'`

func TestWorkflowKeepsTheThreeDependencyGuards(t *testing.T) {
	got := normalize(readWorkflow(t))
	wants := []string{
		// 正向：core 与 scanner 的直接 import 里不许出现 UI 栈。-x 是锚定匹配——
		// 前缀匹配会把 plugins/sqlite 一起误判成违规（spec §5.1 特意点过）。
		`go list ` + importsFormat + ` ./internal/core/... ./internal/scanner/... | grep -x -E ` + uiStackPattern + ` && exit 1 || true`,
		// 反向：驱动是例外，但例外只在 store 那一层。
		`go list ` + importsFormat + ` ./internal/scanner/... ./internal/core/model/... ./internal/core/graph/... | grep -x 'github.com/egoist/mygo/plugins/sqlite' && exit 1 || true`,
		// DoD-4：oac 那份数据不是 GUI 顺手喂给它的。
		`go list ` + importsFormat + ` ./cmd/oac | grep -x -E ` + uiStackPattern + ` && exit 1 || true`,
	}
	for _, want := range wants {
		if !strings.Contains(got, normalize(want)) {
			t.Errorf("ci.yaml 丢了这条守卫：\nwant %s\ngot  %s", want, got)
		}
	}
}

func TestWorkflowRunsVetAndTest(t *testing.T) {
	got := normalize(readWorkflow(t))
	for _, want := range []string{"go vet ./...", "go test ./..."} {
		if !strings.Contains(got, want) {
			t.Errorf("ci.yaml 必须跑 %q", want)
		}
	}
}

// 三条守卫里的 `|| true` 是合法的：它吞的是 grep "没匹配到" 的退出码 1，不
// 是断言失败。vet/test 这两条 run 不一样——本仓库没有已知失败集，给它们加
// continue-on-error 或 || true 等于把后来的真红灯降级成日志。
func TestWorkflowHasNoToleratedGates(t *testing.T) {
	raw := readWorkflow(t)
	if strings.Contains(raw, "continue-on-error") {
		t.Fatal("ci.yaml 没有已知失败可吞，continue-on-error 只会掩盖真故障")
	}
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		gated := strings.HasPrefix(trimmed, "run: go vet") || strings.HasPrefix(trimmed, "run: go test")
		if gated && (strings.Contains(trimmed, "|| true") || strings.Contains(trimmed, "; true")) {
			t.Fatalf("门禁步骤被容忍了：%q", trimmed)
		}
	}
}

func TestWorkflowTimeoutKeepsItsFloor(t *testing.T) {
	m := regexp.MustCompile(`(?m)^\s*timeout-minutes:\s*(\d+)\s*$`).FindStringSubmatch(readWorkflow(t))
	if m == nil {
		t.Fatalf("ci.yaml 没有 job 级 timeout-minutes：卡死的作业会一直占着 runner")
	}
	got, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatal(err)
	}
	if got < minJobTimeout {
		t.Errorf("timeout-minutes = %d，下限 %d", got, minJobTimeout)
	}
}

func TestWorkflowPinsCgoAndGoVersion(t *testing.T) {
	raw := readWorkflow(t)
	// 两个值逐字来自 Global Constraints：CGO_ENABLED=0 与 go1.27.2（onevra 的下限
	// 是 1.27.1）。CI 里放开 CGO 等于让"没有 C 工具链也能构建"这条前提失真。
	for _, want := range []string{`CGO_ENABLED: "0"`, `go-version: "1.27.2"`} {
		if !strings.Contains(raw, want) {
			t.Errorf("ci.yaml 丢了 %q", want)
		}
	}
}
