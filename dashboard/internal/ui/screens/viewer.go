package screens

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/charmbracelet/x/ansi"

	"github.com/santifer/career-ops/dashboard/internal/theme"
)

// ViewerClosedMsg is emitted when the viewer is dismissed.
type ViewerClosedMsg struct{}

// ViewerModel implements an integrated file viewer screen.
type ViewerModel struct {
	lines         []string
	renderedLines []string
	title         string
	scrollOffset  int
	cursorLine    int    // current cursor position in visible area
	width         int
	height        int
	theme         theme.Theme
	// Search state
	searchMode  bool
	searchQuery string
	searchMatches []int // line indices that match
	currentMatch int   // index into searchMatches
	// Line numbers
	showLineNumbers bool
}

// NewViewerModel creates a new file viewer for the given path.
func NewViewerModel(t theme.Theme, path, title string, width, height int) ViewerModel {
	content, err := os.ReadFile(path)
	if err != nil {
		content = []byte("Error reading file: " + err.Error())
	}

	var lines []string
	if len(content) > 0 {
		lines = strings.Split(string(content), "\n")
	}

	m := ViewerModel{
		lines:           lines,
		title:           title,
		width:           width,
		height:          height,
		theme:           t,
		cursorLine:      0,
		showLineNumbers: true,
	}
	m.rebuildRender()
	return m
}

// rebuildRender recomputes renderedLines from raw lines using the current width.
func (m *ViewerModel) rebuildRender() {
	m.renderedLines = m.renderAll()
	m.clampScrollOffset()
	m.clampCursor()
}

func (m *ViewerModel) clampScrollOffset() {
	maxScroll := len(m.renderedLines) - m.bodyHeight()
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m *ViewerModel) clampCursor() {
	maxCursor := m.bodyHeight() - 1
	if maxCursor < 0 {
		maxCursor = 0
	}
	if m.cursorLine > maxCursor {
		m.cursorLine = maxCursor
	}
	if m.cursorLine < 0 {
		m.cursorLine = 0
	}
}

func (m ViewerModel) Init() tea.Cmd {
	return nil
}

func (m *ViewerModel) Resize(width, height int) {
	m.width = width
	m.height = height
	m.rebuildRender()
}

func (m ViewerModel) Update(msg tea.Msg) (ViewerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Search mode handling
		if m.searchMode {
			return m.handleSearchInput(msg)
		}

		switch msg.String() {
		case "q", "esc":
			return m, func() tea.Msg { return ViewerClosedMsg{} }

		// Cursor movement (vim-style)
		case "down", "j":
			m.moveCursorDown()

		case "up", "k":
			m.moveCursorUp()

		// Page movement
		case "pgdown", "ctrl+d":
			m.pageDown()

		case "pgup", "ctrl+u":
			m.pageUp()

		// Jump to boundaries
		case "home", "g", "gg":
			m.scrollOffset = 0
			m.cursorLine = 0

		case "end", "G":
			m.scrollToBottom()

		// Half-page movement
		case "ctrl+f":
			m.halfPageDown()

		case "ctrl+b":
			m.halfPageUp()

		// Center cursor
		case "ctrl+e":
			m.centerCursor()

		// Search
		case "/":
			m.searchMode = true
			m.searchQuery = ""

		case "n":
			m.nextSearchMatch()

		case "N":
			m.prevSearchMatch()

		// Toggle line numbers
		case "F2":
			m.showLineNumbers = !m.showLineNumbers

		// Jump to line (number prefix)
		case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// Could implement number prefix for g jumps
		}

	case tea.MouseMsg:
		// Mouse scroll support - scroll the view directly
		switch msg.Type {
		case tea.MouseWheelUp:
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
		case tea.MouseWheelDown:
			maxScroll := len(m.renderedLines) - m.bodyHeight()
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.scrollOffset < maxScroll {
				m.scrollOffset++
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.rebuildRender()
	}

	return m, nil
}

// handleSearchInput handles keyboard input during search mode.
func (m ViewerModel) handleSearchInput(msg tea.KeyMsg) (ViewerModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchMode = false
		m.searchQuery = ""
		m.searchMatches = nil
		return m, nil

	case "enter":
		m.searchMode = false
		m.findSearchMatches()
		if len(m.searchMatches) > 0 {
			m.currentMatch = 0
			m.jumpToMatch(0)
		}
		return m, nil

	case "backspace":
		if len(m.searchQuery) > 0 {
			runes := []rune(m.searchQuery)
			m.searchQuery = string(runes[:len(runes)-1])
		}
		return m, nil
	}

	// Append printable characters
	if r := msg.Runes; len(r) > 0 {
		m.searchQuery += string(r)
	}

	return m, nil
}

