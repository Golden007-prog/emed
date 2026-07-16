package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func newSearchBarTest(t *testing.T) (*tview.Application, *SearchBar) {
	t.Helper()

	app := tview.NewApplication()
	sb := NewSearchBar()
	app.SetRoot(sb, true)
	app.SetFocus(sb)
	return app, sb
}

func sendBarKey(app *tview.Application, sb *SearchBar, key tcell.Key, r rune, mods tcell.ModMask) {
	sb.InputHandler()(tcell.NewEventKey(key, r, mods), func(p tview.Primitive) {
		app.SetFocus(p)
	})
}

func TestSearchBarTabSwitchesFields(t *testing.T) {
	app, sb := newSearchBarTest(t)

	if !sb.searchInput.HasFocus() {
		t.Fatal("search input should start focused")
	}

	sendBarKey(app, sb, tcell.KeyTab, 0, tcell.ModNone)
	if !sb.replaceInput.HasFocus() || sb.searchInput.HasFocus() {
		t.Fatal("Tab should move focus to the replace input")
	}

	sendBarKey(app, sb, tcell.KeyTab, 0, tcell.ModNone)
	if !sb.searchInput.HasFocus() {
		t.Fatal("Tab should cycle back to the search input")
	}
}

func TestSearchBarAltTogglesFireSearch(t *testing.T) {
	app, sb := newSearchBarTest(t)

	var gotCase, gotWord, gotRegex bool
	calls := 0
	sb.SetCallbacks(func(text string, caseSensitive, wholeWord, useRegex bool) {
		calls++
		gotCase, gotWord, gotRegex = caseSensitive, wholeWord, useRegex
	}, nil, nil, nil, nil, nil)

	sendBarKey(app, sb, tcell.KeyRune, 'c', tcell.ModAlt)
	if calls != 1 || !gotCase {
		t.Fatalf("Alt+C should re-search with case sensitive on, calls=%d case=%v", calls, gotCase)
	}

	sendBarKey(app, sb, tcell.KeyRune, 'r', tcell.ModAlt)
	if calls != 2 || !gotRegex {
		t.Fatal("Alt+R should re-search with regex on")
	}
	if gotWord {
		t.Fatal("whole word should still be off")
	}
}

func TestSearchBarEnterBehaviors(t *testing.T) {
	app, sb := newSearchBarTest(t)

	nextCalls, prevCalls, replaceCalls := 0, 0, 0
	sb.SetCallbacks(nil,
		func(s, r string) { replaceCalls++ },
		nil,
		func() { nextCalls++ },
		func() { prevCalls++ },
		nil)

	sendBarKey(app, sb, tcell.KeyEnter, 0, tcell.ModNone)
	if nextCalls != 1 {
		t.Fatal("Enter in search field should trigger next match")
	}

	sendBarKey(app, sb, tcell.KeyEnter, 0, tcell.ModShift)
	if prevCalls != 1 {
		t.Fatal("Shift+Enter in search field should trigger previous match")
	}

	sendBarKey(app, sb, tcell.KeyTab, 0, tcell.ModNone)
	sendBarKey(app, sb, tcell.KeyEnter, 0, tcell.ModNone)
	if replaceCalls != 1 {
		t.Fatal("Enter in replace field should trigger replace")
	}
}
