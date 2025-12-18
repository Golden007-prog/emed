package ui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// SearchBar represents the search/replace bar for the code editor.
type SearchBar struct {
	*tview.Flex
	form          *tview.Form
	searchInput   *tview.InputField
	replaceInput  *tview.InputField
	matchCounter  *tview.TextView
	caseSensitive *tview.Checkbox
	wholeWord     *tview.Checkbox
	useRegex      *tview.Checkbox
	
	currentMatch int
	totalMatches int
	
	onSearch      func(text string, caseSensitive, wholeWord, useRegex bool)
	onReplace     func(searchText, replaceText string)
	onReplaceAll  func(searchText, replaceText string)
	onNext        func()
	onPrevious    func()
	onClose       func()
}

// NewSearchBar creates a new search bar component.
func NewSearchBar() *SearchBar {
	sb := &SearchBar{
		Flex:         tview.NewFlex().SetDirection(tview.FlexRow),
		matchCounter: tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignRight),
	}
	
	sb.form = tview.NewForm()
	sb.form.SetBorder(true)
	sb.form.SetBorderColor(tcell.NewRGBColor(102, 102, 102))
	sb.form.SetTitle(" Find & Replace ")
	sb.form.SetTitleAlign(tview.AlignLeft)
	sb.form.SetBackgroundColor(tcell.ColorBlack)
	sb.form.SetFieldBackgroundColor(tcell.ColorBlack)
	sb.form.SetButtonBackgroundColor(tcell.ColorBlack)
	
	sb.searchInput = tview.NewInputField().
		SetFieldBackgroundColor(tcell.ColorBlack).
		SetChangedFunc(func(text string) {
			if sb.onSearch != nil {
				sb.onSearch(text, sb.caseSensitive.IsChecked(), sb.wholeWord.IsChecked(), sb.useRegex.IsChecked())
			}
		})
	
	sb.replaceInput = tview.NewInputField().
		SetFieldBackgroundColor(tcell.ColorBlack)
	
	sb.caseSensitive = tview.NewCheckbox()
	sb.wholeWord = tview.NewCheckbox()
	sb.useRegex = tview.NewCheckbox()
	
	sb.caseSensitive.SetChangedFunc(func(checked bool) {
		if sb.onSearch != nil && sb.searchInput.GetText() != "" {
			sb.onSearch(sb.searchInput.GetText(), checked, sb.wholeWord.IsChecked(), sb.useRegex.IsChecked())
		}
	})
	
	sb.wholeWord.SetChangedFunc(func(checked bool) {
		if sb.onSearch != nil && sb.searchInput.GetText() != "" {
			sb.onSearch(sb.searchInput.GetText(), sb.caseSensitive.IsChecked(), checked, sb.useRegex.IsChecked())
		}
	})
	
	sb.useRegex.SetChangedFunc(func(checked bool) {
		if sb.onSearch != nil && sb.searchInput.GetText() != "" {
			sb.onSearch(sb.searchInput.GetText(), sb.caseSensitive.IsChecked(), sb.wholeWord.IsChecked(), checked)
		}
	})
	
	sb.form.
		AddInputField("Find: ", "", 0, nil, nil).
		AddInputField("Replace: ", "", 0, nil, nil).
		AddCheckbox("Case sensitive ", false, nil).
		AddCheckbox("Whole word ", false, nil).
		AddCheckbox("Regex ", false, nil).
		AddButton("Find Next", func() {
			if sb.onNext != nil {
				sb.onNext()
			}
		}).
		AddButton("Find Previous", func() {
			if sb.onPrevious != nil {
				sb.onPrevious()
			}
		}).
		AddButton("Replace", func() {
			if sb.onReplace != nil {
				sb.onReplace(sb.searchInput.GetText(), sb.replaceInput.GetText())
			}
		}).
		AddButton("Replace All", func() {
			if sb.onReplaceAll != nil {
				sb.onReplaceAll(sb.searchInput.GetText(), sb.replaceInput.GetText())
			}
		})
	
	formItem0 := sb.form.GetFormItemByLabel("Find: ")
	sb.searchInput = formItem0.(*tview.InputField)
	sb.searchInput.SetChangedFunc(func(text string) {
		if sb.onSearch != nil {
			sb.onSearch(text, sb.caseSensitive.IsChecked(), sb.wholeWord.IsChecked(), sb.useRegex.IsChecked())
		}
	})
	
	formItem1 := sb.form.GetFormItemByLabel("Replace: ")
	sb.replaceInput = formItem1.(*tview.InputField)
	
	formItem2 := sb.form.GetFormItemByLabel("Case sensitive ")
	sb.caseSensitive = formItem2.(*tview.Checkbox)
	sb.caseSensitive.SetChangedFunc(func(checked bool) {
		if sb.onSearch != nil && sb.searchInput.GetText() != "" {
			sb.onSearch(sb.searchInput.GetText(), checked, sb.wholeWord.IsChecked(), sb.useRegex.IsChecked())
		}
	})
	
	formItem3 := sb.form.GetFormItemByLabel("Whole word ")
	sb.wholeWord = formItem3.(*tview.Checkbox)
	sb.wholeWord.SetChangedFunc(func(checked bool) {
		if sb.onSearch != nil && sb.searchInput.GetText() != "" {
			sb.onSearch(sb.searchInput.GetText(), sb.caseSensitive.IsChecked(), checked, sb.useRegex.IsChecked())
		}
	})
	
	formItem4 := sb.form.GetFormItemByLabel("Regex ")
	sb.useRegex = formItem4.(*tview.Checkbox)
	sb.useRegex.SetChangedFunc(func(checked bool) {
		if sb.onSearch != nil && sb.searchInput.GetText() != "" {
			sb.onSearch(sb.searchInput.GetText(), sb.caseSensitive.IsChecked(), sb.wholeWord.IsChecked(), checked)
		}
	})
	
	sb.form.SetHorizontal(false)
	sb.form.SetItemPadding(0)
	
	counterContainer := tview.NewFlex().
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(sb.matchCounter, 20, 0, false).
		AddItem(tview.NewBox(), 1, 0, false)
	
	sb.Flex.
		AddItem(sb.form, 0, 1, true).
		AddItem(counterContainer, 1, 0, false)
	
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
		sb.matchCounter.SetText("[gray]No matches[-]")
	} else {
		sb.matchCounter.SetText(fmt.Sprintf("[yellow]%d of %d[-]", sb.currentMatch, sb.totalMatches))
	}
}

// Focus sets focus to the search input.
func (sb *SearchBar) Focus(delegate func(p tview.Primitive)) {
	delegate(sb.form)
}

// InputHandler handles key events for the search bar.
func (sb *SearchBar) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return sb.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		switch event.Key() {
		case tcell.KeyEscape:
			if sb.onClose != nil {
				sb.onClose()
			}
			return
		case tcell.KeyEnter:
			if sb.searchInput.HasFocus() {
				if event.Modifiers() == tcell.ModShift {
					if sb.onPrevious != nil {
						sb.onPrevious()
					}
				} else {
					if sb.onNext != nil {
						sb.onNext()
					}
				}
				return
			}
		case tcell.KeyF3:
			if event.Modifiers() == tcell.ModShift {
				if sb.onPrevious != nil {
					sb.onPrevious()
				}
			} else {
				if sb.onNext != nil {
					sb.onNext()
				}
			}
			return
		}
		
		sb.form.InputHandler()(event, setFocus)
	})
}