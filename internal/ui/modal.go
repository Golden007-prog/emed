// Package ui provides user interface components and managers for the emed editor.
package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var (
	mainAppRoot   tview.Primitive
	modalIsOpen   bool
	previousFocus tview.Primitive
)

// EmModal represents a modal dialog that can overlay the application.
type EmModal struct {
	*tview.Box
	app           *tview.Application
	container     *tview.Box
	content       tview.Primitive
	cancellable   bool
	escPassthrough bool
	onCancel      func()
	onClose       func()
	previousRoot  tview.Primitive
	savedFocus    tview.Primitive
	isWrapped     bool
}

// SetPreviousRoot sets the root primitive to restore when the modal is closed.
func (m *EmModal) SetPreviousRoot(root tview.Primitive) {
	m.previousRoot = root
}

// NewEmModal creates a new modal dialog with the given content.
func NewEmModal(app *tview.Application, content tview.Primitive, cancellable bool) *EmModal {
	m := &EmModal{
		Box:         tview.NewBox(),
		app:         app,
		container:   tview.NewBox().SetBorder(false).SetBackgroundColor(tcell.ColorBlack),
		content:     content,
		cancellable: cancellable,
	}

	return m
}

// SetOnCancel sets the callback for when the modal is cancelled.
func (m *EmModal) SetOnCancel(handler func()) *EmModal {
	m.onCancel = handler
	return m
}

// SetEscPassthrough sets whether ESC key should be passed to content instead of closing modal.
func (m *EmModal) SetEscPassthrough(passthrough bool) *EmModal {
	m.escPassthrough = passthrough
	return m
}

// SetOnClose sets the callback for when the modal is closed.
func (m *EmModal) SetOnClose(handler func()) *EmModal {
	m.onClose = handler
	return m
}

// Show displays the modal dialog.
func (m *EmModal) Show() {
	m.savedFocus = m.app.GetFocus()
	previousFocus = m.savedFocus

	modalIsOpen = true

	m.app.SetRoot(m, true)
	m.app.SetFocus(m.content)
}

// Close closes the modal dialog and restores the previous root.
func (m *EmModal) Close() {
	if m.onClose != nil {
		m.onClose()
	}

	modalIsOpen = false

	if m.previousRoot != nil {
		m.app.SetRoot(m.previousRoot, true)
	} else if mainAppRoot != nil {
		m.app.SetRoot(mainAppRoot, true)
	}

	// Restore previous focus
	if m.savedFocus != nil {
		m.app.SetFocus(m.savedFocus)
	}
}

// Cancel cancels the modal dialog if it's cancellable.
func (m *EmModal) Cancel() {
	if m.cancellable {
		if m.onCancel != nil {
			m.onCancel()
		}
		m.Close()
	}
}

// Draw renders the modal to the screen.
func (m *EmModal) Draw(screen tcell.Screen) {
	screenWidth, screenHeight := screen.Size()

	if m.previousRoot != nil {
		m.previousRoot.SetRect(0, 0, screenWidth, screenHeight)
		m.previousRoot.Draw(screen)
	} else if mainAppRoot != nil {
		mainAppRoot.SetRect(0, 0, screenWidth, screenHeight)
		mainAppRoot.Draw(screen)
	}

	if m.isWrapped {
		m.content.Draw(screen)
		return
	}

	contentWidth, contentHeight := screenWidth/2, screenHeight/2

	if _, ok := m.content.(*tview.Box); ok {
		contentWidth = screenWidth * 6 / 10
		contentHeight = screenHeight * 4 / 10
	}

	x := (screenWidth - contentWidth) / 2
	y := (screenHeight - contentHeight) / 2

	m.content.SetRect(x, y, contentWidth, contentHeight)

	m.content.Draw(screen)
}

// Focus sets focus to the modal content.
func (m *EmModal) Focus(delegate func(p tview.Primitive)) {
	delegate(m.content)
}

// HasFocus returns whether the modal has focus.
func (m *EmModal) HasFocus() bool {
	return m.content.HasFocus()
}

// GetRect returns the modal's rectangle.
func (m *EmModal) GetRect() (int, int, int, int) {
	return m.container.GetRect()
}

