package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/futuremeng/OpenArchitect/internal/config"
)

func TestSettingsPageShowsCurrentValues(t *testing.T) {
	a := newTestApp(t)
	a.settings = config.Settings{ScanRoot: "/repos/cart", ImpactDepth: 5}
	tt := ui.NewTester(func(c *ui.Context) { a.settingsPage(c) }, 900, 600)
	tt.Frame()
	for _, want := range []string{"设置", "/repos/cart", "影响分析深度", "5"} {
		if !tt.HasText(want) {
			t.Fatalf("%q missing, texts = %v", want, tt.Texts())
		}
	}
}

func TestSaveSettingsButtonPersists(t *testing.T) {
	a := newTestApp(t)
	a.settings = config.Settings{ScanRoot: "/repos/cart", ImpactDepth: config.DefaultImpactDepth}
	tt := ui.NewTester(func(c *ui.Context) { a.settingsPage(c) }, 900, 600)
	tt.Frame()
	if err := tt.Click("保存设置"); err != nil {
		t.Fatal(err)
	}
	if a.settingsErr != nil {
		t.Fatalf("save failed: %v", a.settingsErr)
	}
	if !a.settingsSaved {
		t.Fatal("the page must confirm what it just wrote")
	}
	got, err := config.Load(context.Background(), a.db)
	if err != nil {
		t.Fatal(err)
	}
	if got.ScanRoot != "/repos/cart" || got.ImpactDepth != config.DefaultImpactDepth {
		t.Fatalf("stored = %+v", got)
	}
}

func TestImpactDepthStepperStaysInBounds(t *testing.T) {
	a := newTestApp(t)
	a.settings.ImpactDepth = maxImpactDepth
	tt := ui.NewTester(func(c *ui.Context) { a.settingsPage(c) }, 900, 600)
	tt.Frame()
	if err := tt.Click("加深影响分析"); err != nil {
		t.Fatal(err)
	}
	if a.settings.ImpactDepth != maxImpactDepth {
		t.Fatalf("depth must clamp at %d, got %d", maxImpactDepth, a.settings.ImpactDepth)
	}
	a.settings.ImpactDepth = minImpactDepth
	tt.Frame()
	if err := tt.Click("减小影响分析深度"); err != nil {
		t.Fatal(err)
	}
	if a.settings.ImpactDepth != minImpactDepth {
		t.Fatalf("depth must clamp at %d, got %d", minImpactDepth, a.settings.ImpactDepth)
	}
	a.settings.ImpactDepth = config.DefaultImpactDepth
	tt.Frame()
	if err := tt.Click("减小影响分析深度"); err != nil {
		t.Fatal(err)
	}
	if a.settings.ImpactDepth != config.DefaultImpactDepth-1 {
		t.Fatalf("depth = %d", a.settings.ImpactDepth)
	}
}

func TestSettingsPageRendersSaveError(t *testing.T) {
	a := NewApp(nil)
	a.settingsErr = errors.New("attempt to write a readonly database")
	tt := ui.NewTester(func(c *ui.Context) { a.settingsPage(c) }, 900, 600)
	tt.Frame()
	if !tt.HasText("readonly database") {
		t.Fatalf("error not surfaced, texts = %v", tt.Texts())
	}
}
