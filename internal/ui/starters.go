package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/project"
	"github.com/laborin/emed/internal/theme"
	"github.com/rivo/tview"
)

// startersModalDeps carries what the starters modal needs from the layout.
type startersModalDeps struct {
	app       *tview.Application
	store     *project.Store
	isRunning func(project.Starter) bool
	onLaunch  func(project.Starter)
	onJump    func(project.Starter)
}

// ShowStartersModal displays the terminal starters selector for the project.
func ShowStartersModal(deps startersModalDeps) {
	starters, err := deps.store.LoadStarters()
	if err != nil {
		ShowAlertModal(deps.app, "Starters", fmt.Sprintf("Cannot read starters config: %v", err))
		return
	}

	height := len(starters) + 9
	if height < 12 {
		height = 12
	}
	if height > 22 {
		height = 22
	}

	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      64,
		FixedHeight:     height,
		BackgroundColor: theme.Current.ModalBg,
		BorderColor:     theme.Current.Border,
		TitleColor:      theme.Current.Title,
		Title:           " Terminal Starters ",
		TitleAlign:      tview.AlignCenter,
	}

	modal, content := NewConfiguredModal(deps.app, config, true)
	modal.SetPreviousRoot(mainAppRoot)

	table := tview.NewTable()
	table.SetBackgroundColor(theme.Current.ModalBg)
	table.SetSelectable(true, false)
	table.SetSelectedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorLightGray))

	if len(starters) == 0 {
		emptyCell := tview.NewTableCell("No starters in this project yet. Press A to add one.").
			SetTextColor(tcell.NewRGBColor(120, 120, 120)).
			SetSelectable(false).
			SetExpansion(1)
		table.SetCell(0, 0, emptyCell)
		table.SetSelectable(false, false)
	}

	for row, s := range starters {
		running := deps.isRunning(s)

		nameColor := tcell.ColorWhite
		commandColor := tcell.NewRGBColor(140, 140, 140)
		statusColor := tcell.ColorAqua
		if running {
			nameColor = tcell.NewRGBColor(110, 110, 110)
			commandColor = tcell.NewRGBColor(90, 90, 90)
		}

		status := ""
		if s.Unique {
			status = "unique"
		}
		if running {
			status = strings.TrimSpace(status + " (running)")
			statusColor = tcell.NewRGBColor(110, 110, 110)
		}

		command := s.Command
		if runes := []rune(command); len(runes) > 28 {
			command = string(runes[:27]) + "…"
		}

		table.SetCell(row, 0, tview.NewTableCell(" "+s.Name).
			SetTextColor(nameColor).
			SetExpansion(1))
		table.SetCell(row, 1, tview.NewTableCell(command).
			SetTextColor(commandColor).
			SetExpansion(1))
		table.SetCell(row, 2, tview.NewTableCell(status+" ").
			SetTextColor(statusColor).
			SetAlign(tview.AlignRight))
	}

	hints := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText("[gray]↑↓ move · Enter run · A add · D delete · Esc close[-]")
	hints.SetBackgroundColor(theme.Current.ModalBg)

	content.AddItem(tview.NewBox().SetBackgroundColor(theme.Current.ModalBg), 1, 0, false)
	content.AddItem(table, 0, 1, true)
	content.AddItem(hints, 1, 0, false)

	table.SetSelectedFunc(func(row, col int) {
		if row < 0 || row >= len(starters) {
			return
		}
		starter := starters[row]
		modal.Close()
		if deps.isRunning(starter) {
			deps.onJump(starter)
		} else {
			deps.onLaunch(starter)
		}
	})

	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() != tcell.KeyRune {
			return event
		}
		switch event.Rune() {
		case 'a', 'A':
			modal.Close()
			showStarterForm(deps, starters)
			return nil
		case 'd', 'D':
			row, _ := table.GetSelection()
			if row < 0 || row >= len(starters) {
				return nil
			}
			starter := starters[row]
			modal.Close()
			confirmStarterDelete(deps, starters, starter)
			return nil
		}
		return event
	})

	modal.Show()
	deps.app.SetFocus(table)
}

// starterForm holds the add-starter modal pieces, exposed for tests.
type starterForm struct {
	wrapper *modalWrapper
	name    *tview.InputField
	command *tview.InputField
	unique  *tview.Checkbox
}

func showStarterForm(deps startersModalDeps, starters []project.Starter) *starterForm {
	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      56,
		FixedHeight:     13,
		BackgroundColor: theme.Current.ModalBg,
		BorderColor:     theme.Current.Border,
		TitleColor:      theme.Current.Title,
		Title:           " New Starter ",
		TitleAlign:      tview.AlignCenter,
	}

	modal, content := NewConfiguredModal(deps.app, config, true)
	modal.SetPreviousRoot(mainAppRoot)

	errorView := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)
	errorView.SetBackgroundColor(theme.Current.ModalBg)

	form := tview.NewForm()
	form.SetBackgroundColor(theme.Current.ModalBg)
	form.SetLabelColor(tcell.ColorLightGray)
	form.SetFieldBackgroundColor(tcell.NewRGBColor(45, 45, 45))
	form.SetFieldTextColor(tcell.ColorWhite)
	form.SetButtonStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(theme.Current.ModalBg))
	form.SetButtonActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))

	nameField := tview.NewInputField().SetLabel("Name")
	commandField := tview.NewInputField().SetLabel("Command")
	uniqueCheck := tview.NewCheckbox().SetLabel("Unique (one instance)")

	form.AddFormItem(nameField)
	form.AddFormItem(commandField)
	form.AddFormItem(uniqueCheck)

	reopenList := func() {
		ShowStartersModal(deps)
	}

	form.AddButton("Save", func() {
		candidate := project.Starter{
			Name:    strings.TrimSpace(nameField.GetText()),
			Command: strings.TrimSpace(commandField.GetText()),
			Unique:  uniqueCheck.IsChecked(),
		}
		if err := project.ValidateStarter(starters, candidate); err != nil {
			errorView.SetText(fmt.Sprintf("[red]%s[-]", err.Error()))
			return
		}

		if err := deps.store.SaveStarters(append(starters, candidate)); err != nil {
			errorView.SetText(fmt.Sprintf("[red]cannot save: %s[-]", err.Error()))
			return
		}

		modal.Close()
		reopenList()
	})
	form.AddButton("Cancel", func() {
		modal.Cancel()
	})

	modal.modal.SetOnCancel(reopenList)

	content.AddItem(form, 0, 1, true)
	content.AddItem(errorView, 1, 0, false)

	modal.Show()
	deps.app.SetFocus(form)

	return &starterForm{
		wrapper: modal,
		name:    nameField,
		command: commandField,
		unique:  uniqueCheck,
	}
}

func confirmStarterDelete(deps startersModalDeps, starters []project.Starter, starter project.Starter) {
	reopenList := func() {
		ShowStartersModal(deps)
	}

	ShowConfirmModal(deps.app, "Delete Starter",
		fmt.Sprintf("Delete starter \"%s\"?\nThis only removes the config, running terminals are not touched.", starter.Name),
		func() {
			remaining := make([]project.Starter, 0, len(starters))
			for _, s := range starters {
				if s.Name != starter.Name {
					remaining = append(remaining, s)
				}
			}
			if err := deps.store.SaveStarters(remaining); err != nil {
				ShowAlertModal(deps.app, "Starters", fmt.Sprintf("Cannot save starters: %v", err))
				return
			}
			reopenList()
		},
		reopenList,
	)
}
