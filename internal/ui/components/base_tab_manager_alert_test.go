package components

import (
	"strings"
	"testing"

	"github.com/rivo/tview"
)

type fakeTab struct {
	id       int
	title    string
	alerting bool
}

func (f *fakeTab) GetID() int                  { return f.id }
func (f *fakeTab) GetTitle() string            { return f.title }
func (f *fakeTab) GetContent() tview.Primitive { return nil }
func (f *fakeTab) SetModified(bool)            {}
func (f *fakeTab) IsModified() bool            { return false }
func (f *fakeTab) IsPreview() bool             { return false }
func (f *fakeTab) IsAlerting() bool            { return f.alerting }

func overrideBlinkPhase(t *testing.T, on bool) {
	t.Helper()
	orig := alertBlinkPhaseOn
	alertBlinkPhaseOn = func() bool { return on }
	t.Cleanup(func() { alertBlinkPhaseOn = orig })
}

func newManagerWithTabs(tabs ...Tab) *BaseTabManager {
	m := NewBaseTabManager()
	for _, tab := range tabs {
		m.AddTab(tab)
	}
	return m
}

func TestAlertingTabBlinksYellow(t *testing.T) {
	m := newManagerWithTabs(
		&fakeTab{id: 1, title: "one"},
		&fakeTab{id: 2, title: "two", alerting: true},
	)
	m.SwitchToTab(1)

	overrideBlinkPhase(t, true)
	text, _ := m.GenerateTabBarTextWithPositions(0, true)
	if !strings.Contains(text, "[yellow]two") {
		t.Fatalf("alerting tab should render yellow on blink phase, got: %q", text)
	}

	overrideBlinkPhase(t, false)
	text, _ = m.GenerateTabBarTextWithPositions(0, true)
	if strings.Contains(text, "yellow") {
		t.Fatalf("no yellow expected on off phase, got: %q", text)
	}
	if !strings.Contains(text, "[#666666]two") {
		t.Fatalf("alerting tab should render normal color on off phase, got: %q", text)
	}
}

func TestAlertingActiveUnfocusedTabBlinks(t *testing.T) {
	m := newManagerWithTabs(&fakeTab{id: 1, title: "one", alerting: true})
	m.SwitchToTab(1)

	overrideBlinkPhase(t, true)
	text, _ := m.GenerateTabBarTextWithPositions(0, false)
	if !strings.Contains(text, "[yellow][ one ]") {
		t.Fatalf("active unfocused alerting tab should blink yellow, got: %q", text)
	}
}

func TestNonAlertingTabsKeepNormalColors(t *testing.T) {
	m := newManagerWithTabs(
		&fakeTab{id: 1, title: "one"},
		&fakeTab{id: 2, title: "two"},
	)
	m.SwitchToTab(1)

	overrideBlinkPhase(t, true)
	text, _ := m.GenerateTabBarTextWithPositions(0, true)
	if !strings.Contains(text, "[blue][ one ]") {
		t.Fatalf("active focused tab should be blue, got: %q", text)
	}
	if !strings.Contains(text, "[#666666]two") {
		t.Fatalf("inactive tab should be gray, got: %q", text)
	}
}
