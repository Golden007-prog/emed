package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Global to track the main application root and modal state
var (
	mainAppRoot     tview.Primitive
	modalIsOpen     bool
	previousFocus   tview.Primitive
)

// EmModal represents a modal dialog with a transparent full-screen background
type EmModal struct {
	*tview.Box
	app          *tview.Application
	container    *tview.Box       // Transparent full-screen container
	content      tview.Primitive  // The actual modal content
	cancellable  bool
	onCancel     func()
	onClose      func()
	previousRoot tview.Primitive
	savedFocus   tview.Primitive  // Store the focus that was active before modal
	isWrapped    bool            // If true, don't resize content in Draw (modalWrapper handles it)
}

// SetPreviousRoot sets the previous root to restore when modal closes
func (m *EmModal) SetPreviousRoot(root tview.Primitive) {
	m.previousRoot = root
}

// NewEmModal creates a new modal dialog
func NewEmModal(app *tview.Application, content tview.Primitive, cancellable bool) *EmModal {
	m := &EmModal{
		Box:         tview.NewBox(),
		app:         app,
		container:   tview.NewBox().SetBorder(false).SetBackgroundColor(tcell.ColorBlack),
		content:     content,
		cancellable: cancellable,
	}
	
	// We no longer set up handlers on the container since we'll handle them in InputHandler and MouseHandler
	
	return m
}

// SetOnCancel sets the cancel callback
func (m *EmModal) SetOnCancel(handler func()) *EmModal {
	m.onCancel = handler
	return m
}

// SetOnClose sets the close callback (called on any close, including cancel)
func (m *EmModal) SetOnClose(handler func()) *EmModal {
	m.onClose = handler
	return m
}

// Show displays the modal
func (m *EmModal) Show() {
	// Store the currently focused element before showing modal
	m.savedFocus = m.app.GetFocus()
	previousFocus = m.savedFocus  // Keep for compatibility
	
	// Mark that a modal is open
	modalIsOpen = true
	
	// Set the container as root (it will draw the content)
	m.app.SetRoot(m, true)
	m.app.SetFocus(m.content)
}

// Close closes the modal and restores the previous view
func (m *EmModal) Close() {
	if m.onClose != nil {
		m.onClose()
	}
	
	// Mark that modal is closed
	modalIsOpen = false
	
	// Restore the previous root
	if m.previousRoot != nil {
		m.app.SetRoot(m.previousRoot, true)
	} else if mainAppRoot != nil {
		// Fallback to the global main app root
		m.app.SetRoot(mainAppRoot, true)
	}
	
	// Restore the focus that was saved when modal was shown (not current focus)
	// Schedule focus restoration for next draw cycle to ensure UI is ready
	go func() {
		m.app.QueueUpdateDraw(func() {
			if m.savedFocus != nil {
				m.app.SetFocus(m.savedFocus)
			} else if previousFocus != nil {
				// Fallback for compatibility
				m.app.SetFocus(previousFocus)
			}
		})
	}()
}

// Cancel cancels the modal (only for cancellable modals)
func (m *EmModal) Cancel() {
	if m.cancellable {
		if m.onCancel != nil {
			m.onCancel()
		}
		m.Close()
	}
}

// Draw draws the modal
func (m *EmModal) Draw(screen tcell.Screen) {
	// First draw the main application content behind the modal
	if m.previousRoot != nil {
		m.previousRoot.Draw(screen)
	} else if mainAppRoot != nil {
		mainAppRoot.Draw(screen)
	}
	
	// If this modal is wrapped by modalWrapper, don't resize content
	// The wrapper will handle sizing
	if m.isWrapped {
		// Just draw the content as-is, the wrapper has already set its position
		m.content.Draw(screen)
		return
	}
	
	// For unwrapped modals, use default sizing
	screenWidth, screenHeight := screen.Size()
	
	// Get content's preferred size
	contentWidth, contentHeight := screenWidth/2, screenHeight/2 // Default 50% of screen
	
	// Allow customization of size through the content itself
	// Check if content has specific size requirements
	if _, ok := m.content.(*tview.Box); ok {
		// You can store custom width/height in the box if needed
		// For now, use reasonable defaults based on screen size
		contentWidth = screenWidth * 6 / 10  // 60% width
		contentHeight = screenHeight * 4 / 10 // 40% height
	}
	
	// Center the content
	x := (screenWidth - contentWidth) / 2
	y := (screenHeight - contentHeight) / 2
	
	// Set the content's position and size
	m.content.SetRect(x, y, contentWidth, contentHeight)
	
	// Draw the modal content on top
	m.content.Draw(screen)
}

