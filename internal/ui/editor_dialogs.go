package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// EditorDialogManager handles dialogs specific to the editor
type EditorDialogManager struct {
	app *tview.Application
}

// NewEditorDialogManager creates a new editor dialog manager
func NewEditorDialogManager(app *tview.Application) *EditorDialogManager {
	return &EditorDialogManager{
		app: app,
	}
}

// ShowSaveConflictDialog shows a dialog when trying to save an externally modified file
func (dm *EditorDialogManager) ShowSaveConflictDialog(
	filePath string,
	onOverwrite func(),
	onSaveAsCopy func(string),
	onCancel func(),
) {
	if dm.app == nil {
		return
	}

	// Create modal configuration
	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      60,
		FixedHeight:     12,
		BackgroundColor: tcell.ColorBlack,
		BorderColor:     tcell.ColorGray,
		TitleColor:      tcell.ColorLightGray,
		Title:           " File Conflict ",
		TitleAlign:      tview.AlignCenter,
	}

	modal, content := NewConfiguredModal(dm.app, config, true)
	modal.SetPreviousRoot(GetMainAppRoot())

	// Create message
	messageText := fmt.Sprintf(
		"The file '%s' has been modified externally.\n\n"+
			"Saving will overwrite the external changes.\n\n"+
			"What would you like to do?",
		filepath.Base(filePath))

	message := tview.NewTextView().
		SetText(messageText).
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true)

	// Create buttons
	buttonsFlex := tview.NewFlex().SetDirection(tview.FlexColumn)

	// Overwrite button
	overwriteBtn := tview.NewButton("Overwrite").
		SetSelectedFunc(func() {
			onOverwrite()
			modal.Close()
		})
	overwriteBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	overwriteBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))

	// Save as Copy button
	saveCopyBtn := tview.NewButton("Save as Copy").
		SetSelectedFunc(func() {
			// Generate copy path
			base := strings.TrimSuffix(filePath, filepath.Ext(filePath))
			ext := filepath.Ext(filePath)
			copyPath := base + "_copy" + ext

			// Find unique name
			counter := 1
			for {
				if _, err := os.Stat(copyPath); os.IsNotExist(err) {
					break
				}
				copyPath = fmt.Sprintf("%s_copy%d%s", base, counter, ext)
				counter++
			}

			onSaveAsCopy(copyPath)
			modal.Close()
		})
	saveCopyBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	saveCopyBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))

	// Cancel button
	cancelBtn := tview.NewButton("Cancel").
		SetSelectedFunc(func() {
			onCancel()
			modal.Close()
		})
	cancelBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	cancelBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))

	// Add buttons to flex
	buttonsFlex.
		AddItem(nil, 0, 1, false).
		AddItem(overwriteBtn, 12, 0, true).
		AddItem(nil, 2, 0, false).
		AddItem(saveCopyBtn, 14, 0, false).
		AddItem(nil, 2, 0, false).
		AddItem(cancelBtn, 10, 0, false).
		AddItem(nil, 0, 1, false)

	// Set up button navigation
	overwriteBtn.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			dm.app.SetFocus(saveCopyBtn)
			return nil
		case tcell.KeyLeft:
			dm.app.SetFocus(cancelBtn)
			return nil
		}
		return event
	})

	saveCopyBtn.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			dm.app.SetFocus(cancelBtn)
			return nil
		case tcell.KeyLeft:
			dm.app.SetFocus(overwriteBtn)
			return nil
		}
		return event
	})

	cancelBtn.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			dm.app.SetFocus(overwriteBtn)
			return nil
		case tcell.KeyLeft:
			dm.app.SetFocus(saveCopyBtn)
			return nil
		}
		return event
	})

	// Add to content
	content.
		AddItem(nil, 1, 0, false).
		AddItem(message, 0, 1, false).
		AddItem(buttonsFlex, 3, 0, true).
		AddItem(nil, 1, 0, false)

	// Show modal
	modal.Show()
	dm.app.SetFocus(overwriteBtn)
}

