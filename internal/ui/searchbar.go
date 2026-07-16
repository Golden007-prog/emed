package ui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/theme"
	"github.com/rivo/tview"
)

// searchBarHeight is the total height of the bar including its border.
const searchBarHeight = 4

// SearchBar is the compact find/replace bar for the code editor: two
// rows with inline toggles and match counter.
type SearchBar struct {
	*tview.Flex
	searchInput  *tview.InputField
	replaceInput *tview.InputField
	toggles      *searchToggles
	matchCounter *tview.TextView
	actions      *tview.TextView

	currentMatch int
	totalMatches int

	onSearch     func(text string, caseSensitive, wholeWord, useRegex bool)
	onReplace    func(searchText, replaceText string)
	onReplaceAll func(searchText, replaceText string)
	onNext       func()
	onPrevious   func()
	onClose      func()
}

// NewSearchBar creates a new search bar component.
func NewSearchBar() *SearchBar {
	sb := &SearchBar{
		Flex: tview.NewFlex().SetDirection(tview.FlexRow),
	}

	bg := theme.Current.AppBg
	sb.Flex.SetBackgroundColor(bg)
	sb.Flex.SetBorder(true)
	sb.Flex.SetBorderColor(theme.Current.Border)
	sb.Flex.SetTitle(" Find & Replace ")
	sb.Flex.SetTitleColor(theme.Current.Title)
	sb.Flex.SetTitleAlign(tview.AlignLeft)

	newInput := func(label string) *tview.InputField {
		input := tview.NewInputField().
			SetLabel(label).
			SetLabelColor(theme.Current.TextMuted).
			SetFieldBackgroundColor(tcell.NewRGBColor(45, 45, 45)).
			SetFieldTextColor(tcell.ColorWhite)
		return input
	}

	sb.searchInput = newInput("Find    ")
	sb.searchInput.SetChangedFunc(func(text string) {
		sb.triggerSearch()
	})

	sb.replaceInput = newInput("Replace ")

	sb.toggles = newSearchToggles(bg)
	sb.toggles.SetOnChange(func() {
		sb.triggerSearch()
	})

	sb.matchCounter = tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignRight)
	sb.matchCounter.SetBackgroundColor(bg)

	sb.actions = tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignRight)
	sb.actions.SetBackgroundColor(bg)
	sb.actions.SetText("[#666666]Enter replace · Alt+A all[-]")
	sb.actions.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseLeftClick {
			sb.replaceAll()
			return action, nil
		}
		return action, event
	})

	findRow := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(sb.searchInput, 0, 1, true).
		AddItem(tview.NewBox().SetBackgroundColor(bg), 1, 0, false).
		AddItem(sb.toggles, searchTogglesWidth, 0, false).
		AddItem(sb.matchCounter, 14, 0, false)
	findRow.SetBackgroundColor(bg)

	replaceRow := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(sb.replaceInput, 0, 1, false).
		AddItem(tview.NewBox().SetBackgroundColor(bg), 1, 0, false).
		AddItem(sb.actions, 28, 0, false)
	replaceRow.SetBackgroundColor(bg)

	sb.Flex.
		AddItem(findRow, 1, 0, true).
		AddItem(replaceRow, 1, 0, false)

	sb.updateMatchCounter()

	return sb
}

// SetSearchText sets the search input text.
func (sb *SearchBar) SetSearchText(text string) {
	sb.searchInput.SetText(text)
}

// SetCallbacks sets the callback functions for search bar actions.
func (sb *SearchBar) SetCallbacks(
	onSearch func(text string, caseSensitive, wholeWord, useRegex bool),
	onReplace func(searchText, replaceText string),
	onReplaceAll func(searchText, replaceText string),
	onNext func(),
	onPrevious func(),
	onClose func(),
) {
	sb.onSearch = onSearch
	sb.onReplace = onReplace
	sb.onReplaceAll = onReplaceAll
	sb.onNext = onNext
	sb.onPrevious = onPrevious
	sb.onClose = onClose
}

// UpdateMatches updates the match counter display.
func (sb *SearchBar) UpdateMatches(current, total int) {
	sb.currentMatch = current
	sb.totalMatches = total
	sb.updateMatchCounter()
}

func (sb *SearchBar) updateMatchCounter() {
	if sb.totalMatches == 0 {
		sb.matchCounter.SetText("[#666666]no matches[-] ")
	} else {
		sb.matchCounter.SetText(fmt.Sprintf("[yellow]%d/%d[-] ", sb.currentMatch, sb.totalMatches))
	}
}

func (sb *SearchBar) triggerSearch() {
	if sb.onSearch == nil {
		return
	}
	caseSensitive, wholeWord, useRegex := sb.toggles.States()
	sb.onSearch(sb.searchInput.GetText(), caseSensitive, wholeWord, useRegex)
}

func (sb *SearchBar) replaceCurrent() {
	if sb.onReplace != nil {
		sb.onReplace(sb.searchInput.GetText(), sb.replaceInput.GetText())
	}
}

func (sb *SearchBar) replaceAll() {
	if sb.onReplaceAll != nil {
		sb.onReplaceAll(sb.searchInput.GetText(), sb.replaceInput.GetText())
	}
}

// Focus sets focus to the search input.
func (sb *SearchBar) Focus(delegate func(p tview.Primitive)) {
	delegate(sb.searchInput)
}

// InputHandler handles key events for the search bar.
func (sb *SearchBar) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return sb.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if event.Modifiers()&tcell.ModAlt != 0 && event.Key() == tcell.KeyRune {
			r := event.Rune()
			if sb.toggles.HandleAltRune(r) {
				return
			}
			if r == 'a' || r == 'A' {
				sb.replaceAll()
				return
			}
		}

		switch event.Key() {
		case tcell.KeyEscape:
			if sb.onClose != nil {
				sb.onClose()
			}
			return
		case tcell.KeyTab, tcell.KeyBacktab:
			if sb.searchInput.HasFocus() {
				setFocus(sb.replaceInput)
			} else {
				setFocus(sb.searchInput)
			}
			return
		case tcell.KeyDown:
			if sb.searchInput.HasFocus() {
				setFocus(sb.replaceInput)
				return
			}
		case tcell.KeyUp:
			if sb.replaceInput.HasFocus() {
				setFocus(sb.searchInput)
				return
			}
		case tcell.KeyEnter:
			if sb.searchInput.HasFocus() {
				if event.Modifiers()&tcell.ModShift != 0 {
					if sb.onPrevious != nil {
						sb.onPrevious()
					}
				} else if sb.onNext != nil {
					sb.onNext()
				}
				return
			}
			if sb.replaceInput.HasFocus() {
				sb.replaceCurrent()
				return
			}
		case tcell.KeyF3:
			if event.Modifiers()&tcell.ModShift != 0 {
				if sb.onPrevious != nil {
					sb.onPrevious()
				}
			} else if sb.onNext != nil {
				sb.onNext()
			}
			return
		}

		sb.Flex.InputHandler()(event, setFocus)
	})
}
