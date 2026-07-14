package log

// Ported from leap-cli (pkg/log/interactive_select.go) so the installer can offer
// the same interactive fuzzy picker experience as `leap push`'s model-path prompt.
// lipgloss was swapped for raw ANSI to avoid adding a dependency — this file
// already drives the terminal with raw escape sequences.

import (
	"bufio"
	"fmt"
	"os"
	"unicode/utf8"

	"golang.org/x/term"
)

// getTerminalHeight returns the terminal height, or a default if it can't be determined
func getTerminalHeight() int {
	_, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || height <= 0 {
		return 24 // Default fallback
	}
	return height
}

// ---------------------------
// Constants
// ---------------------------

const (
	maxVisibleItems = 10 // Maximum options to show at once (when terminal is large enough)
	minVisibleItems = 1  // Minimum options to show (even on tiny screens)
	fixedLinesCount = 4  // Header + input line + scroll indicators margin
	promptWidth     = 2  // Width of "❯ " prompt in characters
)

// Key codes
const (
	keyEscape    = 0x1b
	keyCtrlC     = 0x03
	keyEnter     = 0x0d
	keyLineFeed  = 0x0a
	keyTab       = 0x09
	keyBackspace = 0x7f
	keyDelete    = 0x08
)

// Arrow key codes (third byte of escape sequence)
const (
	arrowUp    = 'A'
	arrowDown  = 'B'
	arrowRight = 'C'
	arrowLeft  = 'D'
)

// UTF-8 byte markers
const (
	utf8MaxSingleByte  = 127
	utf8TwoByteStart   = 0xC0
	utf8ThreeByteStart = 0xE0
	utf8FourByteStart  = 0xF0
)

// ANSI escape sequences
const (
	ansiHideCursor    = "\x1b[?25l"
	ansiShowCursor    = "\x1b[?25h"
	ansiSaveCursor    = "\x1b[s"
	ansiRestoreCursor = "\x1b[u"
	ansiClearToEnd    = "\x1b[J"
	ansiClearLine     = "\x1b[2K"
	ansiMoveUp        = "\x1b[A"

	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiBlue  = "\x1b[96m" // bright cyan ~ #00BFFF
	ansiGray  = "\x1b[90m"
	ansiRed   = "\x1b[91m"
	ansiWhite = "\x1b[97m"
)

// ---------------------------
// Callback Types
// ---------------------------

type OnInputChange func(cursor int, filter string) (title string, options []string, newCursor int, inputErr string, hint string)
type OnEnter func(options []string, cursor int, filter string) (selectedOption *string, newFilter string, inputErr string, hint string)
type OnTabKey func(options []string, cursor int, filter string) (newFilter string)

// ---------------------------
// Styles
// ---------------------------

func styleTitle(s string) string      { return ansiBold + ansiBlue + "? " + s + ansiReset }
func styleError(s string) string      { return ansiBold + ansiRed + s + ansiReset }
func styleHint(s string) string       { return ansiBold + ansiGray + s + ansiReset }
func styleSelected(s string) string   { return ansiBold + ansiBlue + s + ansiReset }
func styleUnselected(s string) string { return ansiWhite + s + ansiReset }
func stylePrompt(s string) string     { return ansiBold + ansiBlue + s + ansiReset }

// ---------------------------
// Interactive Select State
// ---------------------------

type selectState struct {
	title        string
	options      []string
	cursor       int // Selected option index
	scrollOffset int
	filter       string
	textCursor   int // Cursor position within filter text (in bytes)
	inputErr     string
	hint         string
	maxVisible   int // Maximum visible items based on terminal size
}

// ---------------------------
// Terminal Output
// ---------------------------

var termOut = os.Stdout

func showCursor()    { _, _ = fmt.Fprint(termOut, ansiShowCursor) }
func saveCursor()    { _, _ = fmt.Fprint(termOut, ansiSaveCursor) }
func restoreCursor() { _, _ = fmt.Fprint(termOut, ansiRestoreCursor) }
func clearToEnd()    { _, _ = fmt.Fprint(termOut, ansiClearToEnd) }