func (m *ViewerModel) moveCursorDown() {
	bodyH := m.bodyHeight()
	maxScroll := len(m.renderedLines) - bodyH
	if maxScroll < 0 {
		maxScroll = 0
	}

	if m.cursorLine < bodyH-1 {
		m.cursorLine++
	} else if m.scrollOffset < maxScroll {
		m.scrollOffset++
	}
}

func (m *ViewerModel) moveCursorUp() {
	if m.cursorLine > 0 {
		m.cursorLine--
	} else if m.scrollOffset > 0 {
		m.scrollOffset--
	}
}

func (m *ViewerModel) pageDown() {
	jump := m.bodyHeight() / 2
	maxScroll := len(m.renderedLines) - m.bodyHeight()
	if maxScroll < 0 {
		maxScroll = 0
	}
	m.scrollOffset += jump
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
}

func (m *ViewerModel) pageUp() {
	jump := m.bodyHeight() / 2
	m.scrollOffset -= jump
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m *ViewerModel) scrollToBottom() {
	maxScroll := len(m.renderedLines) - m.bodyHeight()
	if maxScroll < 0 {
		maxScroll = 0
	}
	m.scrollOffset = maxScroll
	m.cursorLine = m.bodyHeight() - 1
}

func (m *ViewerModel) halfPageDown() {
	jump := m.bodyHeight() / 4
	if jump < 1 {
		jump = 1
	}
	m.cursorLine += jump
	bodyH := m.bodyHeight()
	if m.cursorLine >= bodyH {
		m.scrollOffset += m.cursorLine - bodyH + 1
		m.cursorLine = bodyH - 1
	}
	maxScroll := len(m.renderedLines) - bodyH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
}

func (m *ViewerModel) halfPageUp() {
	jump := m.bodyHeight() / 4
	if jump < 1 {
		jump = 1
	}
	m.cursorLine -= jump
	if m.cursorLine < 0 {
		m.scrollOffset += m.cursorLine
		m.cursorLine = 0
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m *ViewerModel) centerCursor() {
	bodyH := m.bodyHeight()
	center := bodyH / 2
	m.cursorLine = center
}

func (m *ViewerModel) findSearchMatches() {
	m.searchMatches = nil
	if m.searchQuery == "" {
		return
	}
	query := strings.ToLower(m.searchQuery)
	for i, line := range m.renderedLines {
		if strings.Contains(strings.ToLower(line), query) {
			m.searchMatches = append(m.searchMatches, i)
		}
	}
}

func (m *ViewerModel) jumpToMatch(matchIdx int) {
	if matchIdx < 0 || matchIdx >= len(m.searchMatches) {
		return
	}
	targetLine := m.searchMatches[matchIdx]
	bodyH := m.bodyHeight()

	// Ensure the line is visible
	if targetLine < m.scrollOffset {
		m.scrollOffset = targetLine
		m.cursorLine = 0
	} else if targetLine >= m.scrollOffset+bodyH {
		m.scrollOffset = targetLine - bodyH + 1
		m.cursorLine = bodyH - 1
	} else {
		m.cursorLine = targetLine - m.scrollOffset
	}
}

func (m *ViewerModel) nextSearchMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	m.currentMatch = (m.currentMatch + 1) % len(m.searchMatches)
	m.jumpToMatch(m.currentMatch)
}

func (m *ViewerModel) prevSearchMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	m.currentMatch = (m.currentMatch - 1 + len(m.searchMatches)) % len(m.searchMatches)
	m.jumpToMatch(m.currentMatch)
}

