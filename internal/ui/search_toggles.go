package ui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// searchTogglesWidth is the fixed width the toggles row needs.
const searchTogglesWidth = 14

// searchToggles renders the [Aa] [W] [.*] indicators shared by both
// searches. They toggle with mouse clicks or Alt+C / Alt+W / Alt+R.
type searchToggles struct {
	*tview.Flex
	caseView  *tview.TextView
	wordView  *tview.TextView
	regexView *tview.TextView

	caseOn  bool
	wordOn  bool
	regexOn bool

	onChange func()
}

func newSearchToggles(bg tcell.Color) *searchToggles {
	st := &searchToggles{
		Flex: tview.NewFlex().SetDirection(tview.FlexColumn),
	}
	st.Flex.SetBackgroundColor(bg)

	newView := func() *tview.TextView {
		v := tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignCenter)
		v.SetBackgroundColor(bg)
		return v
	}
	st.caseView = newView()
	st.wordView = newView()
	st.regexView = newView()

	click := func(v *tview.TextView, toggle func()) {
		v.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
			if action == tview.MouseLeftClick {
				toggle()
				return action, nil
			}
			return action, event
		})
	}
	click(st.caseView, func() { st.toggleCase() })
	click(st.wordView, func() { st.toggleWord() })
	click(st.regexView, func() { st.toggleRegex() })

	st.Flex.
		AddItem(st.caseView, 4, 0, false).
		AddItem(st.wordView, 4, 0, false).
		AddItem(st.regexView, 4, 0, false).
		AddItem(tview.NewBox().SetBackgroundColor(bg), 0, 1, false)

	st.render()
	return st
}

// SetOnChange registers the callback fired after any toggle changes.
func (st *searchToggles) SetOnChange(handler func()) {
	st.onChange = handler
}

// States returns the current case/word/regex flags.
func (st *searchToggles) States() (caseSensitive, wholeWord, useRegex bool) {
	return st.caseOn, st.wordOn, st.regexOn
}

// SetStates sets the flags without firing the change callback.
func (st *searchToggles) SetStates(caseSensitive, wholeWord, useRegex bool) {
	st.caseOn = caseSensitive
	st.wordOn = wholeWord
	st.regexOn = useRegex
	st.render()
}

// HandleAltRune toggles a flag for Alt+C / Alt+W / Alt+R and reports
// if the rune was one of the toggle shortcuts.
func (st *searchToggles) HandleAltRune(r rune) bool {
	switch r {
	case 'c', 'C':
		st.toggleCase()
	case 'w', 'W':
		st.toggleWord()
	case 'r', 'R':
		st.toggleRegex()
	default:
		return false
	}
	return true
}

func (st *searchToggles) toggleCase() {
	st.caseOn = !st.caseOn
	st.changed()
}

func (st *searchToggles) toggleWord() {
	st.wordOn = !st.wordOn
	st.changed()
}

func (st *searchToggles) toggleRegex() {
	st.regexOn = !st.regexOn
	st.changed()
}

func (st *searchToggles) changed() {
	st.render()
	if st.onChange != nil {
		st.onChange()
	}
}

func (st *searchToggles) render() {
	render := func(v *tview.TextView, label string, on bool) {
		if on {
			v.SetText(fmt.Sprintf("[black:yellow]%s[-:-]", label))
		} else {
			v.SetText(fmt.Sprintf("[#666666]%s[-]", label))
		}
	}
	render(st.caseView, "Aa", st.caseOn)
	render(st.wordView, "W", st.wordOn)
	render(st.regexView, ".*", st.regexOn)
}