// Focus passes focus to the modal content
func (m *EmModal) Focus(delegate func(p tview.Primitive)) {
	delegate(m.content)
}

// HasFocus returns whether the modal has focus
func (m *EmModal) HasFocus() bool {
	return m.content.HasFocus()
}

// GetRect returns the container's rect (full screen)
func (m *EmModal) GetRect() (int, int, int, int) {
	return m.container.GetRect()
}

// Forward all necessary methods to the wrapped modal
func (w *modalWrapper) SetPreviousRoot(root tview.Primitive) {
	w.modal.SetPreviousRoot(root)
}

func (w *modalWrapper) Show() {
	// Store the currently focused element before showing modal
	w.modal.savedFocus = w.modal.app.GetFocus()
	previousFocus = w.modal.savedFocus  // Keep for compatibility
	
	// Mark that a modal is open
	modalIsOpen = true
	
	// IMPORTANT: Set the root to the wrapper itself, not the inner modal
	// This ensures our custom Draw method is called
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


// SetRect sets the container's rect
func (m *EmModal) SetRect(x, y, width, height int) {
	m.container.SetRect(x, y, width, height)
}

// InputHandler returns the input handler
func (m *EmModal) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		// Handle ESC for cancellable modals FIRST
		if m.cancellable && event.Key() == tcell.KeyEscape {
			m.Cancel()
			return
		}
		
		// Pass through to the modal content FIRST to let it handle its own keys
		if handler := m.content.InputHandler(); handler != nil {
			handler(event, setFocus)
		}
		
		// Always consume the event to prevent it from propagating to the app
		// This blocks all keys from reaching the main app while modal is open
	}
}

// MouseHandler returns the mouse handler
func (m *EmModal) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		if action == tview.MouseLeftClick || action == tview.MouseScrollUp || action == tview.MouseScrollDown {
			// Get modal content position and size
			x, y, width, height := m.content.GetRect()
			mouseX, mouseY := event.Position()
			
			// Check if click is inside modal content
			if mouseX >= x && mouseX < x+width && mouseY >= y && mouseY < y+height {
				// Pass click to modal content if inside
				if handler := m.content.MouseHandler(); handler != nil {
					consumed, capture = handler(action, event, setFocus)
					// Make sure the content stays focused
					setFocus(m.content)
					return consumed, capture
				}
				return true, nil
			} else if action == tview.MouseLeftClick {
				// Click outside modal - close if cancellable
				if m.cancellable {
					m.Cancel()
				}
				return true, nil // Always consume the event
			}
		}
		// Consume all mouse events to prevent interaction with background
		return true, nil
	}
}

// EmModalConfig provides configuration for modal appearance
type EmModalConfig struct {
	Width           int    // Width as percentage of screen (0-100)
	Height          int    // Height as percentage of screen (0-100)
	BackgroundColor tcell.Color
	BorderColor     tcell.Color
	TitleColor      tcell.Color
	Title           string
	TitleAlign      int
	// For compact modals, use these fixed pixel dimensions instead
	UseFixedSize    bool
	FixedWidth      int
	FixedHeight     int
}