func (m ViewerModel) bodyHeight() int {
	h := m.height - 5 // header + footer + search bar + padding
	if m.searchMode || m.searchQuery != "" {
		h--
	}
	if h < 3 {
		h = 3
	}
	return h
}

func (m ViewerModel) View() string {
	header := m.renderHeader()
	searchBar := m.renderSearchBar()
	body := m.renderBody()
	scrollBar := m.renderScrollBar()
	footer := m.renderFooter()

	// Combine body and scroll bar horizontally
	bodyWithScroll := lipgloss.JoinHorizontal(lipgloss.Top, body, scrollBar)

	return lipgloss.JoinVertical(lipgloss.Left, header, searchBar, bodyWithScroll, footer)
}

func (m ViewerModel) renderHeader() string {
	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.Text).
		Background(m.theme.Surface).
		Width(m.width).
		Padding(0, 2)

	title := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Blue).Render(m.title)

	// Scroll info
	right := lipgloss.NewStyle().Foreground(m.theme.Subtext)
	scrollInfo := m.getScrollInfo()
	scrollPercent := m.getScrollPercent()

	scroll := right.Render(fmt.Sprintf("%s (%s)", scrollInfo, scrollPercent))

	gap := m.width - lipgloss.Width(m.title) - lipgloss.Width(scroll) - 8
	if gap < 1 {
		gap = 1
	}

	return style.Render(title + strings.Repeat(" ", gap) + scroll)
}

func (m ViewerModel) renderSearchBar() string {
	if !m.searchMode && m.searchQuery == "" {
		return ""
	}

	style := lipgloss.NewStyle().
		Width(m.width).
		Padding(0, 1)

	promptStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Blue)
	queryStyle := lipgloss.NewStyle().Foreground(m.theme.Text)
	matchStyle := lipgloss.NewStyle().Foreground(m.theme.Subtext)

	prompt := "/"
	query := m.searchQuery
	if m.searchMode {
		query += "█" // cursor
	}

	matchInfo := ""
	if len(m.searchMatches) > 0 {
		matchInfo = fmt.Sprintf("  [%d/%d matches]", m.currentMatch+1, len(m.searchMatches))
	}

	return style.Render(promptStyle.Render(prompt) + queryStyle.Render(query) + matchStyle.Render(matchInfo))
}

func (m ViewerModel) renderBody() string {
	bh := m.bodyHeight()
	padStyle := lipgloss.NewStyle().Padding(0, 1)

	if len(m.renderedLines) == 0 {
		emptyStyle := lipgloss.NewStyle().Foreground(m.theme.Subtext)
		return padStyle.Render(emptyStyle.Render("(empty file)"))
	}

	end := m.scrollOffset + bh
	if end > len(m.renderedLines) {
		end = len(m.renderedLines)
	}
	visible := m.renderedLines[m.scrollOffset:end]

	// Build lines with line numbers and cursor
	var displayLines []string
	lineNumWidth := 4
	if len(m.renderedLines) > 999 {
		lineNumWidth = 5
	}

	for i, line := range visible {
		absLine := m.scrollOffset + i
		isCursor := i == m.cursorLine

		// Line number
		lineNum := fmt.Sprintf("%*d", lineNumWidth, absLine+1)
		lineNumStyle := lipgloss.NewStyle().Foreground(m.theme.Overlay)
		if isCursor {
			lineNumStyle = lineNumStyle.Foreground(m.theme.Yellow).Bold(true)
		}

		// Cursor indicator
		cursorIndicator := " "
		if isCursor {
			cursorIndicator = lipgloss.NewStyle().Foreground(m.theme.Blue).Bold(true).Render("▸")
		} else {
			cursorIndicator = " "
		}

		// Highlight search matches
		displayLine := line
		if m.searchQuery != "" && !m.searchMode {
			query := strings.ToLower(m.searchQuery)
			if strings.Contains(strings.ToLower(line), query) {
				// Highlight match (simple highlight - could be enhanced)
				displayLine = lipgloss.NewStyle().Background(lipgloss.Color("52")).Render(line)
			}
		}

		// Assemble line
		if m.showLineNumbers {
			displayLines = append(displayLines, cursorIndicator+lineNumStyle.Render(lineNum)+" │ "+displayLine)
		} else {
			displayLines = append(displayLines, cursorIndicator+" "+displayLine)
		}
	}

	// Pad to fill height
	for len(displayLines) < bh {
		displayLines = append(displayLines, "")
	}

	return padStyle.Render(strings.Join(displayLines, "\n"))
}