// ShowDeletedFileDialog shows a dialog when trying to save a file that was deleted externally
func (dm *EditorDialogManager) ShowDeletedFileDialog(
	filePath string,
	onCreate func(),
	onDiscard func(),
	onCancel func(),
) {
	if dm.app == nil {
		return
	}

	// Create modal configuration
	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      60,
		FixedHeight:     12,
		BackgroundColor: tcell.ColorBlack,
		BorderColor:     tcell.ColorGray,
		TitleColor:      tcell.ColorLightGray,
		Title:           " File Deleted ",
		TitleAlign:      tview.AlignCenter,
	}

	modal, content := NewConfiguredModal(dm.app, config, true)
	modal.SetPreviousRoot(GetMainAppRoot())

	// Create message
	messageText := fmt.Sprintf(
		"The file '%s' was deleted externally.\n\n"+
			"If you save, you will recreate the file.\n\n"+
			"What would you like to do?",
		filepath.Base(filePath))

	message := tview.NewTextView().
		SetText(messageText).
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true)

	// Create buttons
	buttonsFlex := tview.NewFlex().SetDirection(tview.FlexColumn)

	// Create button
	createBtn := tview.NewButton("Create").
		SetSelectedFunc(func() {
			onCreate()
			modal.Close()
		})
	createBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	createBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))

	// Discard button
	discardBtn := tview.NewButton("Discard").
		SetSelectedFunc(func() {
			onDiscard()
			modal.Close()
		})
	discardBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	discardBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))

	// Cancel button
	cancelBtn := tview.NewButton("Cancel").
		SetSelectedFunc(func() {
			onCancel()
			modal.Close()
		})
	cancelBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	cancelBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))

	// Add buttons to flex
	buttonsFlex.
		AddItem(nil, 0, 1, false).
		AddItem(createBtn, 10, 0, true).
		AddItem(nil, 2, 0, false).
		AddItem(discardBtn, 10, 0, false).
		AddItem(nil, 2, 0, false).
		AddItem(cancelBtn, 10, 0, false).
		AddItem(nil, 0, 1, false)

	// Set up button navigation
	createBtn.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			dm.app.SetFocus(discardBtn)
			return nil
		case tcell.KeyLeft:
			dm.app.SetFocus(cancelBtn)
			return nil
		}
		return event
	})

	discardBtn.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			dm.app.SetFocus(cancelBtn)
			return nil
		case tcell.KeyLeft:
			dm.app.SetFocus(createBtn)
			return nil
		}
		return event
	})

	cancelBtn.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			dm.app.SetFocus(createBtn)
			return nil
		case tcell.KeyLeft:
			dm.app.SetFocus(discardBtn)
			return nil
		}
		return event
	})

	// Add to content
	content.
		AddItem(nil, 1, 0, false).
		AddItem(message, 0, 1, false).
		AddItem(buttonsFlex, 3, 0, true).
		AddItem(nil, 1, 0, false)

	// Show modal
	modal.Show()
	dm.app.SetFocus(createBtn)
}

// ShowUnsavedChangesDialog shows a confirmation dialog for unsaved changes
func (dm *EditorDialogManager) ShowUnsavedChangesDialog(fileName string, onYes, onNo func()) {
	if dm.app == nil {
		return
	}

	message := fmt.Sprintf("'%s' has unsaved changes.\n\nDo you want to close it anyway?", fileName)
	
	// Create modal configuration with fixed size
	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      50,
		FixedHeight:     12,
		BackgroundColor: tcell.ColorBlack,
		BorderColor:     tcell.ColorGray,
		TitleColor:      tcell.ColorLightGray,
		Title:           " Unsaved Changes ",
		TitleAlign:      tview.AlignCenter,
	}

	modal, contentFlex := NewConfiguredModal(dm.app, config, true)
	modal.SetPreviousRoot(GetMainAppRoot())

	// Create message text
	messageView := tview.NewTextView().
		SetText(message).
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true)

	// Create buttons
	buttonsFlex := tview.NewFlex().SetDirection(tview.FlexColumn)

	yesButton := tview.NewButton("YES").
		SetSelectedFunc(func() {
			onYes()
			modal.Close()
		})
	yesButton.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	yesButton.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))

	noButton := tview.NewButton("NO").
		SetSelectedFunc(func() {
			onNo()
			modal.Close()
		})
	noButton.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	noButton.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))

	// Set up button navigation
	yesButton.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			dm.app.SetFocus(noButton)
			return nil
		case tcell.KeyLeft:
			dm.app.SetFocus(noButton)
			return nil
		}
		return event
	})

	noButton.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			dm.app.SetFocus(yesButton)
			return nil
		case tcell.KeyLeft:
			dm.app.SetFocus(yesButton)
			return nil
		}
		return event
	})

	buttonsFlex.
		AddItem(nil, 0, 1, false).
		AddItem(yesButton, 10, 0, true).
		AddItem(nil, 2, 0, false).
		AddItem(noButton, 10, 0, false).
		AddItem(nil, 0, 1, false)

	// Add message and buttons to content flex
	contentFlex.
		AddItem(nil, 0, 1, false).
		AddItem(messageView, 0, 2, false).
		AddItem(buttonsFlex, 3, 0, true).
		AddItem(nil, 0, 1, false)

	// Show modal
	modal.Show()
	dm.app.SetFocus(yesButton)
}