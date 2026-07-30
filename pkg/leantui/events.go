package leantui

import (
	"context"
	"time"

	"github.com/docker/docker-agent/pkg/leantui/ui"
	"github.com/docker/docker-agent/pkg/runtime"
	"github.com/docker/docker-agent/pkg/tools"
	msgtypes "github.com/docker/docker-agent/pkg/tui/messages"
	tuitypes "github.com/docker/docker-agent/pkg/tui/types"
)

// handleEvent applies a single runtime event emitted by the App to the model,
// updating the conversation, tool state, status footer, or busy state.
func (m *model) handleEvent(ctx context.Context, ev any) {
	switch e := ev.(type) {
	case msgtypes.SendMsg:
		if e.BypassQueue {
			m.submit(ctx, e.Content, submitOptions{busyMode: busySubmitSteer})
		} else {
			m.submitFollowUp(ctx, e.Content)
		}
	case *runtime.StreamStartedEvent:
		m.busy = true
		m.trackStreamStarted(e.SessionID)
	case *runtime.UserMessageEvent:
		m.handleUserMessageEvent(e)
	case *runtime.StreamStoppedEvent:
		m.trackStreamStopped()
		m.handleStreamStopped(ctx)
	case *runtime.AgentChoiceReasoningEvent:
		m.screen.Transcript.AppendReasoning(e.Content)
	case *runtime.AgentChoiceEvent:
		m.screen.Transcript.AppendAssistant(e.Content)
	case *runtime.PartialToolCallEvent:
		m.screen.Transcript.FlushPending()
		toolDef := tools.Tool{Name: e.ToolCall.Function.Name}
		if e.ToolDefinition != nil {
			toolDef = *e.ToolDefinition
		}
		m.screen.Transcript.UpsertTool(e.GetAgentName(), e.ToolCall, toolDef, tuitypes.ToolStatusPending)
	case *runtime.ToolCallEvent:
		m.screen.Transcript.FlushPending()
		m.screen.Transcript.UpsertTool(e.GetAgentName(), e.ToolCall, e.ToolDefinition, tuitypes.ToolStatusRunning)
	case *runtime.ToolCallOutputEvent:
		if tv := m.screen.Transcript.Tool(e.ToolCallID); tv != nil && tv.Message() != nil {
			tv.Message().AppendToolOutput(e.Output)
			if tv.Message().ToolStatus == tuitypes.ToolStatusPending {
				tv.Message().ToolStatus = tuitypes.ToolStatusRunning
				if tv.Message().StartedAt == nil {
					now := time.Now()
					tv.Message().StartedAt = &now
				}
			}
		}
	case *runtime.ToolCallResponseEvent:
		var images []ui.InlineImage
		if m.renderImages {
			images = inlineImagesFromToolResult(e.Result)
		}
		m.screen.Transcript.FinishTool(e.ToolCallID, ui.ToolResult{Response: e.Response, Result: e.Result, AgentName: e.GetAgentName(), ToolDefinition: e.ToolDefinition, Images: images}, m.sessionState)
	case *runtime.ToolCallConfirmationEvent:
		m.screen.Transcript.RemoveTool(ui.ToolViewID(e.ToolCall))
		toolDef := ui.EnsureToolDefinition(e.ToolCall, e.ToolDefinition)
		m.screen.Confirm = &ui.ConfirmModel{
			Tool: toolDef.Name,
			View: *ui.NewToolView(e.GetAgentName(), e.ToolCall, toolDef, tuitypes.ToolStatusConfirmation),
		}
	case *runtime.ElicitationRequestEvent:
		m.handleElicitationRequest(ctx, e)
	case *runtime.TokenUsageEvent:
		m.setTokenUsage(e.SessionID, e.Usage)
	case *runtime.AgentInfoEvent:
		m.status.Agent = e.AgentName
		if m.sessionState != nil {
			m.sessionState.SetCurrentAgentName(e.AgentName)
		}
		if e.Model != "" {
			m.status.Model = e.Model
		}
		if e.ContextLimit > 0 {
			m.status.ContextLimit = e.ContextLimit
		}
	case *runtime.TeamInfoEvent:
		m.applyTeamInfo(ctx, e)
	case *runtime.SessionCompactionEvent:
		m.handleSessionCompaction(ctx, e)
	case *runtime.ErrorEvent:
		m.screen.Transcript.FlushPending()
		m.addNotice("✗ ", e.Error, ui.StError())
	case *runtime.WarningEvent:
		m.addNotice("⚠ ", e.Message, ui.StWarning())
	case *runtime.ShellOutputEvent:
		output := e.Output
		m.screen.Transcript.AddBlock(func(w int) []string { return ui.RenderToolOutput(output, w) })
	case *runtime.AgentSwitchingEvent:
		if e.Switching && e.ToAgent != "" {
			m.addNotice("→ ", "Switching to "+e.ToAgent, ui.StMuted())
		}
	case *runtime.MaxIterationsReachedEvent:
		m.addNotice("⚠ ", "Maximum iterations reached.", ui.StWarning())
	case *runtime.ModelFallbackEvent:
		m.addNotice("⚠ ", "Model "+e.FailedModel+" failed, falling back to "+e.FallbackModel+".", ui.StWarning())
	}
}