// NewConfiguredModal creates a modal with specific configuration
func NewConfiguredModal(app *tview.Application, config EmModalConfig, cancellable bool) (*modalWrapper, *tview.Flex) {
	// Create the content flex that will hold user content
	contentFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	
	var modalContent tview.Primitive
	
	if config.UseFixedSize {
		// For fixed size modals, create a simple bordered box
		bordered := tview.NewFlex().SetDirection(tview.FlexRow)
		bordered.SetBorder(true).
			SetTitle(config.Title).
			SetTitleAlign(config.TitleAlign).
			SetBackgroundColor(tcell.ColorBlack).
			SetBorderColor(config.BorderColor).
			SetTitleColor(config.TitleColor)
		
		// Add the content directly to the bordered flex
		bordered.AddItem(contentFlex, 0, 1, true)
		modalContent = bordered
	} else {
		// For percentage-based modals, use the backgroundWithContent for solid fill
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
	
	// Create the modal with the content
	modal := NewEmModal(app, modalContent, cancellable)
	modal.isWrapped = true  // Tell the modal it's wrapped, so it won't resize content
	
	// Create a wrapper that overrides Draw to use custom sizing
	wrapper := &modalWrapper{
		modal:  modal,
		config: config,
	}
	
	return wrapper, contentFlex
}

// modalWrapper wraps EmModal to provide custom drawing
type modalWrapper struct {
	modal  *EmModal  // Don't embed, use composition
	config EmModalConfig
}

// Draw draws the modal with custom sizing
func (w *modalWrapper) Draw(screen tcell.Screen) {
	// First draw the main application content behind the modal
	if w.modal.previousRoot != nil {
		w.modal.previousRoot.Draw(screen)
	} else if mainAppRoot != nil {
		mainAppRoot.Draw(screen)
	}
	
	// Now draw THIS modal with its configured dimensions
	w.drawWithOwnDimensions(screen)
}

// drawWithOwnDimensions draws this modal with its configured dimensions
func (w *modalWrapper) drawWithOwnDimensions(screen tcell.Screen) {
	screenWidth, screenHeight := screen.Size()
	
	// Safety check for screen dimensions
	if screenWidth <= 0 || screenHeight <= 0 {
		return
	}
	
	var contentWidth, contentHeight int
	
	if w.config.UseFixedSize {
		// Use explicit fixed dimensions
		contentWidth = w.config.FixedWidth
		contentHeight = w.config.FixedHeight
	} else {
		// Calculate width (always percentage)
		width := w.config.Width
		if width <= 0 || width > 100 {
			width = 60 // Default 60%
		}
		contentWidth = screenWidth * width / 100
		
		// Calculate height as percentage
		height := w.config.Height
		if height <= 0 || height > 100 {
			height = 40 // Default 40%
		}
		contentHeight = screenHeight * height / 100
	}
	
	// For fixed size modals, don't apply minimum size constraints
	if !w.config.UseFixedSize {
		// Make sure dimensions are reasonable for percentage-based modals
		if contentHeight < 5 {
			contentHeight = 5  // Minimum 5 lines
		}
		if contentHeight > screenHeight-4 {
			contentHeight = screenHeight - 4  // Leave some margin
		}
		if contentWidth < 20 {
			contentWidth = 20  // Minimum width
		}
		if contentWidth > screenWidth-4 {
			contentWidth = screenWidth - 4  // Leave some margin
		}
	}
	
	// Center the content
	x := (screenWidth - contentWidth) / 2
	y := (screenHeight - contentHeight) / 2
	
	// Ensure position is valid
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	
	// Set the content's position and size
	w.modal.content.SetRect(x, y, contentWidth, contentHeight)
	
	// If this is a fixed-size modal with a Flex, we need to fill the background
	if w.config.UseFixedSize {
		// Fill the background with black before drawing the content
		for row := y; row < y+contentHeight; row++ {
			for col := x; col < x+contentWidth; col++ {
				screen.SetContent(col, row, ' ', nil, tcell.StyleDefault.Background(tcell.ColorBlack))
			}
		}
	}
	
	// Draw the modal content on top
	w.modal.content.Draw(screen)
}

// SetMainAppRoot sets the main application root for modal restoration
func SetMainAppRoot(root tview.Primitive) {
	mainAppRoot = root
}

// GetMainAppRoot returns the main application root for modal restoration
func GetMainAppRoot() tview.Primitive {
	return mainAppRoot
}

// IsModalOpen returns true if a modal is currently open
func IsModalOpen() bool {
	return modalIsOpen
}

// solidBackground is a primitive that draws a solid background behind content
type solidBackground struct {
	*tview.Box
	content tview.Primitive
}

// Draw draws the background and content
func (s *solidBackground) Draw(screen tcell.Screen) {
	// Draw the box with border and background first
	s.Box.DrawForSubclass(screen, s)
	
	// Get inner rect
	x, y, width, height := s.GetInnerRect()
	
	// Set content rect and draw it on top
	s.content.SetRect(x, y, width, height)
	s.content.Draw(screen)
}

// Focus passes focus to the content
func (s *solidBackground) Focus(delegate func(p tview.Primitive)) {
	delegate(s.content)
}

// HasFocus returns whether the content has focus
func (s *solidBackground) HasFocus() bool {
	return s.content.HasFocus()
}

// InputHandler returns the content's input handler
func (s *solidBackground) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return s.content.InputHandler()
}

