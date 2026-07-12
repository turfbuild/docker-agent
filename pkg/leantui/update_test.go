package leantui

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/docker/docker-agent/pkg/app"
	"github.com/docker/docker-agent/pkg/effort"
	"github.com/docker/docker-agent/pkg/runtime"
	"github.com/docker/docker-agent/pkg/session"
	"github.com/docker/docker-agent/pkg/sessiontitle"
	"github.com/docker/docker-agent/pkg/tools"
	skillstool "github.com/docker/docker-agent/pkg/tools/builtin/skills"
	mcptools "github.com/docker/docker-agent/pkg/tools/mcp"
)

type cycleThinkingRuntime struct {
	supports   bool
	level      effort.Level
	err        error
	cycleCalls int
	setCalls   int
	setLevel   effort.Level
	steered    []runtime.QueuedMessage
	steerErr   error

	elicitCalls   int
	elicitAction  tools.ElicitationAction
	elicitContent map[string]any
}

func (r *cycleThinkingRuntime) CurrentAgentInfo(context.Context) runtime.CurrentAgentInfo {
	return runtime.CurrentAgentInfo{}
}
func (r *cycleThinkingRuntime) CurrentAgentName(context.Context) string       { return "coder" }
func (r *cycleThinkingRuntime) SetCurrentAgent(context.Context, string) error { return nil }
func (r *cycleThinkingRuntime) CurrentAgentTools(context.Context) ([]tools.Tool, error) {
	return nil, nil
}
func (r *cycleThinkingRuntime) CurrentAgentToolsetStatuses() []tools.ToolsetStatus { return nil }
func (r *cycleThinkingRuntime) RestartToolset(context.Context, string) error       { return nil }
func (r *cycleThinkingRuntime) EmitStartupInfo(context.Context, *session.Session, runtime.EventSink) {
}

func (r *cycleThinkingRuntime) EmitAgentInfo(_ context.Context, sink runtime.EventSink) {
	sink.Emit(runtime.TeamInfo([]runtime.AgentDetails{{Name: "coder", Thinking: r.level.String()}}, "coder"))
}
func (r *cycleThinkingRuntime) ResetStartupInfo() {}
func (r *cycleThinkingRuntime) RunStream(context.Context, *session.Session) <-chan runtime.Event {
	ch := make(chan runtime.Event)
	close(ch)
	return ch
}

func (r *cycleThinkingRuntime) Run(context.Context, *session.Session) ([]session.Message, error) {
	return nil, nil
}
func (r *cycleThinkingRuntime) Resume(context.Context, runtime.ResumeRequest) {}
func (r *cycleThinkingRuntime) ResumeElicitation(_ context.Context, action tools.ElicitationAction, content map[string]any) error {
	r.elicitCalls++
	r.elicitAction = action
	r.elicitContent = content
	return nil
}
func (r *cycleThinkingRuntime) SessionStore() session.Store { return nil }
func (r *cycleThinkingRuntime) Summarize(context.Context, *session.Session, string, runtime.EventSink) {
}
func (r *cycleThinkingRuntime) PermissionsInfo() *runtime.PermissionsInfo { return nil }
func (r *cycleThinkingRuntime) CurrentAgentSkillsToolset() *skillstool.ToolSet {
	return nil
}

func (r *cycleThinkingRuntime) RunSkillFork(context.Context, *session.Session, skillstool.RunSkillArgs, runtime.EventSink) (*tools.ToolCallResult, error) {
	return nil, nil
}

func (r *cycleThinkingRuntime) CurrentMCPPrompts(context.Context) map[string]mcptools.PromptInfo {
	return nil
}

func (r *cycleThinkingRuntime) ExecuteMCPPrompt(context.Context, string, map[string]string) (string, error) {
	return "", nil
}

func (r *cycleThinkingRuntime) UpdateSessionTitle(_ context.Context, sess *session.Session, title string) error {
	sess.Title = title
	return nil
}
func (r *cycleThinkingRuntime) TitleGenerator(context.Context) *sessiontitle.Generator { return nil }
func (r *cycleThinkingRuntime) Close() error                                           { return nil }
func (r *cycleThinkingRuntime) Stop()                                                  {}
func (r *cycleThinkingRuntime) Steer(_ context.Context, msg runtime.QueuedMessage) error {
	if r.steerErr != nil {
		return r.steerErr
	}
	r.steered = append(r.steered, msg)
	return nil
}
func (r *cycleThinkingRuntime) FollowUp(context.Context, runtime.QueuedMessage) error { return nil }
func (r *cycleThinkingRuntime) QueueStatus() runtime.QueueStatus                      { return runtime.QueueStatus{} }