func (w *modalWrapper) SetPreviousRoot(root tview.Primitive) {
	w.modal.SetPreviousRoot(root)
}

func (w *modalWrapper) Show() {
	w.modal.savedFocus = w.modal.app.GetFocus()
	previousFocus = w.modal.savedFocus

	modalIsOpen = true

	w.modal.app.SetRoot(w, true)
	w.modal.app.SetFocus(w.modal.content)
}

func (w *modalWrapper) Close() {
	w.modal.Close()
}

func (w *modalWrapper) Cancel() {
	w.modal.Cancel()
}

func (w *modalWrapper) Focus(delegate func(p tview.Primitive)) {
	w.modal.Focus(delegate)
}

func (w *modalWrapper) HasFocus() bool {
	return w.modal.HasFocus()
}

func (w *modalWrapper) GetRect() (int, int, int, int) {
	return w.modal.GetRect()
}

func (w *modalWrapper) SetRect(x, y, width, height int) {
	w.modal.SetRect(x, y, width, height)
}

func (w *modalWrapper) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return w.modal.InputHandler()
}

func (w *modalWrapper) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return w.modal.MouseHandler()
}

func (w *modalWrapper) Blur() {
	w.modal.Blur()
}

func (w *modalWrapper) PasteHandler() func(pastedText string, setFocus func(p tview.Primitive)) {
	return w.modal.PasteHandler()
}

// SetRect sets the modal's rectangle.
func (m *EmModal) SetRect(x, y, width, height int) {
	m.container.SetRect(x, y, width, height)
}

// InputHandler returns the input handler for keyboard events.
func (m *EmModal) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if m.cancellable && !m.escPassthrough && event.Key() == tcell.KeyEscape {
			m.Cancel()
			return
		}

		if handler := m.content.InputHandler(); handler != nil {
			handler(event, setFocus)
		}

	}
}

// MouseHandler returns the mouse handler for mouse events.
func (m *EmModal) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		x, y, width, height := m.content.GetRect()
		mouseX, mouseY := event.Position()

		isInsideContent := mouseX >= x && mouseX < x+width && mouseY >= y && mouseY < y+height

		if isInsideContent {
			if handler := m.content.MouseHandler(); handler != nil {
				consumed, capture = handler(action, event, setFocus)
				setFocus(m.content)
				return consumed, capture
			}
			return true, nil
		}

		if action == tview.MouseLeftClick && m.cancellable {
			m.Cancel()
		}
		return true, nil
	}
}

// EmModalConfig contains configuration options for modal dialogs.
type EmModalConfig struct {
	Width           int
	Height          int
	BackgroundColor tcell.Color
	BorderColor     tcell.Color
	TitleColor      tcell.Color
	Title           string
	TitleAlign      int
	UseFixedSize    bool
	FixedWidth      int
	FixedHeight     int
	EscPassthrough  bool
	UseBreakpoints  bool
}

// NewConfiguredModal creates a new modal with the specified configuration.
func NewConfiguredModal(app *tview.Application, config EmModalConfig, cancellable bool) (*modalWrapper, *tview.Flex) {
	contentFlex := tview.NewFlex().SetDirection(tview.FlexRow)

	var modalContent tview.Primitive

	if config.UseFixedSize {
		bordered := tview.NewFlex().SetDirection(tview.FlexRow)
		bordered.SetBorder(true).
			SetTitle(config.Title).
			SetTitleAlign(config.TitleAlign).
			SetBackgroundColor(tcell.ColorBlack).
			SetBorderColor(config.BorderColor).
			SetTitleColor(config.TitleColor)

		bordered.AddItem(contentFlex, 0, 1, true)
		modalContent = bordered
	} else {
		backgroundBox := tview.NewBox().
			SetBorder(true).
			SetTitle(config.Title).
			SetTitleAlign(config.TitleAlign).
			SetBackgroundColor(tcell.ColorBlack).
			SetBorderColor(config.BorderColor).
			SetTitleColor(config.TitleColor)

		modalContent = &backgroundWithContent{
			Box:        tview.NewBox(),
			background: backgroundBox,
			content:    contentFlex,
		}
	}

	modal := NewEmModal(app, modalContent, cancellable)
	modal.isWrapped = true
	modal.SetEscPassthrough(config.EscPassthrough)

	wrapper := &modalWrapper{
		modal:  modal,
		config: config,
	}

	return wrapper, contentFlex
}

