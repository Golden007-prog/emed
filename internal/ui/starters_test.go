package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/project"
	"github.com/rivo/tview"
)

func newStarterFormTest(t *testing.T) (*tview.Application, *starterForm) {
	t.Helper()

	app := tview.NewApplication()
	prevRoot := mainAppRoot
	SetMainAppRoot(tview.NewFlex())
	t.Cleanup(func() {
		SetMainAppRoot(prevRoot)
		modalIsOpen = false
	})

	deps := startersModalDeps{
		app:       app,
		store:     project.NewStore(t.TempDir()),
		isRunning: func(project.Starter) bool { return false },
		onLaunch:  func(project.Starter) {},
		onJump:    func(project.Starter) {},
	}

	return app, showStarterForm(deps, nil)
}

func sendKey(app *tview.Application, root tview.Primitive, key tcell.Key, r rune) {
	if handler := root.InputHandler(); handler != nil {
		handler(tcell.NewEventKey(key, r, tcell.ModNone), func(p tview.Primitive) {
			app.SetFocus(p)
		})
	}
}

func TestStarterFormTabCyclesFocus(t *testing.T) {
	app, form := newStarterFormTest(t)

	if !form.name.HasFocus() {
		t.Fatal("expected the Name field focused initially")
	}

	sendKey(app, form.wrapper, tcell.KeyTab, 0)

	if form.name.HasFocus() {
		t.Fatal("Name field still has focus after Tab, focus is stuck")
	}
	if !form.command.HasFocus() {
		t.Fatal("expected the Command field focused after Tab")
	}

	sendKey(app, form.wrapper, tcell.KeyTab, 0)

	if form.command.HasFocus() {
		t.Fatal("Command field still has focus after second Tab")
	}
	if !form.unique.HasFocus() {
		t.Fatal("expected the Unique checkbox focused after second Tab")
	}
}

func TestStarterFormBacktabGoesBack(t *testing.T) {
	app, form := newStarterFormTest(t)

	sendKey(app, form.wrapper, tcell.KeyTab, 0)
	sendKey(app, form.wrapper, tcell.KeyBacktab, 0)

	if !form.name.HasFocus() || form.command.HasFocus() {
		t.Fatal("Backtab did not return focus to the Name field")
	}
}

func TestStarterFormTypingLandsOnFocusedField(t *testing.T) {
	app, form := newStarterFormTest(t)

	sendKey(app, form.wrapper, tcell.KeyRune, 'a')
	sendKey(app, form.wrapper, tcell.KeyTab, 0)
	sendKey(app, form.wrapper, tcell.KeyRune, 'b')

	if form.name.GetText() != "a" {
		t.Errorf("expected Name field text \"a\", got %q", form.name.GetText())
	}
	if form.command.GetText() != "b" {
		t.Errorf("expected Command field text \"b\", got %q", form.command.GetText())
	}
}