func (r *cycleThinkingRuntime) TogglePause(context.Context) (bool, error) {
	return false, nil
}
func (r *cycleThinkingRuntime) SetAgentModel(context.Context, string, string) error { return nil }
func (r *cycleThinkingRuntime) CycleAgentThinkingLevel(context.Context, string) (effort.Level, error) {
	r.cycleCalls++
	if r.err != nil {
		return "", r.err
	}
	return r.level, nil
}

func (r *cycleThinkingRuntime) SetAgentThinkingLevel(_ context.Context, _ string, level effort.Level) (effort.Level, error) {
	r.setCalls++
	if r.err != nil {
		return "", r.err
	}
	r.setLevel = level
	return level, nil
}
func (r *cycleThinkingRuntime) AvailableModels(context.Context) []runtime.ModelChoice { return nil }
func (r *cycleThinkingRuntime) SupportsModelSwitching() bool                          { return r.supports }
func (r *cycleThinkingRuntime) OnToolsChanged(func(runtime.Event))                    {}
func (r *cycleThinkingRuntime) OnBackgroundEvent(func(runtime.Event))                 {}

var _ runtime.Runtime = (*cycleThinkingRuntime)(nil)

func TestShiftTabCyclesThinkingLevel(t *testing.T) {
	t.Parallel()
	rt := &cycleThinkingRuntime{supports: true, level: effort.High}
	m := bareModel(24)
	m.app = app.New(t.Context(), rt, session.New())

	m.handleKey(t.Context(), key{typ: keyShiftTab})

	assert.Equal(t, 1, rt.cycleCalls)
	assert.Equal(t, "high", m.status.thinking)
	assert.Empty(t, m.transcript.blocks)
}

func TestShiftTabReportsUnsupportedThinkingLevel(t *testing.T) {
	t.Parallel()
	rt := &cycleThinkingRuntime{supports: true, err: runtime.ErrUnsupported}
	m := bareModel(24)
	m.app = app.New(t.Context(), rt, session.New())

	m.handleKey(t.Context(), key{typ: keyShiftTab})

	assert.Equal(t, 1, rt.cycleCalls)
	assert.Empty(t, m.status.thinking)
	assert.Len(t, m.transcript.blocks, 1)
}

func TestEffortCommandSetsThinkingLevel(t *testing.T) {
	t.Parallel()
	rt := &cycleThinkingRuntime{supports: true}
	m := bareModel(24)
	m.app = app.New(t.Context(), rt, session.New())

	m.handleSetThinkingLevel(t.Context(), "high")

	assert.Equal(t, 1, rt.setCalls)
	assert.Equal(t, effort.High, rt.setLevel)
	assert.Equal(t, "high", m.status.thinking)
}

func TestEffortCommandRejectsUnknownLevel(t *testing.T) {
	t.Parallel()
	rt := &cycleThinkingRuntime{supports: true}
	m := bareModel(24)
	m.app = app.New(t.Context(), rt, session.New())

	m.handleSetThinkingLevel(t.Context(), "turbo")

	assert.Zero(t, rt.setCalls)
	assert.Empty(t, m.status.thinking)
	assert.Len(t, m.transcript.blocks, 1)
}

func TestElicitationPromptAcceptsWithY(t *testing.T) {
	t.Parallel()
	rt := &cycleThinkingRuntime{}
	m := bareModel(24)
	m.app = app.New(t.Context(), rt, session.New())

	ev := runtime.ElicitationRequest("Approve plan?", "form", nil, "", "eid", nil, "coder")
	m.handleEvent(t.Context(), ev)

	// The prompt is active and the message was written to the transcript.
	if assert.NotNil(t, m.elicit) {
		assert.Equal(t, "Question", m.elicit.title)
	}
	joined := strings.Join(m.transcript.lines(80, 0, false, m.sessionState, nil), "\n")
	assert.Contains(t, joined, "Approve plan?")

	m.handleKey(t.Context(), key{typ: keyRune, runes: []rune{'y'}})

	assert.Equal(t, 1, rt.elicitCalls)
	assert.Equal(t, tools.ElicitationActionAccept, rt.elicitAction)
	assert.Equal(t, map[string]any{"response": "yes"}, rt.elicitContent)
	assert.Nil(t, m.elicit)
}