// modalWrapper wraps an EmModal with configuration.
type modalWrapper struct {
	modal  *EmModal
	config EmModalConfig
}

// Draw renders the modal wrapper to the screen.
func (w *modalWrapper) Draw(screen tcell.Screen) {
	screenWidth, screenHeight := screen.Size()

	if w.modal.previousRoot != nil {
		w.modal.previousRoot.SetRect(0, 0, screenWidth, screenHeight)
		w.modal.previousRoot.Draw(screen)
	} else if mainAppRoot != nil {
		mainAppRoot.SetRect(0, 0, screenWidth, screenHeight)
		mainAppRoot.Draw(screen)
	}

	w.drawWithOwnDimensions(screen)
}

func calculateBreakpointDimensions(screenWidth, screenHeight int) (int, int) {
	var widthPercent, heightPercent int

	switch {
	case screenWidth < 80:
		widthPercent = 95
	case screenWidth < 120:
		widthPercent = 85
	case screenWidth < 160:
		widthPercent = 75
	default:
		widthPercent = 65
	}

	switch {
	case screenHeight < 30:
		heightPercent = 95
	case screenHeight < 40:
		heightPercent = 85
	case screenHeight < 50:
		heightPercent = 75
	default:
		heightPercent = 65
	}

	contentWidth := screenWidth * widthPercent / 100
	contentHeight := screenHeight * heightPercent / 100

	if contentWidth < 60 {
		contentWidth = 60
	}
	if contentHeight < 15 {
		contentHeight = 15
	}

	return contentWidth, contentHeight
}

func (w *modalWrapper) drawWithOwnDimensions(screen tcell.Screen) {
	screenWidth, screenHeight := screen.Size()

	if screenWidth <= 0 || screenHeight <= 0 {
		return
	}

	var contentWidth, contentHeight int

	if w.config.UseFixedSize {
		contentWidth = w.config.FixedWidth
		contentHeight = w.config.FixedHeight
	} else if w.config.UseBreakpoints {
		contentWidth, contentHeight = calculateBreakpointDimensions(screenWidth, screenHeight)
	} else {
		width := w.config.Width
		if width <= 0 || width > 100 {
			width = 60
		}
		contentWidth = screenWidth * width / 100

		height := w.config.Height
		if height <= 0 || height > 100 {
			height = 40
		}
		contentHeight = screenHeight * height / 100
	}

	if !w.config.UseFixedSize {
		if contentHeight < 5 {
			contentHeight = 5
		}
		if contentHeight > screenHeight-4 {
			contentHeight = screenHeight - 4
		}
		if contentWidth < 20 {
			contentWidth = 20
		}
		if contentWidth > screenWidth-4 {
			contentWidth = screenWidth - 4
		}
	}

	x := (screenWidth - contentWidth) / 2
	y := (screenHeight - contentHeight) / 2

	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	w.modal.content.SetRect(x, y, contentWidth, contentHeight)

	if w.config.UseFixedSize {
		for row := y; row < y+contentHeight; row++ {
			for col := x; col < x+contentWidth; col++ {
				screen.SetContent(col, row, ' ', nil, tcell.StyleDefault.Background(tcell.ColorBlack))
			}
		}
	}

	w.modal.content.Draw(screen)
}

// SetMainAppRoot sets the main application root primitive.
// SetMainAppRoot sets the main application root primitive.
func SetMainAppRoot(root tview.Primitive) {
	mainAppRoot = root
}

// GetMainAppRoot returns the main application root primitive.
// GetMainAppRoot returns the main application root primitive.
func GetMainAppRoot() tview.Primitive {
	return mainAppRoot
}

// IsModalOpen returns true if a modal dialog is currently open.
// IsModalOpen returns true if a modal dialog is currently open.
func IsModalOpen() bool {
	return modalIsOpen
}

type solidBackground struct {
	*tview.Box
	content tview.Primitive
}

