package language

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func parseFixture(t *testing.T, name string) Output {
	t.Helper()
	out, err := goAdapter{}.Parse(context.Background(), fixturesDir(t)+"/"+name)
	if err != nil {
		t.Fatalf("Parse(%s): %v", name, err)
	}
	return out
}

func fixturesDir(t *testing.T) string {
	t.Helper()
	return "../../../testdata/fixtures"
}

func idsOf(elements []model.Element) []string {
	out := make([]string, 0, len(elements))
	for _, e := range elements {
		out = append(out, e.ID)
	}
	slices.Sort(out)
	return out
}

func TestSimpleMultiMainElements(t *testing.T) {
	out := parseFixture(t, "simple-multi-main")
	want := []string{
		// Cart 被两个 main 传递引用 → 挂 System（§6.3 的多归属规则）。
		"cmp:example.com/simple/internal/store.Cart",
		"ctn:example.com/simple/cmd/api",
		"ctn:example.com/simple/cmd/worker",
		"sys:example.com/simple",
	}
	if got := idsOf(out.Elements); !slices.Equal(got, want) {
		t.Fatalf("elements =\n%v\nwant\n%v", got, want)
	}
	for _, e := range out.Elements {
		if e.ID != "cmp:example.com/simple/internal/store.Cart" {
			continue
		}
		if e.Name != "Cart" || e.Description != "Cart is shared by both commands." {
			t.Fatalf("element = %+v", e)
		}
		if !slices.Equal(e.Methods, []string{"Add", "Total"}) {
			t.Fatalf("methods = %v", e.Methods)
		}
		if e.FilePath != "internal/store/store.go" {
			t.Fatalf("file path = %q", e.FilePath)
		}
	}
	// slices.Contains 的第一个参数必须是切片，这里要查的是拼起来的 id 串，
	// 所以用 strings.Contains（brief 抄成了 slices.Contains，编译不过）。
	if strings.Contains(strings.Join(idsOf(out.Elements), " "), "hidden") {
		t.Fatal("unexported type leaked into elements")
	}
	// §6.3 归属规则的正反两面都要有边断言：Cart 被两个 main 传递引用（多归属）
	// → 只挂 System；两个 Container 只接收 System→Container 边。若多归属规则
	// 被改成「每个可达 Container 都挂一条」，下面既会多出 ctn→cmp 边（显式
	// 拒绝），也会让精确清单失配。
	var contains []string
	for _, r := range out.Relationships {
		if r.Type != model.RelContains {
			continue
		}
		if strings.HasPrefix(r.FromID, "ctn:") && strings.HasPrefix(r.ToID, "cmp:") {
			t.Fatalf("multi-owner component must not hang on a container: %s -> %s", r.FromID, r.ToID)
		}
		contains = append(contains, r.FromID+" -> "+r.ToID)
	}
	slices.Sort(contains)
	wantContains := []string{
		"sys:example.com/simple -> cmp:example.com/simple/internal/store.Cart",
		"sys:example.com/simple -> ctn:example.com/simple/cmd/api",
		"sys:example.com/simple -> ctn:example.com/simple/cmd/worker",
	}
	if !slices.Equal(contains, wantContains) {
		t.Fatalf("contains =\n%v\nwant\n%v", contains, wantContains)
	}
}

func TestSystemElementDescribesTheModule(t *testing.T) {
	out := parseFixture(t, "simple-multi-main")
	// spec §6.3：System 恒 1 个，名字取 modulePath 末段，描述取根包的文档注释
	// （fixture 的 doc.go 就是为这条写的）。
	var systems []model.Element
	for _, e := range out.Elements {
		if e.Type == model.TypeSystem {
			systems = append(systems, e)
		}
	}
	if len(systems) != 1 {
		t.Fatalf("systems = %v, want exactly one", idsOf(systems))
	}
	system := systems[0]
	if system.ID != "sys:example.com/simple" || system.Name != "simple" {
		t.Fatalf("system = %+v", system)
	}
	if system.Description != "Package simple is the System root of this fixture." {
		t.Fatalf("system description = %q", system.Description)
	}
	if !strings.HasPrefix(system.RepoID, "repo:") {
		t.Fatalf("system repo id = %q", system.RepoID)
	}
}