func printLine(s string) {
	_, _ = fmt.Fprint(termOut, "\r"+ansiClearLine+s+"\r\n")
}

func moveUpLines(n int) {
	_, _ = fmt.Fprintf(termOut, "\x1b[%dA", n)
}

func moveToColumn(col int) {
	_, _ = fmt.Fprintf(termOut, "\x1b[%dG", col)
}

// ---------------------------
// Rendering
// ---------------------------

func (s *selectState) render() {
	restoreCursor()
	clearToEnd()

	s.renderHeader()
	s.renderInput()
	linesAfterInput := s.renderOptions()

	s.positionTextCursor(linesAfterInput)
}

func (s *selectState) renderHeader() {
	statusLine := "↑↓ move | TAB complete | ENTER select"
	var subtitle string

	if s.inputErr != "" {
		subtitle = styleError(s.inputErr)
	} else if s.hint != "" {
		subtitle = styleHint(statusLine + " | " + s.hint)
	} else {
		subtitle = styleHint(statusLine)
	}

	title := styleTitle(s.title + ":")
	printLine(fmt.Sprintf("%s %s", title, subtitle))
}

func (s *selectState) renderInput() {
	prompt := stylePrompt("❯ ")
	printLine(prompt + s.filter)
}

func (s *selectState) renderOptions() int {
	// Start at 1 because after printing input line, cursor is already 1 line below
	linesAfterInput := 1

	if len(s.options) == 0 {
		printLine(styleHint("  (no matches)"))
		return linesAfterInput + 1
	}

	visibleCount := min(len(s.options), s.maxVisible)
	endIdx := min(s.scrollOffset+visibleCount, len(s.options))

	// Scroll indicator: items above
	if s.scrollOffset > 0 {
		printLine(styleHint(fmt.Sprintf("  ↑ %d more above", s.scrollOffset)))
		linesAfterInput++
	}

	// Visible options
	for i := s.scrollOffset; i < endIdx; i++ {
		if i == s.cursor {
			printLine(styleSelected("> " + s.options[i]))
		} else {
			printLine(styleUnselected("  " + s.options[i]))
		}
		linesAfterInput++
	}

	// Scroll indicator: items below
	remaining := len(s.options) - endIdx
	if remaining > 0 {
		printLine(styleHint(fmt.Sprintf("  ↓ %d more below", remaining)))
		linesAfterInput++
	}

	return linesAfterInput
}

func (s *selectState) positionTextCursor(linesAfterInput int) {
	moveUpLines(linesAfterInput)
	runeCount := utf8.RuneCountInString(s.filter[:s.textCursor])
	col := promptWidth + runeCount + 1 // +1 for 1-based column indexing
	moveToColumn(col)
}

// ---------------------------
// Scroll Management
// ---------------------------

func (s *selectState) ensureCursorVisible() {
	if len(s.options) == 0 {
		s.scrollOffset = 0
		return
	}

	visibleCount := min(len(s.options), s.maxVisible)

	// Scroll up if cursor is above visible area
	if s.cursor < s.scrollOffset {
		s.scrollOffset = s.cursor
	}

	// Scroll down if cursor is below visible area
	if s.cursor >= s.scrollOffset+visibleCount {
		s.scrollOffset = s.cursor - visibleCount + 1
	}

	// Clamp scroll offset
	maxOffset := max(0, len(s.options)-visibleCount)
	s.scrollOffset = clamp(s.scrollOffset, 0, maxOffset)
}

// ---------------------------
// Text Cursor Movement
// ---------------------------

func (s *selectState) moveTextCursorLeft() {
	if s.textCursor > 0 {
		_, size := utf8.DecodeLastRuneInString(s.filter[:s.textCursor])
		s.textCursor -= size
	}
}

func (s *selectState) moveTextCursorRight() {
	if s.textCursor < len(s.filter) {
		_, size := utf8.DecodeRuneInString(s.filter[s.textCursor:])
		s.textCursor += size
	}
}

