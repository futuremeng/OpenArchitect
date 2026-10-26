package language

import (
	"context"
	"errors"
	"testing"
)

type stub struct {
	name     string
	language string
	detect   bool
	err      error
}

func (s stub) Language() string { return s.language }
func (s stub) Detect(string) (bool, error) {
	return s.detect, s.err
}
func (s stub) Parse(context.Context, string) (Output, error) {
	return Output{ModulePath: s.name}, nil
}

func resetRegistry(t *testing.T) {
	t.Helper()
	previous := registry
	registry = map[string]Adapter{}
	t.Cleanup(func() { registry = previous })
}

func TestAllSortedAndGet(t *testing.T) {
	resetRegistry(t)
	Register(stub{language: "go"})
	Register(stub{language: "python"})
	Register(stub{language: "java"})
	// Each round re-iterates the map, whose range order is randomized per
	// call: repeating the assertion means a removed sort.Slice cannot pass
	// by accidentally iterating into ascending order.
	for round := 0; round < 8; round++ {
		got := All()
		if len(got) != 3 || got[0].Language() != "go" || got[1].Language() != "java" || got[2].Language() != "python" {
			t.Fatalf("All() round %d = %v", round, names(got))
		}
	}
	if a, ok := Get("java"); !ok || a.Language() != "java" {
		t.Fatalf("Get(java) = %v %v", a, ok)
	}
	if _, ok := Get("rust"); ok {
		t.Fatal("rust must be absent")
	}
}

func TestRegisterRejectsDuplicates(t *testing.T) {
	resetRegistry(t)
	Register(stub{language: "go"})
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate registration must panic")
		}
	}()
	Register(stub{language: "go"})
}

func TestDetectReportsUnreadableSeparatelyFromUnsupported(t *testing.T) {
	// 偏差 11 的目的：UI 要对这两种情况给不同文案。
	ok, err := stub{language: "go", detect: false}.Detect("/anywhere")
	if ok || err != nil {
		t.Fatalf("not-go = %v %v", ok, err)
	}
	// The composite literal needs parentheses in an `if` init clause, else
	// the parser treats `{` as the start of the if body.
	if _, err := (stub{language: "go", err: errors.New("permission denied")}).Detect("/nope"); err == nil {
		t.Fatal("unreadable must surface an error")
	}
}

func names(adapters []Adapter) []string {
	out := make([]string, 0, len(adapters))
	for _, a := range adapters {
		out = append(out, a.Language())
	}
	return out
}