// handleElicitationRequest surfaces an MCP elicitation (e.g. cagent's built-in
// user_prompt tool, which agents call to ask for confirmation before a gated
// action). The lean TUI has no dialog surface, so it treats a form elicitation
// as a compact yes/no confirmation: the message is written to the transcript
// (so long prompt text scrolls with history) and a prompt collects the answer
// (see ui.ElicitModel). OAuth and URL-mode elicitations need a browser flow
// lean mode has no way to drive, so they are declined immediately rather than
// left hanging. The discrimination mirrors the full TUI's handleElicitationRequest.
func (m *model) handleElicitationRequest(ctx context.Context, e *runtime.ElicitationRequestEvent) {
	if isOAuthElicitation(e.Meta) || e.Mode == "url" {
		m.addNotice("⚠ ", "Skipping unsupported prompt in lean mode.", ui.StWarning())
		_ = m.app.ResumeElicitation(ctx, tools.ElicitationActionDecline, nil, e.ElicitationID)
		return
	}

	m.screen.Transcript.FlushPending()
	if e.Message != "" {
		m.addNotice("", e.Message, ui.StAccent())
	}
	m.screen.Elicit = &ui.ElicitModel{
		Title:         elicitationTitle(e.Meta),
		ElicitationID: e.ElicitationID,
	}
}

// isOAuthElicitation reports whether an elicitation carries the OAuth-flow meta
// marker, matching the full TUI's detection (Meta["docker-agent/type"] ==
// "oauth_flow"). Guards against a nil Meta map.
func isOAuthElicitation(meta map[string]any) bool {
	if meta == nil {
		return false
	}
	t, ok := meta["docker-agent/type"].(string)
	return ok && t == "oauth_flow"
}

// elicitationTitle reads the dialog title an elicitation may carry in its meta,
// matching cagent's user_prompt convention (key "cagent/title"), and defaults
// to "Question" as the full TUI does.
func elicitationTitle(meta map[string]any) string {
	if meta == nil {
		return "Question"
	}
	if t, ok := meta["cagent/title"].(string); ok && t != "" {
		return t
	}
	return "Question"
}

func (m *model) handleUserMessageEvent(e *runtime.UserMessageEvent) {
	if m.consumeIgnoredUserEcho(e.Message) {
		return
	}
	if pending, ok := m.consumePendingUser(ui.PendingUserSteer, e.Message); ok {
		m.screen.Transcript.FlushPending()
		m.addUserEcho(pending.Display)
		return
	}
	m.screen.Transcript.FlushPending()
	m.addUserEcho(e.Message)
}

func (m *model) handleStreamStopped(ctx context.Context) {
	if m.finishBusy(ctx) {
		return
	}

	if m.app != nil && m.app.ShouldExitAfterFirstResponse() {
		m.quit()
	}
}

func (m *model) handleSessionCompaction(ctx context.Context, e *runtime.SessionCompactionEvent) {
	switch e.Status {
	case "started":
		m.busy = true
		m.status.Compacting = true
	case "completed":
		m.status.Compacting = false
		m.finishBusy(ctx)
	}
}

// finishBusy clears the busy state at the end of a run and starts the next
// queued message, if any. It reports whether a queued run was started.
func (m *model) finishBusy(ctx context.Context) bool {
	m.screen.Transcript.FlushPending()
	m.busy = false
	m.runCancel = nil

	if len(m.queue) > 0 {
		next := m.queue[0]
		m.queue[0] = ui.PendingUserMessage{}
		m.queue = m.queue[1:]
		if pending, ok := m.consumePendingUser(ui.PendingUserFollowUp, next.Content); ok {
			next.Display = pending.Display
		}
		m.addUserEcho(next.Display)
		m.ignoreUserEcho(next.Content)
		m.startRun(ctx, next.Content, nil)
		return true
	}
	return false
}

func (m *model) applyTeamInfo(ctx context.Context, e *runtime.TeamInfoEvent) {
	if m.sessionState != nil {
		m.sessionState.SetAvailableAgents(e.AvailableAgents)
		m.sessionState.SetCurrentAgentName(e.CurrentAgent)
	}
	for _, a := range e.AvailableAgents {
		if a.Name != e.CurrentAgent {
			continue
		}
		m.status.Agent = a.Name
		switch {
		case a.Provider != "" && a.Model != "":
			m.status.Model = a.Provider + "/" + a.Model
		case a.Model != "":
			m.status.Model = a.Model
		}
		m.status.Thinking = a.Thinking
	}
	m.refreshCommands(ctx)
}