func (s *selectState) moveTextCursorToEnd() {
	s.textCursor = len(s.filter)
}

// ---------------------------
// Text Editing
// ---------------------------

func (s *selectState) deleteCharBefore() bool {
	if s.textCursor <= 0 {
		return false
	}

	_, size := utf8.DecodeLastRuneInString(s.filter[:s.textCursor])
	s.filter = s.filter[:s.textCursor-size] + s.filter[s.textCursor:]
	s.textCursor -= size
	return true
}

func (s *selectState) insertText(text string) {
	s.filter = s.filter[:s.textCursor] + text + s.filter[s.textCursor:]
	s.textCursor += len(text)
}

func (s *selectState) setFilter(newFilter string) {
	s.filter = newFilter
	s.moveTextCursorToEnd()
}

// ---------------------------
// Input Handling
// ---------------------------

func readUTF8Char(reader *bufio.Reader, firstByte byte) string {
	if firstByte < utf8MaxSingleByte {
		return string(firstByte)
	}

	if firstByte < utf8TwoByteStart {
		// Continuation byte without start - invalid
		return ""
	}

	// Determine continuation bytes count
	var numContinuation int
	switch {
	case firstByte >= utf8FourByteStart:
		numContinuation = 3
	case firstByte >= utf8ThreeByteStart:
		numContinuation = 2
	default:
		numContinuation = 1
	}

	bytes := []byte{firstByte}
	for i := 0; i < numContinuation; i++ {
		cb, err := reader.ReadByte()
		if err != nil {
			break
		}
		bytes = append(bytes, cb)
	}

	return string(bytes)
}

// ---------------------------
// State Updates
// ---------------------------

func (s *selectState) updateFromCallback(onInputChange OnInputChange) {
	title, options, _, inputErr, hint := onInputChange(s.cursor, s.filter)
	s.title = title
	s.options = options
	s.inputErr = inputErr
	s.hint = hint
	s.scrollOffset = 0
	// Always reset cursor to 0 when filter changes (options list changed)
	if len(options) > 0 {
		s.cursor = 0
	} else {
		s.cursor = -1
	}
	s.ensureCursorVisible()
}

// ---------------------------
// Helper Functions
// ---------------------------

func clamp(value, minVal, maxVal int) int {
	if value < minVal {
		return minVal
	}
	if value > maxVal {
		return maxVal
	}
	return value
}

// ---------------------------
// Public API
// ---------------------------

func InteractiveSelectTea(initialFilter string, onInputChange OnInputChange, onEnter OnEnter, onTabKey OnTabKey) (string, error) {
	state := initState(initialFilter, onInputChange)

	cleanup, err := setupTerminal()
	if err != nil {
		return "", err
	}
	defer cleanup()

	reserveSpace(state.maxVisible)
	saveCursor()
	state.ensureCursorVisible()
	state.render()

	return runInputLoop(state, onInputChange, onEnter, onTabKey)
}

func calculateMaxVisible() int {
	termHeight := getTerminalHeight()
	// Available lines = terminal height - fixed lines (header, input, margin)
	available := termHeight - fixedLinesCount
	// Clamp between min and max
	return clamp(available, minVisibleItems, maxVisibleItems)
}

func initState(initialFilter string, onInputChange OnInputChange) *selectState {
	title, options, cursor, inputErr, hint := onInputChange(0, initialFilter)

	// Ensure cursor is valid
	if cursor < 0 || cursor >= len(options) {
		if len(options) > 0 {
			cursor = 0
		} else {
			cursor = -1
		}
	}

	return &selectState{
		title:      title,
		options:    options,
		cursor:     cursor,
		filter:     initialFilter,
		textCursor: len(initialFilter),
		inputErr:   inputErr,
		hint:       hint,
		maxVisible: calculateMaxVisible(),
	}
}