func (m ViewerModel) renderScrollBar() string {
	if len(m.renderedLines) <= m.bodyHeight() {
		return ""
	}

	scrollBarHeight := m.bodyHeight()
	totalLines := len(m.renderedLines)
	visibleLines := m.bodyHeight()

	// Calculate thumb position and size
	thumbPos := 0
	thumbSize := 1
	if totalLines > 0 {
		thumbSize = scrollBarHeight * visibleLines / totalLines
		if thumbSize < 1 {
			thumbSize = 1
		}
		maxPos := scrollBarHeight - thumbSize
		if maxPos > 0 {
			thumbPos = m.scrollOffset * maxPos / (totalLines - visibleLines)
		}
	}

	// Render scroll bar
	var scrollBar []string
	scrollBarStyle := lipgloss.NewStyle().Foreground(m.theme.Overlay)
	thumbStyle := lipgloss.NewStyle().Foreground(m.theme.Blue).Bold(true)

	for i := 0; i < scrollBarHeight; i++ {
		if i >= thumbPos && i < thumbPos+thumbSize {
			scrollBar = append(scrollBar, thumbStyle.Render("█"))
		} else {
			scrollBar = append(scrollBar, scrollBarStyle.Render("│"))
		}
	}

	return strings.Join(scrollBar, "\n")
}

func (m ViewerModel) getScrollInfo() string {
	if len(m.renderedLines) == 0 {
		return "0/0"
	}
	start := m.scrollOffset + 1
	end := m.scrollOffset + m.bodyHeight()
	if end > len(m.renderedLines) {
		end = len(m.renderedLines)
	}
	return fmt.Sprintf("%d-%d/%d", start, end, len(m.renderedLines))
}

func (m ViewerModel) getScrollPercent() string {
	if len(m.renderedLines) == 0 {
		return "0%"
	}
	maxScroll := len(m.renderedLines) - m.bodyHeight()
	if maxScroll <= 0 {
		return "100%"
	}
	pct := m.scrollOffset * 100 / maxScroll
	return fmt.Sprintf("%d%%", pct)
}

func (m ViewerModel) renderFooter() string {
	style := lipgloss.NewStyle().
		Foreground(m.theme.Subtext).
		Background(m.theme.Surface).
		Width(m.width).
		Padding(0, 1)

	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Text)
	descStyle := lipgloss.NewStyle().Foreground(m.theme.Subtext)

	return style.Render(
		keyStyle.Render("↑↓") + descStyle.Render(" move  ") +
			keyStyle.Render("PgUp/Dn") + descStyle.Render(" page  ") +
			keyStyle.Render("g/G") + descStyle.Render(" top/end  ") +
			keyStyle.Render("/") + descStyle.Render(" search  ") +
			keyStyle.Render("n/N") + descStyle.Render(" next/prev  ") +
			keyStyle.Render("F2") + descStyle.Render(" lines  ") +
			keyStyle.Render("Esc") + descStyle.Render(" back"))
}

