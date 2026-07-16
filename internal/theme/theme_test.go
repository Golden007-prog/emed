package theme

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestInitFixedColors(t *testing.T) {
	Init(false)

	if Current.AppBg != tcell.ColorBlack {
		t.Errorf("expected black app background, got %v", Current.AppBg)
	}
	if Current.TermDefaultBg != tcell.ColorBlack {
		t.Errorf("expected black terminal default background, got %v", Current.TermDefaultBg)
	}
}

func TestInitTerminalBackground(t *testing.T) {
	Init(true)
	defer Init(false)

	if Current.AppBg != tcell.ColorDefault {
		t.Errorf("expected terminal default app background, got %v", Current.AppBg)
	}
	if Current.TermDefaultBg != tcell.ColorDefault {
		t.Errorf("expected terminal default bg passthrough, got %v", Current.TermDefaultBg)
	}
	if Current.TermDefaultFg != tcell.ColorDefault {
		t.Errorf("expected terminal default fg passthrough, got %v", Current.TermDefaultFg)
	}
	if Current.ModalBg != tcell.ColorBlack {
		t.Errorf("modals must stay opaque, got %v", Current.ModalBg)
	}
	if Current.StatusBarBg == tcell.ColorDefault {
		t.Error("status bar must keep its contrast band")
	}
}