func (s *solidBackground) Draw(screen tcell.Screen) {
	s.Box.DrawForSubclass(screen, s)

	x, y, width, height := s.GetInnerRect()

	s.content.SetRect(x, y, width, height)
	s.content.Draw(screen)
}

func (s *solidBackground) Focus(delegate func(p tview.Primitive)) {
	delegate(s.content)
}

func (s *solidBackground) HasFocus() bool {
	return s.content.HasFocus()
}

func (s *solidBackground) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return s.content.InputHandler()
}

// MouseHandler returns the content's mouse handler
func (s *solidBackground) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return s.content.MouseHandler()
}

// ShowConfirmModal displays a confirmation dialog with Yes/No buttons.
// ShowConfirmModal displays a confirmation dialog with Yes/No buttons.
func ShowConfirmModal(app *tview.Application, title, message string, onConfirm, onCancel func()) {
	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      50,
		FixedHeight:     12,
		BackgroundColor: tcell.ColorBlack,
		BorderColor:     tcell.NewRGBColor(102, 102, 102),
		TitleColor:      tcell.ColorLightGray,
		Title:           " " + title + " ",
		TitleAlign:      tview.AlignCenter,
	}

	modal, contentFlex := NewConfiguredModal(app, config, true)
	modal.SetPreviousRoot(mainAppRoot)

	messageView := tview.NewTextView().
		SetText(message).
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true)

	buttonsFlex := tview.NewFlex().SetDirection(tview.FlexColumn)

	yesButton := tview.NewButton("YES")
	yesButton.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	yesButton.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))

	noButton := tview.NewButton("NO")
	noButton.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	noButton.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))

	leftSpacer := tview.NewBox().SetBackgroundColor(tcell.ColorBlack)
	middleSpacer := tview.NewBox().SetBackgroundColor(tcell.ColorBlack)
	rightSpacer := tview.NewBox().SetBackgroundColor(tcell.ColorBlack)
	topSpacer := tview.NewBox().SetBackgroundColor(tcell.ColorBlack)
	bottomSpacer := tview.NewBox().SetBackgroundColor(tcell.ColorBlack)

	buttonsFlex.
		AddItem(leftSpacer, 0, 1, false).
		AddItem(yesButton, 10, 0, true).
		AddItem(middleSpacer, 2, 0, false).
		AddItem(noButton, 10, 0, false).
		AddItem(rightSpacer, 0, 1, false)

	contentFlex.
		AddItem(topSpacer, 0, 1, false).
		AddItem(messageView, 0, 2, false).
		AddItem(buttonsFlex, 3, 0, true).
		AddItem(bottomSpacer, 0, 1, false)

	yesButton.SetSelectedFunc(func() {
		modal.Close()
		if onConfirm != nil {
			onConfirm()
		}
	})

	noButton.SetSelectedFunc(func() {
		modal.Cancel()
	})

	yesButton.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			app.SetFocus(noButton)
			return nil
		case tcell.KeyLeft:
			app.SetFocus(noButton)
			return nil
		}
		return event
	})

	noButton.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			app.SetFocus(yesButton)
			return nil
		case tcell.KeyLeft:
			app.SetFocus(yesButton)
			return nil
		}
		return event
	})

	modal.modal.SetOnCancel(onCancel)
	modal.Show()
	app.SetFocus(yesButton)
}

type backgroundWithContent struct {
	*tview.Box
	background *tview.Box
	content    tview.Primitive
}

func (b *backgroundWithContent) Draw(screen tcell.Screen) {
	x, y, width, height := b.GetRect()

	// Safety check - if we haven't been given a proper rect, don't draw
	if width <= 0 || height <= 0 {
		return
	}

	// Set the background box to fill the entire area
	b.background.SetRect(x, y, width, height)

	// Draw the background box (with border and filled background)
	b.background.DrawForSubclass(screen, b)

	// Get the inner rect (inside the border)
	innerX, innerY, innerWidth, innerHeight := b.background.GetInnerRect()

	// Fill the background with black
	for row := innerY; row < innerY+innerHeight; row++ {
		for col := innerX; col < innerX+innerWidth; col++ {
			screen.SetContent(col, row, ' ', nil, tcell.StyleDefault.Background(tcell.ColorBlack))
		}
	}

	// Set content rect to the inner area and draw it
	b.content.SetRect(innerX, innerY, innerWidth, innerHeight)
	b.content.Draw(screen)
}