// renderAll converts every raw markdown line into visual terminal lines.
func (m ViewerModel) renderAll() []string {
	var styled []string
	i := 0
	for i < len(m.lines) {
		line := m.lines[i]
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			styled = append(styled, "")
			i++
			continue
		}

		if isTableLine(line) {
			tableStart := i
			for i < len(m.lines) && isTableLine(m.lines[i]) {
				i++
			}
			styled = append(styled, m.renderTableBlock(m.lines[tableStart:i])...)
			continue
		}

		if strings.HasPrefix(trimmed, "```") {
			i++
			var codeLines []string
			for i < len(m.lines) {
				if strings.TrimSpace(m.lines[i]) == "```" {
					i++
					break
				}
				codeLines = append(codeLines, m.lines[i])
				i++
			}
			codeStyle := lipgloss.NewStyle().Background(m.theme.Surface).Foreground(m.theme.Text)
			w := m.width - 8
			if w < 10 {
				w = 10
			}
			for _, cl := range codeLines {
				for _, wl := range strings.Split(ansi.Wrap("  "+cl, w, ""), "\n") {
					styled = append(styled, codeStyle.Render(wl))
				}
			}
			continue
		}

		if isSpecialBlockLine(trimmed) {
			styled = append(styled, m.styleLine(line))
			i++
			continue
		}

		start := i
		for i < len(m.lines) {
			next := strings.TrimSpace(m.lines[i])
			if next == "" || isSpecialBlockLine(next) {
				break
			}
			i++
		}
		if i > start {
			paraLines := m.lines[start:i]
			para := strings.Join(paraLines, " ")
			w := m.width - 8
			if w < 10 {
				w = 10
			}
			wrapped := m.wrapParagraph(m.renderInlineElements(para), w)
			for _, wl := range wrapped {
				styled = append(styled, wl)
			}
		}
	}

	var flat []string
	for _, s := range styled {
		if strings.IndexByte(s, '\n') >= 0 {
			flat = append(flat, strings.Split(s, "\n")...)
		} else {
			flat = append(flat, s)
		}
	}
	return flat
}

func isTableLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return len(trimmed) > 1 && trimmed[0] == '|'
}

// isTableSeparator checks if a line is a table separator (|---|---|).
func isTableSeparator(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") {
		return false
	}
	cleaned := strings.NewReplacer("|", "", "-", "", ":", "", " ", "").Replace(trimmed)
	return cleaned == ""
}

// parseTableCells splits a table line into trimmed cells.
func parseTableCells(line string) []string {
	trimmed := strings.TrimSpace(line)
	// Remove leading and trailing pipes
	if len(trimmed) > 0 && trimmed[0] == '|' {
		trimmed = trimmed[1:]
	}
	if len(trimmed) > 0 && trimmed[len(trimmed)-1] == '|' {
		trimmed = trimmed[:len(trimmed)-1]
	}
	parts := strings.Split(trimmed, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}

func detectAlignment(sep string) lipgloss.Position {
	s := strings.TrimSpace(sep)
	if strings.HasPrefix(s, ":") && strings.HasSuffix(s, ":") {
		return lipgloss.Center
	}
	if strings.HasSuffix(s, ":") {
		return lipgloss.Right
	}
	return lipgloss.Left
}

func (m ViewerModel) renderTableBlock(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}

	var headers []string
	var dataRows [][]string
	var alignments []lipgloss.Position

	for _, line := range lines {
		if isTableSeparator(line) {
			if len(alignments) == 0 {
				for _, cell := range parseTableCells(line) {
					alignments = append(alignments, detectAlignment(cell))
				}
			}
			continue
		}
		cells := parseTableCells(line)
		rendered := make([]string, len(cells))
		for i, c := range cells {
			rendered[i] = m.renderInlineElements(c)
		}
		if headers == nil {
			headers = rendered
		} else {
			dataRows = append(dataRows, rendered)
		}
	}

	if len(headers) == 0 {
		var result []string
		for _, line := range lines {
			result = append(result, m.styleLine(line))
		}
		return result
	}

	w := m.width - 8
	if w < 10 {
		w = 10
	}

	borderStyle := lipgloss.NewStyle().Foreground(m.theme.Overlay)
	t := table.New().
		Width(w).
		Wrap(true).
		BorderStyle(borderStyle).
		BorderTop(true).BorderBottom(true).
		BorderLeft(true).BorderRight(true).
		BorderHeader(true).BorderColumn(true)

	t.Headers(headers...)
	if len(dataRows) > 0 {
		t.Rows(dataRows...)
	}

	t.StyleFunc(func(row, col int) lipgloss.Style {
		st := lipgloss.NewStyle().Padding(0, 1)
		if row == table.HeaderRow {
			return st.Bold(true).Foreground(m.theme.Sky)
		}
		if col < len(alignments) {
			st = st.Align(alignments[col])
		}
		return st.Foreground(m.theme.Text)
	})

	return strings.Split(t.String(), "\n")
}

