// Package ui provides the main user interface components for the emed text editor.
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// EditorDialogManager manages editor-related dialogs and user interactions.
type EditorDialogManager struct {
	app *tview.Application
}

// NewEditorDialogManager creates a new editor dialog manager instance.
func NewEditorDialogManager(app *tview.Application) *EditorDialogManager {
	return &EditorDialogManager{
		app: app,
	}
}

// ShowSaveConflictDialog displays a dialog for handling save conflicts.
func (dm *EditorDialogManager) ShowSaveConflictDialog(
	filePath string,
	onOverwrite func(),
	onSaveAsCopy func(string),
	onCancel func(),
) {
	if dm.app == nil {
		return
	}

	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      60,
		FixedHeight:     12,
		BackgroundColor: tcell.ColorBlack,
		BorderColor:     tcell.NewRGBColor(102, 102, 102),
		TitleColor:      tcell.ColorLightGray,
		Title:           " File Conflict ",
		TitleAlign:      tview.AlignCenter,
	}

	modal, content := NewConfiguredModal(dm.app, config, true)
	modal.SetPreviousRoot(GetMainAppRoot())

	messageText := fmt.Sprintf(
		"The file '%s' has been modified externally.\n\n"+
			"Saving will overwrite the external changes.\n\n"+
			"What would you like to do?",
		filepath.Base(filePath))

	message := tview.NewTextView().
		SetText(messageText).
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true)

	buttonsFlex := tview.NewFlex().SetDirection(tview.FlexColumn)

	overwriteBtn := tview.NewButton("Overwrite").
		SetSelectedFunc(func() {
			onOverwrite()
			modal.Close()
		})
	overwriteBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	overwriteBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))

	saveCopyBtn := tview.NewButton("Save as Copy").
		SetSelectedFunc(func() {
			base := strings.TrimSuffix(filePath, filepath.Ext(filePath))
			ext := filepath.Ext(filePath)
			copyPath := base + "_copy" + ext

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

	cancelBtn := tview.NewButton("Cancel").
		SetSelectedFunc(func() {
			onCancel()
			modal.Close()
		})
	cancelBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	cancelBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))

	buttonsFlex.
		AddItem(nil, 0, 1, false).
		AddItem(overwriteBtn, 12, 0, true).
		AddItem(nil, 2, 0, false).
		AddItem(saveCopyBtn, 14, 0, false).
		AddItem(nil, 2, 0, false).
		AddItem(cancelBtn, 10, 0, false).
		AddItem(nil, 0, 1, false)

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

	content.
		AddItem(nil, 1, 0, false).
		AddItem(message, 0, 1, false).
		AddItem(buttonsFlex, 3, 0, true).
		AddItem(nil, 1, 0, false)

	modal.Show()
	dm.app.SetFocus(overwriteBtn)
}

// ShowDeletedFileDialog displays a dialog when a file has been deleted externally.
func (dm *EditorDialogManager) ShowDeletedFileDialog(
	filePath string,
	onCreate func(),
	onDiscard func(),
	onCancel func(),
) {
	if dm.app == nil {
		return
	}

	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      60,
		FixedHeight:     12,
		BackgroundColor: tcell.ColorBlack,
		BorderColor:     tcell.NewRGBColor(102, 102, 102),
		TitleColor:      tcell.ColorLightGray,
		Title:           " File Deleted ",
		TitleAlign:      tview.AlignCenter,
	}

	modal, content := NewConfiguredModal(dm.app, config, true)
	modal.SetPreviousRoot(GetMainAppRoot())

	messageText := fmt.Sprintf(
		"The file '%s' was deleted externally.\n\n"+
			"If you save, you will recreate the file.\n\n"+
			"What would you like to do?",
		filepath.Base(filePath))

	message := tview.NewTextView().
		SetText(messageText).
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true)

	buttonsFlex := tview.NewFlex().SetDirection(tview.FlexColumn)

	createBtn := tview.NewButton("Create").
		SetSelectedFunc(func() {
			onCreate()
			modal.Close()
		})
	createBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	createBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))

	discardBtn := tview.NewButton("Discard").
		SetSelectedFunc(func() {
			onDiscard()
			modal.Close()
		})
	discardBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	discardBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))

	cancelBtn := tview.NewButton("Cancel").
		SetSelectedFunc(func() {
			onCancel()
			modal.Close()
		})
	cancelBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	cancelBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))

	buttonsFlex.
		AddItem(nil, 0, 1, false).
		AddItem(createBtn, 10, 0, true).
		AddItem(nil, 2, 0, false).
		AddItem(discardBtn, 10, 0, false).
		AddItem(nil, 2, 0, false).
		AddItem(cancelBtn, 10, 0, false).
		AddItem(nil, 0, 1, false)

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

	content.
		AddItem(nil, 1, 0, false).
		AddItem(message, 0, 1, false).
		AddItem(buttonsFlex, 3, 0, true).
		AddItem(nil, 1, 0, false)

	modal.Show()
	dm.app.SetFocus(createBtn)
}

// ShowUnsavedChangesDialog displays a dialog for unsaved changes confirmation.
func (dm *EditorDialogManager) ShowUnsavedChangesDialog(fileName string, onYes, onNo func()) {
	if dm.app == nil {
		return
	}

	message := fmt.Sprintf("'%s' has unsaved changes.\n\nDo you want to close it anyway?", fileName)

	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      50,
		FixedHeight:     12,
		BackgroundColor: tcell.ColorBlack,
		BorderColor:     tcell.NewRGBColor(102, 102, 102),
		TitleColor:      tcell.ColorLightGray,
		Title:           " Unsaved Changes ",
		TitleAlign:      tview.AlignCenter,
	}

	modal, contentFlex := NewConfiguredModal(dm.app, config, true)
	modal.SetPreviousRoot(GetMainAppRoot())

	messageView := tview.NewTextView().
		SetText(message).
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true)

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

	contentFlex.
		AddItem(nil, 0, 1, false).
		AddItem(messageView, 0, 2, false).
		AddItem(buttonsFlex, 3, 0, true).
		AddItem(nil, 0, 1, false)

	modal.Show()
	dm.app.SetFocus(yesButton)
}
