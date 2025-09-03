package components

import (
	"math/rand"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// SplashScreen displays the application splash screen with random taglines.
type SplashScreen struct {
	taglines []string
}

// NewSplashScreen creates a new splash screen with predefined taglines.
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

// CreateNoDocsView creates the view shown when no documents are open.
func (s *SplashScreen) CreateNoDocsView() tview.Primitive {
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

	tagline := s.getRandomTagline()

	helpText = strings.ReplaceAll(helpText, "{tagline}", tagline)

	helpTextView := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText(helpText).
		SetDynamicColors(true)
	helpTextView.SetBackgroundColor(tcell.ColorBlack)
	
	helpTextView.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		return action, nil
	})

	topSpacer := tview.NewBox().SetBackgroundColor(tcell.ColorBlack)
	bottomSpacer := tview.NewBox().SetBackgroundColor(tcell.ColorBlack)
	
	topSpacer.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		return action, nil
	})
	bottomSpacer.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		return action, nil
	})

	centerFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(topSpacer, 0, 1, false).
		AddItem(helpTextView, 11, 0, false).
		AddItem(bottomSpacer, 0, 1, false)
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