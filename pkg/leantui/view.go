package leantui

// buildLines produces the entire frame: the conversation, the slash-command
// popup, the input box (or a confirmation prompt) and the status footer. It
// returns the lines plus the hardware cursor position (a line index and column).
func (m *model) buildLines() (lines []string, cursorLine, cursorCol int) {
	width := m.width

	lines = m.transcript.lines(width, m.spinnerFrame, m.busy, m.sessionState, m.pendingUsers)

	lines = append(lines, m.ac.render(width)...)

	inputStart := len(lines)
	switch {
	case m.elicit != nil:
		promptLines := m.elicit.render(width)
		lines = append(lines, promptLines...)
		cursorLine = inputStart + max(len(promptLines)-1, 0)
		if len(promptLines) > 0 {
			cursorCol = min(displayWidth(promptLines[len(promptLines)-1]), max(width-1, 0))
		}
	case m.confirm != nil:
		confirmLines := m.confirm.render(width)
		lines = append(lines, confirmLines...)
		cursorLine = inputStart + max(len(confirmLines)-1, 0)
		if len(confirmLines) > 0 {
			cursorCol = min(displayWidth(confirmLines[len(confirmLines)-1]), max(width-1, 0))
		}
	default:
		editorLines, row, col := m.editor.layout(width)
		lines = append(lines, editorLines...)
		cursorLine = inputStart + row
		cursorCol = col
	}

	lines = append(lines, "")
	lines = append(lines, renderStatus(m.status, width)...)

	return lines, cursorLine, cursorCol
}

// confirmState holds a pending tool-approval prompt.
type confirmState struct {
	tool     string // raw tool name, used to scope "always allow"
	toolView toolView
}

func (c *confirmState) render(width int) []string {
	lines := []string{truncate(stWarning().Render("● Approve tool call"), width)}
	lines = append(lines, renderTool(c.toolView, width)...)
	lines = append(lines, truncate(stMuted().Render("[y] yes   [a] always this tool   [s] whole session   [n] no"), width))
	return lines
}

// elicitState holds a pending MCP elicitation prompt (e.g. cagent's user_prompt
// tool). The lean TUI handles it as a yes/no confirmation: the elicitation
// message is written to the transcript when the request arrives, so this prompt
// area stays compact — just the title and the answer keys.
type elicitState struct {
	title string // dialog title, from the request meta; defaults to "Question"
}

func (e *elicitState) render(width int) []string {
	title := e.title
	if title == "" {
		title = "Question"
	}
	return []string{
		truncate(stWarning().Render("● "+title), width),
		truncate(stMuted().Render("[y] yes   [n] no   [esc] cancel"), width),
	}
}