func TestElicitationPromptDeclinesWithN(t *testing.T) {
	t.Parallel()
	rt := &cycleThinkingRuntime{}
	m := bareModel(24)
	m.app = app.New(t.Context(), rt, session.New())

	m.handleEvent(t.Context(), runtime.ElicitationRequest("Approve plan?", "form", nil, "", "eid", nil, "coder"))
	m.handleKey(t.Context(), key{typ: keyRune, runes: []rune{'n'}})

	assert.Equal(t, 1, rt.elicitCalls)
	assert.Equal(t, tools.ElicitationActionDecline, rt.elicitAction)
	assert.Nil(t, rt.elicitContent)
	assert.Nil(t, m.elicit)
}

func TestElicitationTitleFromMeta(t *testing.T) {
	t.Parallel()
	rt := &cycleThinkingRuntime{}
	m := bareModel(24)
	m.app = app.New(t.Context(), rt, session.New())

	meta := map[string]any{"cagent/title": "Approve deployment"}
	m.handleEvent(t.Context(), runtime.ElicitationRequest("msg", "form", nil, "", "eid", meta, "coder"))

	if assert.NotNil(t, m.elicit) {
		assert.Equal(t, "Approve deployment", m.elicit.title)
	}
}

func TestOAuthElicitationDeclinedInLeanMode(t *testing.T) {
	t.Parallel()
	rt := &cycleThinkingRuntime{}
	m := bareModel(24)
	m.app = app.New(t.Context(), rt, session.New())

	meta := map[string]any{"docker-agent/type": "oauth_flow"}
	m.handleEvent(t.Context(), runtime.ElicitationRequest("Authorize", "url", nil, "https://example.com", "eid", meta, "coder"))

	assert.Nil(t, m.elicit) // no interactive prompt; declined immediately
	assert.Equal(t, 1, rt.elicitCalls)
	assert.Equal(t, tools.ElicitationActionDecline, rt.elicitAction)
}

func TestEditorSubmitWhileBusySteersAndRendersAtStreamEnd(t *testing.T) {
	t.Parallel()
	rt := &cycleThinkingRuntime{}
	m := bareModel(24)
	m.app = app.New(t.Context(), rt, session.New())
	m.busy = true
	m.transcript.appendPending(blockAssistant, "assistant is still streaming")
	m.editor.setText("turn left")

	m.handleEnter(t.Context())

	if assert.Len(t, rt.steered, 1) {
		assert.Equal(t, "turn left", rt.steered[0].Content)
	}
	assert.Empty(t, m.queue)
	assert.Len(t, m.pendingUsers, 1)

	joined := strings.Join(m.transcript.lines(80, 0, true, m.sessionState, m.pendingUsers), "\n")
	assistantAt := strings.Index(joined, "assistant is still streaming")
	steerAt := strings.Index(joined, "turn left")
	assert.NotEqual(t, -1, assistantAt)
	assert.NotEqual(t, -1, steerAt)
	assert.Less(t, assistantAt, steerAt)
}

func TestSteeredUserEventConfirmsPendingAfterAssistant(t *testing.T) {
	t.Parallel()
	m := bareModel(24)
	m.busy = true
	m.transcript.appendPending(blockAssistant, "assistant response")
	m.addPendingUser("/change", "resolved steering prompt", pendingUserSteer)

	m.handleEvent(t.Context(), runtime.UserMessage("resolved steering prompt\n", "session", nil, 1))

	assert.Empty(t, m.pendingUsers)
	assert.Len(t, m.transcript.blocks, 2)
	joined := strings.Join(m.transcript.lines(80, 0, true, m.sessionState, nil), "\n")
	assistantAt := strings.Index(joined, "assistant response")
	steerAt := strings.Index(joined, "/change")
	assert.NotEqual(t, -1, assistantAt)
	assert.NotEqual(t, -1, steerAt)
	assert.Less(t, assistantAt, steerAt)
}