// MouseHandler returns the content's mouse handler  
func (s *solidBackground) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return s.content.MouseHandler()
}

// ShowConfirmModal shows a confirmation dialog
func ShowConfirmModal(app *tview.Application, title, message string, onConfirm, onCancel func()) {
	// Create modal configuration with fixed size
	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      50,  // 50 chars wide
		FixedHeight:     12,  // 12 lines tall for message + buttons
		BackgroundColor: tcell.ColorBlack,
		BorderColor:     tcell.ColorGray,
		TitleColor:      tcell.ColorLightGray,
		Title:           " " + title + " ",
		TitleAlign:      tview.AlignCenter,
	}
	
	modal, contentFlex := NewConfiguredModal(app, config, true)
	modal.SetPreviousRoot(mainAppRoot)
	
	// Create message text
	messageView := tview.NewTextView().
		SetText(message).
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true)
	
	// Create buttons
	buttonsFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	
	yesButton := tview.NewButton("YES")
	yesButton.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	yesButton.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))
	
	noButton := tview.NewButton("NO")
	noButton.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	noButton.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))
	
	// Create spacer boxes with black background
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
	
	// Add message and buttons to content flex
	contentFlex.
		AddItem(topSpacer, 0, 1, false).
		AddItem(messageView, 0, 2, false).
		AddItem(buttonsFlex, 3, 0, true).
		AddItem(bottomSpacer, 0, 1, false)
	
	// Set button handlers with access to modal
	yesButton.SetSelectedFunc(func() {
		modal.Close()
		if onConfirm != nil {
			onConfirm()
		}
	})
	
	noButton.SetSelectedFunc(func() {
		modal.Cancel()
	})
	
	// Set up button navigation
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

// backgroundWithContent combines a background box with content
type backgroundWithContent struct {
	*tview.Box
	background *tview.Box
	content    tview.Primitive
}

// Draw draws the background box fully, then the content on top
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

// GetRect returns the rect
func (b *backgroundWithContent) GetRect() (int, int, int, int) {
	return b.Box.GetRect()
}

// SetRect sets the rect
func (b *backgroundWithContent) SetRect(x, y, width, height int) {
	b.Box.SetRect(x, y, width, height)
	// Also set the background box rect to match
	if b.background != nil {
		b.background.SetRect(x, y, width, height)
	}
}

// Focus passes focus to the content
func (b *backgroundWithContent) Focus(delegate func(p tview.Primitive)) {
	delegate(b.content)
}

// HasFocus returns whether the content has focus
func (b *backgroundWithContent) HasFocus() bool {
	return b.content.HasFocus()
}

// InputHandler returns the content's input handler
func (b *backgroundWithContent) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return b.content.InputHandler()
}

// MouseHandler returns the content's mouse handler
func (b *backgroundWithContent) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return b.content.MouseHandler()
}


// ShowHelpModal shows a help dialog
func ShowHelpModal(app *tview.Application, helpText string) {
	// Create modal configuration
	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      70,  // 70 chars wide for comfortable reading
		FixedHeight:     25,  // 25 lines for help content
		BackgroundColor: tcell.ColorBlack,  // Use black background for consistency
		BorderColor:     tcell.ColorGray,
		TitleColor:      tcell.ColorLightGray,
		Title:           " Help ",
		TitleAlign:      tview.AlignCenter,
	}
	
	modal, content := NewConfiguredModal(app, config, true)
	modal.SetPreviousRoot(mainAppRoot)
	
	// Create scrollable text view for help
	helpView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(helpText).
		SetScrollable(true).
		SetTextAlign(tview.AlignLeft)
	
	// Add padding and help text
	content.
		AddItem(helpView, 0, 1, true)
	
	// Help can be closed with ESC or by clicking outside
	modal.Show()
	app.SetFocus(helpView)
}