var (
	reBold       = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reLink       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reBareURL    = regexp.MustCompile(`https?://\S*[^\s\)\]\.,;:!?]`)
	reInlineCode = regexp.MustCompile("`([^`]+)`")
	reListNumber = regexp.MustCompile(`^(\s*\d+\.\s+)(.*)$`)
)

func isHeadingLine(line string) bool {
	return strings.HasPrefix(line, "# ") ||
		strings.HasPrefix(line, "## ") ||
		strings.HasPrefix(line, "### ") ||
		strings.HasPrefix(line, "#### ") ||
		strings.HasPrefix(line, "##### ") ||
		strings.HasPrefix(line, "###### ")
}

func isSpecialBlockLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return isHeadingLine(trimmed) ||
		trimmed == "---" || trimmed == "***" ||
		strings.HasPrefix(trimmed, "> ") ||
		strings.HasPrefix(trimmed, "|") ||
		strings.HasPrefix(trimmed, "```") ||
		strings.HasPrefix(trimmed, "- ") ||
		strings.HasPrefix(trimmed, "* ") ||
		reListNumber.MatchString(trimmed) ||
		(strings.HasPrefix(trimmed, "**") && strings.Contains(trimmed, ":**"))
}

func (m ViewerModel) wrapParagraph(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	wrapped := ansi.Wrap(text, width, "")
	return strings.Split(wrapped, "\n")
}

func (m ViewerModel) renderInlineElements(line string) string {
	return m.renderInlineElementsAs(line, m.theme.Subtext)
}

// renderInlineElementsAs walks the raw line once and reapplies baseColor around
// every plain-text span, so resets emitted by inline tokens (code, bold, link,
// bare URL) don't leak through to subsequent text.
func (m ViewerModel) renderInlineElementsAs(line string, baseColor lipgloss.Color) string {
	baseStyle := lipgloss.NewStyle().Foreground(baseColor)
	codeStyle := lipgloss.NewStyle().Background(m.theme.Surface).Foreground(m.theme.Text)
	boldStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Yellow)
	linkStyle := lipgloss.NewStyle().Foreground(m.theme.Blue)

	var b strings.Builder
	rest := line
	for rest != "" {
		match := findInlineMatch(rest, codeStyle, boldStyle, linkStyle)
		if match == nil {
			b.WriteString(baseStyle.Render(rest))
			break
		}
		if match.start > 0 {
			b.WriteString(baseStyle.Render(rest[:match.start]))
		}
		b.WriteString(match.rendered)
		rest = rest[match.end:]
	}
	return b.String()
}

type inlineMatch struct {
	start, end int
	rendered   string
}

func findInlineMatch(s string, codeStyle, boldStyle, linkStyle lipgloss.Style) *inlineMatch {
	var best *inlineMatch
	consider := func(loc []int, rendered func() string) {
		if loc == nil || (best != nil && loc[0] >= best.start) {
			return
		}
		best = &inlineMatch{start: loc[0], end: loc[1], rendered: rendered()}
	}

	if loc := reInlineCode.FindStringIndex(s); loc != nil {
		consider(loc, func() string { return codeStyle.Render(s[loc[0]+1 : loc[1]-1]) })
	}
	if loc := reBold.FindStringIndex(s); loc != nil {
		consider(loc, func() string { return boldStyle.Render(s[loc[0]+2 : loc[1]-2]) })
	}
	if loc := reLink.FindStringIndex(s); loc != nil {
		consider(loc, func() string {
			sm := reLink.FindStringSubmatch(s[loc[0]:loc[1]])
			if len(sm) >= 2 {
				return linkStyle.Render(sm[1])
			}
			return s[loc[0]:loc[1]]
		})
	}
	if loc := reBareURL.FindStringIndex(s); loc != nil {
		consider(loc, func() string { return linkStyle.Render(s[loc[0]:loc[1]]) })
	}
	return best
}

