package leantui

import (
	"context"
	"time"

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
		m.transcript.appendPending(blockReasoning, e.Content)
	case *runtime.AgentChoiceEvent:
		m.transcript.appendPending(blockAssistant, e.Content)
	case *runtime.PartialToolCallEvent:
		m.transcript.flushPending()
		toolDef := tools.Tool{Name: e.ToolCall.Function.Name}
		if e.ToolDefinition != nil {
			toolDef = *e.ToolDefinition
		}
		m.transcript.upsertTool(e.GetAgentName(), e.ToolCall, toolDef, tuitypes.ToolStatusPending)
	case *runtime.ToolCallEvent:
		m.transcript.flushPending()
		m.transcript.upsertTool(e.GetAgentName(), e.ToolCall, e.ToolDefinition, tuitypes.ToolStatusRunning)
	case *runtime.ToolCallOutputEvent:
		if tv := m.transcript.tool(e.ToolCallID); tv != nil && tv.message != nil {
			tv.message.AppendToolOutput(e.Output)
			if tv.message.ToolStatus == tuitypes.ToolStatusPending {
				tv.message.ToolStatus = tuitypes.ToolStatusRunning
				if tv.message.StartedAt == nil {
					now := time.Now()
					tv.message.StartedAt = &now
				}
			}
		}
	case *runtime.ToolCallResponseEvent:
		m.transcript.finishTool(e, m.sessionState)
	case *runtime.ToolCallConfirmationEvent:
		m.transcript.removeTool(toolViewID(e.ToolCall))
		toolDef := ensureToolDefinition(e.ToolCall, e.ToolDefinition)
		m.confirm = &confirmState{
			tool:     toolDef.Name,
			toolView: *newToolView(e.GetAgentName(), e.ToolCall, toolDef, tuitypes.ToolStatusConfirmation),
		}
	case *runtime.ElicitationRequestEvent:
		m.handleElicitationRequest(ctx, e)
	case *runtime.TokenUsageEvent:
		m.setTokenUsage(e.SessionID, e.Usage)
	case *runtime.AgentInfoEvent:
		m.status.agent = e.AgentName
		if m.sessionState != nil {
			m.sessionState.SetCurrentAgentName(e.AgentName)
		}
		if e.Model != "" {
			m.status.model = e.Model
		}
		if e.ContextLimit > 0 {
			m.status.contextLimit = e.ContextLimit
		}
	case *runtime.TeamInfoEvent:
		m.applyTeamInfo(ctx, e)
	case *runtime.SessionCompactionEvent:
		m.handleSessionCompaction(ctx, e)
	case *runtime.ErrorEvent:
		m.transcript.flushPending()
		m.addNotice("✗ ", e.Error, stError())
	case *runtime.WarningEvent:
		m.addNotice("⚠ ", e.Message, stWarning())
	case *runtime.ShellOutputEvent:
		output := e.Output
		m.transcript.addBlock(func(w int) []string { return renderToolOutput(output, w) })
	case *runtime.AgentSwitchingEvent:
		if e.Switching && e.ToAgent != "" {
			m.addNotice("→ ", "Switching to "+e.ToAgent, stMuted())
		}
	case *runtime.MaxIterationsReachedEvent:
		m.addNotice("⚠ ", "Maximum iterations reached.", stWarning())
	case *runtime.ModelFallbackEvent:
		m.addNotice("⚠ ", "Model "+e.FailedModel+" failed, falling back to "+e.FallbackModel+".", stWarning())
	}
}

// handleElicitationRequest surfaces an MCP elicitation (e.g. cagent's
// user_prompt tool, which the turf /up and /destroy flows use to seek plan
// approval). The lean TUI treats it as a yes/no confirmation: the message is
// written to the transcript so long plan text scrolls with history, and a
// compact prompt (see elicitState) collects the answer. URL/OAuth-mode
// elicitations don't arise in the up/destroy flows lean drives, so they are
// declined rather than left hanging.
func (m *model) handleElicitationRequest(ctx context.Context, e *runtime.ElicitationRequestEvent) {
	if e.Mode == "url" || isOAuthElicitation(e.Meta) {
		m.addNotice("⚠ ", "Skipping unsupported prompt in lean mode.", stWarning())
		_ = m.app.ResumeElicitation(ctx, tools.ElicitationActionDecline, nil)
		return
	}

	m.transcript.flushPending()
	if e.Message != "" {
		m.addNotice("", e.Message, stAccent())
	}
	m.elicit = &elicitState{title: elicitationTitle(e.Meta)}
}

// elicitationTitle reads the dialog title an elicitation may carry in its meta,
// matching cagent's user_prompt convention (key "cagent/title"), defaulting to
// "Question" as the full TUI does.
func elicitationTitle(meta map[string]any) string {
	if t, ok := meta["cagent/title"].(string); ok && t != "" {
		return t
	}
	return "Question"
}

func isOAuthElicitation(meta map[string]any) bool {
	t, ok := meta["docker-agent/type"].(string)
	return ok && t == "oauth_flow"
}

func (m *model) handleUserMessageEvent(e *runtime.UserMessageEvent) {
	if m.consumeIgnoredUserEcho(e.Message) {
		return
	}
	if pending, ok := m.consumePendingUser(pendingUserSteer, e.Message); ok {
		m.transcript.flushPending()
		m.addUserEcho(pending.display)
		return
	}
	m.transcript.flushPending()
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
	case "completed":
		m.finishBusy(ctx)
	}
}

// finishBusy clears the busy state at the end of a run and starts the next
// queued message, if any. It reports whether a queued run was started.
func (m *model) finishBusy(ctx context.Context) bool {
	m.transcript.flushPending()
	m.busy = false
	m.runCancel = nil

	if len(m.queue) > 0 {
		next := m.queue[0]
		m.queue[0] = pendingUserMessage{}
		m.queue = m.queue[1:]
		if pending, ok := m.consumePendingUser(pendingUserFollowUp, next.content); ok {
			next.display = pending.display
		}
		m.addUserEcho(next.display)
		m.ignoreUserEcho(next.content)
		m.startRun(ctx, next.content, nil)
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
		m.status.agent = a.Name
		switch {
		case a.Provider != "" && a.Model != "":
			m.status.model = a.Provider + "/" + a.Model
		case a.Model != "":
			m.status.model = a.Model
		}
		m.status.thinking = a.Thinking
	}
	m.refreshCommands(ctx)
}
