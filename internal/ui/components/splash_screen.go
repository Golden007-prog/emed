package components

import (
	"math/rand"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// SplashScreen handles the no-documents view with ASCII art
type SplashScreen struct {
	taglines []string
}

// NewSplashScreen creates a new splash screen component
func NewSplashScreen() *SplashScreen {
	return &SplashScreen{
		taglines: []string{
			"Efficiently Manage, Elegantly Develop.",
			"Edit More, Endlessly Debug.",
			"Every Method, Elegantly Documented.",
			"Effortlessly Merge, Easily Diff.",
			"Escape Mainframe, Enter DOS.",
			"Edit. Modify. Enhance. Deploy.",
			"Even More Errors Detected.",

			"Emmanuel's Masterfully Engineered Distraction.",
			"Eventually, My Editor Dominates.",
			"Erase My Endless Despair.",
			"Every Mistake, Eventually Discovered.",
			"Exists Mainly Expecting Donations.",
			"Emed Means Emed, Duh.",

			"Eating Many Electric Donuts.",
			"Existential Machine, Endless Dread.",
			"Every Macro Evokes Destiny.",
			"Entertaining My Electric Duck.",
			"Even My Editor Dreams.",
			"Exploding Monkeys, Every Day.",
			"Enter My Elegant Dungeon.",

			// Spanglish mix
			"Editor Magnífico, Excelente Diseño.",
			"Edita Más, Escribe Doble.",
			"El Mundo Es Digital.",
			"Error Mayor, Error Digital.",
			"Este Modo Es Diferente.",
			"El Mejor Editor, Definitivo.",
			"Escribe Más, Extra Despacio.",
			"Emmanuel's Muy Especial Device.",
			"Entra, Modifica, Elimina, Done.",
			"El Macho Editor Definitivo.",
		},
	}
}

// CreateNoDocsView creates the view shown when no documents are open
func (s *SplashScreen) CreateNoDocsView() tview.Primitive {
	// Create no documents view with blue ASCII art
	helpText := `[blue]███████╗███╗   ███╗███████╗██████╗ 
██╔════╝████╗ ████║██╔════╝██╔══██╗
█████╗  ██╔████╔██║█████╗  ██║  ██║
██╔══╝  ██║╚██╔╝██║██╔══╝  ██║  ██║
███████╗██║ ╚═╝ ██║███████╗██████╔╝
╚══════╝╚═╝     ╚═╝╚══════╝╚═════╝[-]
[darkgray]{tagline}
made with ♥ in Sonora[-]

[gray]Ayuda / Help: Alt+H / F1[-]
`

	// Select a tagline
	tagline := s.getRandomTagline()

	// Replace {tagline} with actual tagline
	helpText = strings.ReplaceAll(helpText, "{tagline}", tagline)

	// Create the text view for the help text
	helpTextView := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText(helpText).
		SetDynamicColors(true)
	helpTextView.SetBackgroundColor(tcell.ColorBlack)
	
	// Make the text view ignore all mouse events
	helpTextView.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		// Return nil to prevent the mouse event from being processed
		return action, nil
	})

	// Create spacer boxes with black background
	topSpacer := tview.NewBox().SetBackgroundColor(tcell.ColorBlack)
	bottomSpacer := tview.NewBox().SetBackgroundColor(tcell.ColorBlack)
	
	// Make spacer boxes ignore mouse events
	topSpacer.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		return action, nil
	})
	bottomSpacer.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		return action, nil
	})

	// Create a flex container to center vertically
	centerFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(topSpacer, 0, 1, false).      // Top spacer
		AddItem(helpTextView, 11, 0, false). // ASCII art (fixed height with extra line for rendering)
		AddItem(bottomSpacer, 0, 1, false)   // Bottom spacer
	centerFlex.SetBackgroundColor(tcell.ColorBlack)
	
	// Make the entire flex container ignore mouse events
	centerFlex.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		return action, nil
	})

	return centerFlex
}

// getRandomTagline returns a random tagline, with 10% chance of default
func (s *SplashScreen) getRandomTagline() string {
	if rand.Intn(10) == 0 {
		return "Emmanuel's Editor"
	}
	return s.taglines[rand.Intn(len(s.taglines))]
}