func (m ViewerModel) styleLine(line string) string {
	trimmed := strings.TrimSpace(line)
	w := m.width - 8
	if w < 10 {
		w = 10
	}

	if strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "## ") {
		content := strings.TrimPrefix(trimmed, "# ")
		return lipgloss.NewStyle().Bold(true).Foreground(m.theme.Blue).Width(w).Render("  " + content)
	}
	if strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "### ") {
		content := strings.TrimPrefix(trimmed, "## ")
		return lipgloss.NewStyle().Bold(true).Foreground(m.theme.Mauve).Width(w).Render("  " + content)
	}
	if strings.HasPrefix(trimmed, "### ") && !strings.HasPrefix(trimmed, "#### ") {
		content := strings.TrimPrefix(trimmed, "### ")
		return lipgloss.NewStyle().Bold(true).Foreground(m.theme.Sky).Width(w).Render("  " + content)
	}
	if strings.HasPrefix(trimmed, "#### ") && !strings.HasPrefix(trimmed, "##### ") {
		content := strings.TrimPrefix(trimmed, "#### ")
		return lipgloss.NewStyle().Bold(true).Foreground(m.theme.Subtext).Width(w).Render("    " + content)
	}
	if strings.HasPrefix(trimmed, "##### ") && !strings.HasPrefix(trimmed, "###### ") {
		content := strings.TrimPrefix(trimmed, "##### ")
		return lipgloss.NewStyle().Bold(true).Foreground(m.theme.Overlay).Width(w).Render("      " + content)
	}
	if strings.HasPrefix(trimmed, "###### ") {
		content := strings.TrimPrefix(trimmed, "###### ")
		return lipgloss.NewStyle().Bold(true).Foreground(m.theme.Overlay).Width(w).Render("        " + content)
	}
	if trimmed == "---" || trimmed == "***" {
		return lipgloss.NewStyle().Foreground(m.theme.Overlay).Width(w).Render(strings.Repeat("─", w))
	}
	if strings.HasPrefix(trimmed, "> ") {
		content := strings.TrimPrefix(trimmed, "> ")
		border := lipgloss.NewStyle().Foreground(m.theme.Overlay).Render("▎ ")
		textStyle := lipgloss.NewStyle().Foreground(m.theme.Subtext).Italic(true)
		wrapped := strings.Split(ansi.Wrap(textStyle.Render(content), w-2, ""), "\n")
		result := make([]string, 0, len(wrapped))
		for i, line := range wrapped {
			if i == 0 {
				result = append(result, border+line)
			} else {
				result = append(result, strings.Repeat(" ", ansi.StringWidth(border))+line)
			}
		}
		return strings.Join(result, "\n")
	}
	if strings.HasPrefix(trimmed, "**") && strings.Contains(trimmed, ":**") {
		styled := m.renderInlineElements(line)
		return ansi.Wrap(styled, w, "")
	}
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
		content := trimmed[2:]
		marker := lipgloss.NewStyle().Foreground(m.theme.Blue).Render("• ")
		return m.renderListItem(marker, content, w)
	}
	if reListNumber.MatchString(trimmed) {
		sm := reListNumber.FindStringSubmatch(trimmed)
		if len(sm) >= 3 {
			marker := lipgloss.NewStyle().Foreground(m.theme.Blue).Render(sm[1])
			return m.renderListItem(marker, sm[2], w)
		}
	}

	styled := m.renderInlineElementsAs(trimmed, m.theme.Subtext)
	return ansi.Wrap(styled, w, "")
}

func (m ViewerModel) renderListItem(marker, content string, width int) string {
	markerWidth := ansi.StringWidth(marker)
	textWidth := width - markerWidth
	if textWidth < 10 {
		textWidth = 10
	}
	styled := m.renderInlineElementsAs(content, m.theme.Text)
	lines := strings.Split(ansi.Wrap(styled, textWidth, ""), "\n")
	result := make([]string, 0, len(lines))
	for i, line := range lines {
		if i == 0 {
			result = append(result, marker+line)
		} else {
			result = append(result, strings.Repeat(" ", markerWidth)+line)
		}
	}
	return strings.Join(result, "\n")
}
