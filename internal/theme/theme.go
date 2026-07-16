// Package theme centralizes the colors used across the emed UI as
// semantic roles, so surfaces stay consistent and the palette can be
// switched in one place.
package theme

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ShowGutterSeparator toggles the vertical line drawn between the line
// numbers and the code. Set to false to remove the separator.
const ShowGutterSeparator = true

// Theme holds the semantic color roles used by the UI.
type Theme struct {
	// AppBg is the base background for the main surfaces: panels, file
	// tree, editor, tab bars and the embedded terminal chrome. In
	// terminal-background mode it becomes the host terminal default
	// background, so transparency and user themes show through.
	AppBg tcell.Color

	// ModalBg is the background of modal dialogs. It stays opaque even
	// in terminal-background mode, overlays need solid readability.
	ModalBg tcell.Color

	// Border and Title style the frames of panels and modals.
	Border tcell.Color
	Title  tcell.Color

	// TextMuted is for secondary text like tree guides and hints.
	TextMuted tcell.Color

	// StatusBarBg keeps its own contrast band in every mode.
	StatusBarBg tcell.Color

	// GutterText and GutterSeparator style the line numbers column,
	// which shares the editor background.
	GutterText      tcell.Color
	GutterSeparator tcell.Color

	// TermDefaultFg and TermDefaultBg are what the embedded terminal
	// paints when the shell uses its default colors.
	TermDefaultFg tcell.Color
	TermDefaultBg tcell.Color
}

// Current is the active theme. Init replaces it at startup before any
// UI component is built.
var Current = defaultTheme()

func defaultTheme() Theme {
	return Theme{
		AppBg:           tcell.ColorBlack,
		ModalBg:         tcell.ColorBlack,
		Border:          tcell.NewRGBColor(102, 102, 102),
		Title:           tcell.ColorLightGray,
		TextMuted:       tcell.NewRGBColor(102, 102, 102),
		StatusBarBg:     tcell.NewRGBColor(40, 40, 40),
		GutterText:      tcell.NewRGBColor(90, 90, 90),
		GutterSeparator: tcell.NewRGBColor(60, 60, 60),
		TermDefaultFg:   tcell.ColorLightGray,
		TermDefaultBg:   tcell.ColorBlack,
	}
}

// Init configures the active theme. With useTerminalBackground, the
// base backgrounds use the terminal own default colors instead of the
// fixed ones, so semi-transparent terminal setups keep their effect.
// Must be called before building the UI.
func Init(useTerminalBackground bool) {
	Current = defaultTheme()
	if useTerminalBackground {
		Current.AppBg = tcell.ColorDefault
		Current.TermDefaultFg = tcell.ColorDefault
		Current.TermDefaultBg = tcell.ColorDefault
	}

	// Unstyled primitives are mostly modal internals (message views,
	// button rows, spacers), so they fall back to the opaque modal
	// background. App chrome sets AppBg explicitly where transparency
	// should show through.
	tview.Styles.PrimitiveBackgroundColor = Current.ModalBg
	tview.Styles.BorderColor = Current.Border
	tview.Styles.TitleColor = Current.Title
}
