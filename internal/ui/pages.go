package ui

import "github.com/rivo/tview"

func preparePagesForMutation(pages *tview.Pages, app *tview.Application) {
	if pages == nil || !pages.HasFocus() {
		return
	}
	if app != nil {
		pages.Focus(func(p tview.Primitive) {
			app.SetFocus(p)
		})
	} else {
		pages.Focus(func(tview.Primitive) {})
	}
}

func addPageSafely(pages *tview.Pages, name string, item tview.Primitive, resize, visible bool, app *tview.Application) {
	if pages == nil {
		return
	}
	preparePagesForMutation(pages, app)
	pages.AddPage(name, item, resize, visible)
}

func switchToPageSafely(pages *tview.Pages, name string, app *tview.Application) {
	if pages == nil {
		return
	}
	preparePagesForMutation(pages, app)
	pages.SwitchToPage(name)
}

func removePageSafely(pages *tview.Pages, name string, app *tview.Application) {
	if pages == nil {
		return
	}
	preparePagesForMutation(pages, app)
	pages.RemovePage(name)
}
