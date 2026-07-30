package ui

import "github.com/docker/docker-agent/pkg/tui/service"

// Screen aggregates the lean TUI presentation models and lays out a full frame.
type Screen struct {
	Transcript   *Transcript
	Editor       *Editor
	Autocomplete *Autocomplete
	Status       StatusModel
	Confirm      *ConfirmModel
	Elicit       *ElicitModel
}

func NewScreen(workingDir, branch, editorPlaceholder string) *Screen {
	return &Screen{
		Transcript:   NewTranscript(),
		Editor:       NewEditor(editorPlaceholder),
		Autocomplete: NewAutocomplete(),
		Status:       StatusModel{WorkingDir: workingDir, Branch: branch},
	}
}

// Frame produces the full terminal frame and cursor position.
func (s *Screen) Frame(width, _, spinnerFrame int, busy bool, sessionState service.SessionStateReader, pendingUsers []PendingUserMessage) (lines []string, cursorLine, cursorCol int) {
	lines = s.Transcript.Lines(width, spinnerFrame, busy, sessionState, pendingUsers)

	lines = append(lines, s.Autocomplete.Render(width)...)

	inputStart := len(lines)
	switch {
	case s.Elicit != nil:
		elicitLines := s.Elicit.Render(width)
		lines = append(lines, elicitLines...)
		cursorLine = inputStart + max(len(elicitLines)-1, 0)
		if len(elicitLines) > 0 {
			cursorCol = min(DisplayWidth(elicitLines[len(elicitLines)-1]), max(width-1, 0))
		}
	case s.Confirm != nil:
		confirmLines := s.Confirm.Render(width)
		lines = append(lines, confirmLines...)
		cursorLine = inputStart + max(len(confirmLines)-1, 0)
		if len(confirmLines) > 0 {
			cursorCol = min(DisplayWidth(confirmLines[len(confirmLines)-1]), max(width-1, 0))
		}
	default:
		editorLines, row, col := s.Editor.Layout(width)
		lines = append(lines, editorLines...)
		cursorLine = inputStart + row
		cursorCol = col
	}

	lines = append(lines, "")
	lines = append(lines, RenderStatus(s.Status, width)...)

	return lines, cursorLine, cursorCol
}

// ConfirmModel holds a pending tool-approval prompt.
type ConfirmModel struct {
	Tool string
	View ToolView
}

func (c *ConfirmModel) Render(width int) []string {
	lines := []string{Truncate(StWarning().Render("● Approve tool call"), width)}
	lines = append(lines, RenderTool(c.View, width)...)
	lines = append(lines, Truncate(StMuted().Render("[y] yes   [a] always this tool   [b] auto-approve safe   [s] whole session   [n] no"), width))
	return lines
}

// ElicitModel holds a pending MCP elicitation prompt (e.g. cagent's user_prompt
// tool). The lean TUI renders it as a compact yes/no confirmation; the prompt
// message itself is written to the transcript when the request arrives.
type ElicitModel struct {
	// Title is the dialog title carried in the elicitation meta (defaults to
	// "Question", matching the full TUI).
	Title string
	// ElicitationID routes the ResumeElicitation response back to this request.
	ElicitationID string
}

func (e *ElicitModel) Render(width int) []string {
	title := e.Title
	if title == "" {
		title = "Question"
	}
	lines := []string{Truncate(StAccent().Render("● "+title), width)}
	lines = append(lines, Truncate(StMuted().Render("[y] yes   [n] no   [esc] cancel"), width))
	return lines
}