func (b *backgroundWithContent) GetRect() (int, int, int, int) {
	return b.Box.GetRect()
}

func (b *backgroundWithContent) SetRect(x, y, width, height int) {
	b.Box.SetRect(x, y, width, height)
	// Also set the background box rect to match
	if b.background != nil {
		b.background.SetRect(x, y, width, height)
	}
}

func (b *backgroundWithContent) Focus(delegate func(p tview.Primitive)) {
	delegate(b.content)
}

func (b *backgroundWithContent) HasFocus() bool {
	return b.content.HasFocus()
}

func (b *backgroundWithContent) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return b.content.InputHandler()
}

func (b *backgroundWithContent) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return b.content.MouseHandler()
}

// ShowHelpModal displays a help dialog with scrollable text.
func ShowHelpModal(app *tview.Application, helpText string) {
	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      70,
		FixedHeight:     25,
		BackgroundColor: tcell.ColorBlack,
		BorderColor:     tcell.NewRGBColor(102, 102, 102),
		TitleColor:      tcell.ColorLightGray,
		Title:           " Help ",
		TitleAlign:      tview.AlignCenter,
	}

	modal, content := NewConfiguredModal(app, config, true)
	modal.SetPreviousRoot(mainAppRoot)

	helpView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(helpText).
		SetScrollable(true).
		SetTextAlign(tview.AlignLeft)

	content.AddItem(helpView, 0, 1, true)

	modal.Show()
	app.SetFocus(helpView)
}

// ShowBlameModal displays detailed git blame information.
func ShowBlameModal(app *tview.Application, blameText string, commitHash string) {
	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      55,
		FixedHeight:     19,
		BackgroundColor: tcell.ColorBlack,
		BorderColor:     tcell.NewRGBColor(102, 102, 102),
		TitleColor:      tcell.ColorLightGray,
		Title:           " Git Blame ",
		TitleAlign:      tview.AlignCenter,
	}

	modal, content := NewConfiguredModal(app, config, true)
	modal.SetPreviousRoot(mainAppRoot)

	blameView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(blameText).
		SetScrollable(true).
		SetTextAlign(tview.AlignLeft)

	closeButton := tview.NewButton("Close")
	closeButton.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	closeButton.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))
	closeButton.SetSelectedFunc(func() {
		modal.Close()
	})

	copyButton := tview.NewButton("Copy Hash")
	copyButton.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	copyButton.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))
	copyButton.SetSelectedFunc(func() {
		copyToSystemClipboard(commitHash)
		modal.Close()
	})

	closeButton.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyRight {
			app.SetFocus(copyButton)
			return nil
		}
		return event
	})

	copyButton.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyLeft {
			app.SetFocus(closeButton)
			return nil
		}
		return event
	})

	buttonFlex := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(nil, 0, 1, false).
		AddItem(closeButton, 10, 0, true).
		AddItem(nil, 2, 0, false).
		AddItem(copyButton, 13, 0, false).
		AddItem(nil, 0, 1, false)

	content.AddItem(blameView, 0, 1, false)
	content.AddItem(buttonFlex, 3, 0, true)

	modal.Show()
	app.SetFocus(closeButton)
}

// ShowAlertModal displays a simple alert message.
func ShowAlertModal(app *tview.Application, title, message string) {
	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      40,
		FixedHeight:     7,
		BackgroundColor: tcell.ColorBlack,
		BorderColor:     tcell.NewRGBColor(102, 102, 102),
		TitleColor:      tcell.ColorLightGray,
		Title:           " " + title + " ",
		TitleAlign:      tview.AlignCenter,
	}

	modal, content := NewConfiguredModal(app, config, true)
	modal.SetPreviousRoot(mainAppRoot)

	messageView := tview.NewTextView().
		SetText(message).
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true)

	content.AddItem(messageView, 0, 1, false)

	modal.Show()
	app.SetFocus(messageView)
}