func setupTerminal() (func(), error) {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return nil, fmt.Errorf("failed to set terminal raw mode: %w", err)
	}

	showCursor()

	return func() {
		_ = term.Restore(int(os.Stdin.Fd()), oldState)
	}, nil
}

func reserveSpace(maxVisible int) {
	termHeight := getTerminalHeight()
	// Calculate lines we'll actually use
	linesToReserve := maxVisible + 4 // header + input + options + scroll indicators

	// On small screens, don't reserve more than available
	maxReserve := termHeight - 1
	if linesToReserve > maxReserve {
		linesToReserve = maxReserve
	}
	if linesToReserve < 1 {
		return // Screen too small, skip reservation
	}

	// Reserve space by printing blank lines, then move back up
	for i := 0; i < linesToReserve; i++ {
		_, _ = fmt.Fprint(termOut, "\r\n")
	}
	for i := 0; i < linesToReserve; i++ {
		_, _ = fmt.Fprint(termOut, ansiMoveUp)
	}
}

func cleanupAndExit() {
	restoreCursor()
	clearToEnd()
}

func runInputLoop(state *selectState, onInputChange OnInputChange, onEnter OnEnter, onTabKey OnTabKey) (string, error) {
	reader := bufio.NewReader(os.Stdin)

	for {
		b, err := reader.ReadByte()
		if err != nil {
			return "", err
		}

		switch {
		case b == keyEscape:
			handleEscapeSequence(state, reader)

		case b == keyCtrlC:
			cleanupAndExit()
			return "", fmt.Errorf("cancelled")

		case b == keyEnter || b == keyLineFeed:
			if result, done := handleEnter(state, onInputChange, onEnter); done {
				return result, nil
			}

		case b == keyTab:
			handleTab(state, onInputChange, onTabKey)

		case b == keyBackspace || b == keyDelete:
			handleBackspace(state, onInputChange)

		case b >= 32: // Printable character
			handleCharInput(state, reader, onInputChange, b)

		default:
			continue
		}

		state.render()
	}
}

func handleEscapeSequence(state *selectState, reader *bufio.Reader) {
	b2, _ := reader.ReadByte()
	if b2 != '[' {
		return
	}

	b3, _ := reader.ReadByte()
	switch b3 {
	case arrowUp:
		if len(state.options) > 0 && state.cursor > 0 {
			state.cursor--
			state.ensureCursorVisible()
		}
	case arrowDown:
		if len(state.options) > 0 && state.cursor < len(state.options)-1 {
			state.cursor++
			state.ensureCursorVisible()
		}
	case arrowRight:
		state.moveTextCursorRight()
	case arrowLeft:
		state.moveTextCursorLeft()
	}
}

func handleEnter(state *selectState, onInputChange OnInputChange, onEnter OnEnter) (string, bool) {
	selIdx := -1
	if len(state.options) > 0 {
		selIdx = state.cursor
	}

	opt, newFilter, errMsg, hint := onEnter(state.options, selIdx, state.filter)

	if opt != nil {
		cleanupAndExit()
		return *opt, true
	}

	if newFilter != state.filter {
		state.setFilter(newFilter)
		state.updateFromCallback(onInputChange)
	}

	state.inputErr = errMsg
	state.hint = hint
	return "", false
}

func handleTab(state *selectState, onInputChange OnInputChange, onTabKey OnTabKey) {
	selIdx := -1
	if len(state.options) > 0 {
		selIdx = state.cursor
	}

	newFilter := onTabKey(state.options, selIdx, state.filter)
	if newFilter != state.filter {
		state.setFilter(newFilter)
		state.updateFromCallback(onInputChange)
	}
}

func handleBackspace(state *selectState, onInputChange OnInputChange) {
	if state.deleteCharBefore() {
		state.updateFromCallback(onInputChange)
	}
}

func handleCharInput(state *selectState, reader *bufio.Reader, onInputChange OnInputChange, firstByte byte) {
	char := readUTF8Char(reader, firstByte)
	if char == "" {
		return
	}

	state.insertText(char)
	state.updateFromCallback(onInputChange)
}