func TestSharedLibContainment(t *testing.T) {
	out := parseFixture(t, "shared-lib")
	// 只有 cmd/app 一个 main：core 与 edge 都挂它，循环依赖不能引起死循环。
	want := []string{"cmp:example.com/shared/core.Server", "cmp:example.com/shared/edge.Client", "ctn:example.com/shared/cmd/app", "sys:example.com/shared"}
	if got := idsOf(out.Elements); !slices.Equal(got, want) {
		t.Fatalf("elements =\n%v\nwant\n%v", got, want)
	}
	var contains []string
	for _, r := range out.Relationships {
		if r.Type == model.RelContains {
			contains = append(contains, r.FromID+" -> "+r.ToID)
		}
	}
	slices.Sort(contains)
	wantContains := []string{
		"ctn:example.com/shared/cmd/app -> cmp:example.com/shared/core.Server",
		"ctn:example.com/shared/cmd/app -> cmp:example.com/shared/edge.Client",
		"sys:example.com/shared -> ctn:example.com/shared/cmd/app",
	}
	if !slices.Equal(contains, wantContains) {
		t.Fatalf("contains =\n%v\nwant\n%v", contains, wantContains)
	}
}

func TestGenericAndInterfaceElements(t *testing.T) {
	out := parseFixture(t, "generic-and-iface")
	// §6.3 排除集：fixture 里的 `type Sink = Reader` 是别名而非此处声明的命名
	// 类型（§6.3 要求 `Type() == Obj()`），不得成为自己的 Component。别名在
	// 生产代码里漏网时还会按被别名类型的身份复写出元素，所以下面的 want 清单
	// （条数与内容都定死）同样属于这条排除的断言：放宽 exportedComponentTypes
	// 守卫会让这里以多出的元素 loudly 失败。
	for _, e := range out.Elements {
		if e.ID == "cmp:example.com/generic/iface.Sink" {
			t.Fatalf("type alias became its own component: %+v", e)
		}
	}
	want := []string{
		"cmp:example.com/generic/box.Box", "cmp:example.com/generic/box.Pair",
		"cmp:example.com/generic/iface.Reader",
		"sys:example.com/generic",
	}
	if got := idsOf(out.Elements); !slices.Equal(got, want) {
		t.Fatalf("elements =\n%v\nwant\n%v", got, want)
	}
	// spec §6.3 只把底层为 struct/interface 的导出 named type 算 Component，所以
	// fixture 里的 `type String string`（底层 *types.Basic）不出节点。brief 的期望
	// 表把它列进去了，与 §6.3 以及 brief 自己给出的 exportedComponentTypes 相互
	// 矛盾；这条断言把「排除」钉住，防止后来人为了凑期望表去放宽类型过滤。
	for _, id := range idsOf(out.Elements) {
		if strings.HasSuffix(id, ".String") {
			t.Fatalf("scalar named type must not become a component: %s", id)
		}
	}
	for _, e := range out.Elements {
		if strings.HasPrefix(e.ID, "ctn:") {
			t.Fatalf("fixture has no main package, got container %q", e.ID)
		}
	}
	// 无 main 包：没有任何 Component 该挂到 Container 上；同时把 §6.3 归属规则
	// 的「不被任何 main 引用 → 挂 System」写成精确的正向断言——三条 sys→cmp
	// 边少任何一条（或改了挂法）都会让下面失配。
	var contains []string
	for _, r := range out.Relationships {
		if r.Type == model.RelContains {
			if strings.HasPrefix(r.FromID, "ctn:") {
				t.Fatalf("unexpected container edge %+v", r)
			}
			contains = append(contains, r.FromID+" -> "+r.ToID)
		}
	}
	slices.Sort(contains)
	wantContains := []string{
		"sys:example.com/generic -> cmp:example.com/generic/box.Box",
		"sys:example.com/generic -> cmp:example.com/generic/box.Pair",
		"sys:example.com/generic -> cmp:example.com/generic/iface.Reader",
	}
	if !slices.Equal(contains, wantContains) {
		t.Fatalf("contains =\n%v\nwant\n%v", contains, wantContains)
	}
}

func TestDetect(t *testing.T) {
	root := fixturesDir(t) + "/generic-and-iface"
	ok, err := goAdapter{}.Detect(root)
	if !ok || err != nil {
		t.Fatalf("Detect(go fixture) = %v %v", ok, err)
	}
	// 无 go.mod 与路径读不了是同一个答案：Detect 把任何 os.Stat 错误（含
	// ErrNotExist）都往外传，spec §11 的 R2 把两者都算「这个目录不能被当作 Go
	// 模块解析」。计划 Task 9 的 TestDetect 断言的就是 false+err；brief 抄写的
	// `ok || err != nil` 与它自己给出的实现和 §11 都矛盾。
	ok, err = goAdapter{}.Detect(fixturesDir(t))
	if ok || err == nil {
		t.Fatalf("Detect(fixtures dir, no go.mod) = %v %v, want false plus an error", ok, err)
	}
	// The composite literal needs parentheses in an `if` init clause, else the
	// parser treats `{` as the start of the if body.
	if _, err := (goAdapter{}).Detect("/definitely/not/here"); err == nil {
		t.Fatal("unreadable root must return an error")
	}
}
