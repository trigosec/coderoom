package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/trigosec/coderoom/internal/participant"
	"github.com/trigosec/coderoom/internal/session"
	"github.com/trigosec/coderoom/internal/ui/editor"
)

const (
	// marginH is the number of columns reserved on each horizontal side. Only a
	// left prefix is applied in View(); the right margin is implicit because
	// viewport, separator, and input are all sized to inner = width-2*marginH.
	marginH = 2
	// marginV is the number of empty rows below the input.
	marginV = 1
)

func chromeHeight(inputHeight, toolboxH int) int {
	// viewport separator + input + toolbox + bottom margin
	return 1 + inputHeight + toolboxH + marginV
}

// Init starts the session event listener; called once by Bubble Tea on startup.
func (m Model) Init() tea.Cmd {
	return tea.Batch(awaitEvent(m.queue), m.compose.Init())
}

// Update handles incoming messages and returns the next model state.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		return m.handleResize(msg), nil
	case editor.Response:
		return m.handleEditorResult(msg), nil
	case sessionEventMsg:
		next, cmd := m.handleEvent(session.Event(msg))
		return next, tea.Batch(cmd, awaitEvent(m.queue))
	default:
		var composeCmd tea.Cmd
		m.compose, composeCmd = m.compose.Update(msg)
		m = m.syncAfterCompose()
		var toolboxCmd tea.Cmd
		m.toolbox, toolboxCmd = m.toolbox.Update(msg)
		return m, tea.Batch(composeCmd, toolboxCmd)
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlO {
		return m.toggleFocus()
	}

	// PgUp/PgDn always scroll the viewport, regardless of focus.
	if msg.Type == tea.KeyPgUp {
		m.history = m.history.HalfPageUp()
		return m, nil
	}
	if msg.Type == tea.KeyPgDown {
		m.history = m.history.HalfPageDown()
		return m, nil
	}

	if m.focus == focusViewport {
		return m.handleViewportKey(msg)
	}
	return m.handleComposerKey(msg)
}

func (m Model) toggleFocus() (Model, tea.Cmd) {
	if m.focus == focusComposer {
		m.focus = focusViewport
		m.compose = m.compose.Blur()
		return m, nil
	}
	m.focus = focusComposer
	var cmd tea.Cmd
	m.compose, cmd = m.compose.Focus()
	return m, cmd
}

func (m Model) handleComposerKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlG {
		return m.startEditorCompose()
	}
	if msg.Type == tea.KeyEnter && !msg.Alt {
		return m.handleSubmit()
	}
	var cmd tea.Cmd
	m.compose, cmd = m.compose.Update(msg)
	m = m.syncAfterCompose()
	return m, cmd
}

func (m Model) handleViewportKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, nil
	case tea.KeyCtrlG:
		return m.openEditorWithTranscript()
	case tea.KeyUp:
		m.history = m.history.ScrollUp(1)
		return m, nil
	case tea.KeyDown:
		m.history = m.history.ScrollDown(1)
		return m, nil
	case tea.KeyHome:
		m.history = m.history.GotoTop()
		return m, nil
	case tea.KeyEnd:
		m.history = m.history.GotoBottom()
		return m, nil
	case tea.KeyEsc:
		m.focus = focusComposer
		var cmd tea.Cmd
		m.compose, cmd = m.compose.Focus()
		return m, cmd
	default:
		var cmd tea.Cmd
		m.history, cmd = m.history.Update(msg)
		return m, cmd
	}
}

func (m Model) handleResize(msg tea.WindowSizeMsg) Model {
	m.lastSize = msg
	inner := max(msg.Width-2*marginH, 1)
	m.toolbox = m.toolbox.SetWidth(inner)
	m.compose = m.compose.SetWidth(inner).SetMaxHeightFromTotal(msg.Height)
	h := max(msg.Height-chromeHeight(m.compose.Height(), m.toolbox.Height()), 1)
	m.history = m.history.SetSize(inner, h)
	m.history = m.history.RebuildColors()
	return m
}

func (m Model) handleSubmit() (Model, tea.Cmd) {
	raw := m.compose.Value()
	m.compose = m.compose.Reset()
	m = m.syncAfterCompose()
	if strings.TrimSpace(raw) == "" {
		return m, nil
	}
	action, err := Parse(raw)
	var routing []string
	if err == nil {
		routing = routingFor(action, m.sess.RoutableParticipants())
	}
	m.history = m.history.AppendUserInputRecord(raw, routing)
	if err != nil {
		m.history = m.history.AppendSystemRecord("error: " + err.Error())
		return m, nil
	}
	return m.executeAction(action)
}

// routingFor returns the aliases that will receive the action, used to
// populate the routing footer on the user input record. Aliases are sorted
// for a stable display order.
func routingFor(a Action, ps []participant.Participant) []string {
	if _, ok := a.(Broadcast); ok {
		aliases := make([]string, len(ps))
		for i, p := range ps {
			aliases[i] = p.Alias
		}
		slices.Sort(aliases)
		return aliases
	}
	if s, ok := a.(Send); ok {
		return []string{s.Alias}
	}
	return nil
}

func (m Model) handleEvent(e session.Event) (Model, tea.Cmd) {
	var next Model
	if out, ok := m.handleAgentLifecycleEvent(e); ok {
		next = out
	} else {
		next = m.handleMessageEvent(e)
	}
	var cmd tea.Cmd
	next.toolbox, cmd = next.toolbox.SetParticipants(next.sess.Roster())
	return next, cmd
}

func (m Model) handleAgentLifecycleEvent(e session.Event) (Model, bool) {
	switch e.Kind {
	case session.KindAgentStarting:
		m.history = m.history.AppendSystemRecord("[" + e.Alias + " starting]")
		return m, true
	case session.KindAgentStarted:
		m.history = m.history.AppendSystemRecord("[" + e.Alias + " joined]")
		return m, true
	case session.KindAgentStopped:
		m.history = m.history.ClearStreaming(e.Alias)
		m.history = m.history.MarkDeparted(e.Alias)
		m.history = m.history.AppendSystemRecord("[" + e.Alias + " left]")
		return m, true
	case session.KindAgentCrashed:
		m.history = m.history.ClearStreaming(e.Alias)
		m.history = m.history.MarkDeparted(e.Alias)
		m.history = m.history.AppendSystemRecord("[" + e.Alias + " crashed]")
		return m, true
	default:
		return m, false
	}
}

func (m Model) handleMessageEvent(e session.Event) Model {
	switch e.Kind {
	case session.KindBroadcast:
		m.history = m.history.AppendSystemRecord("[all] " + e.Text)
	case session.KindSharedSend:
		m.history = m.history.AppendSystemRecord("[→ " + e.Alias + "] " + e.Text)
	case session.KindSharedNotice:
		m.history = m.history.AppendSystemRecord("[notice → " + e.Alias + "]")
	case session.KindAgentLog:
		m.history = m.history.AppendLogRecord(e.Alias, e.Text)
	case session.KindDelta:
		m.history = m.history.HandleDelta(e.Alias, e.Text)
	case session.KindDone:
		m.history = m.history.ClearStreaming(e.Alias)
	default:
		// Lifecycle events are handled by handleAgentLifecycleEvent.
	}
	return m
}

func (m Model) executeAction(a Action) (Model, tea.Cmd) {
	if out, ok := m.executeAgentAction(a); ok {
		return out, nil
	}
	if out, ok := m.executeDebugAction(a); ok {
		return out, nil
	}
	return m.executeUIAction(a)
}

func (m Model) executeAgentAction(a Action) (Model, bool) {
	switch act := a.(type) {
	case Invite:
		return m.inviteAgent(act.Alias), true
	case Remove:
		return m.removeAgent(act.Alias), true
	case Cancel:
		return m.cancelAgent(act.Alias), true
	case Send:
		return m.sendToAgent(act.Alias, act.Text), true
	case Broadcast:
		return m.broadcastAll(act.Text), true
	default:
		return m, false
	}
}

func (m Model) executeDebugAction(a Action) (Model, bool) {
	switch a.(type) {
	case DebugView:
		if !m.debug {
			m.history = m.history.AppendSystemRecord("error: debug commands disabled (set CODEROOM_DEBUG=1)")
			return m, true
		}
		return m.debugView(), true
	case DebugRows:
		if !m.debug {
			m.history = m.history.AppendSystemRecord("error: debug commands disabled (set CODEROOM_DEBUG=1)")
			return m, true
		}
		m.history = m.history.ToggleDebugRowNums()
		return m, true
	default:
		return m, false
	}
}

func (m Model) executeUIAction(a Action) (Model, tea.Cmd) {
	switch a.(type) {
	case Who:
		return m.showWho(), nil
	case Help:
		return m.showHelp(), nil
	case Quit:
		m.sess.Shutdown()
		return m, tea.Quit
	default:
		return m, nil
	}
}

func (m Model) inviteAgent(alias string) Model {
	color, nextPalette := m.palette.Next()
	err := m.sess.Execute(session.InviteCommand{
		Alias:      alias,
		Role:       participant.RoleBuilder,
		Initiative: participant.InitiativeManual,
		Color:      color,
	})
	if err != nil {
		m.history = m.history.AppendSystemRecord(fmt.Sprintf("error: invite %q: %v", alias, err))
		return m
	}
	m.palette = nextPalette
	return m
}

func (m Model) removeAgent(alias string) Model {
	if err := m.sess.Execute(session.RemoveCommand{Alias: alias}); err != nil {
		m.history = m.history.AppendSystemRecord(fmt.Sprintf("error: remove %q: %v", alias, err))
	}
	return m
}

func (m Model) cancelAgent(alias string) Model {
	if err := m.sess.Execute(session.CancelCommand{Alias: alias}); err != nil {
		m.history = m.history.AppendSystemRecord(fmt.Sprintf("error: cancel %q: %v", alias, err))
		return m
	}
	m.history = m.history.AppendSystemRecord("[→ " + alias + "] cancel requested")
	return m
}

func (m Model) sendToAgent(alias, text string) Model {
	err := m.sess.Execute(session.SharedSendCommand{
		Alias:         alias,
		TextDirect:    text,
		TextListeners: fmt.Sprintf("@%s: %s", alias, text),
	})
	if err != nil {
		m.history = m.history.AppendSystemRecord(fmt.Sprintf("error: send to %q: %v", alias, err))
	}
	return m
}

func (m Model) broadcastAll(text string) Model {
	if len(m.sess.RoutableParticipants()) == 0 {
		m.history = m.history.AppendSystemRecord("[no agents — use /invite <alias> to start one]")
		return m
	}
	if err := m.sess.Execute(session.BroadcastCommand{Text: text}); err != nil {
		m.history = m.history.AppendSystemRecord(fmt.Sprintf("error: broadcast: %v", err))
	}
	return m
}

func (m Model) showWho() Model {
	ps := m.sess.Participants()
	if len(ps) == 0 {
		m.history = m.history.AppendSystemRecord("[no agents]")
		return m
	}
	aliases := make([]string, len(ps))
	for i, p := range ps {
		aliases[i] = p.Alias
	}
	slices.Sort(aliases)
	m.history = m.history.AppendSystemRecord("[agents] " + strings.Join(aliases, ", "))
	return m
}

func (m Model) showHelp() Model {
	var b strings.Builder
	b.WriteString("[help]\n")
	b.WriteString("  /invite <alias>   start an agent\n")
	b.WriteString("  /remove <alias>   remove an agent\n")
	b.WriteString("  /cancel <alias>   interrupt an agent's current turn\n")
	b.WriteString("  /who              list agents\n")
	if m.debug {
		b.WriteString("  /debugview        print viewport debug\n")
		b.WriteString("  /debugrows        toggle row number overlay\n")
	}
	b.WriteString("  /help             show this message\n")
	b.WriteString("  @<alias> <text>   send to one agent\n")
	b.WriteString("  <text>            broadcast to all agents\n")
	b.WriteString("  /quit             exit")
	m.history = m.history.AppendSystemRecord(b.String())
	return m
}

func (m Model) debugView() Model {
	if !m.history.Ready() {
		m.history = m.history.AppendSystemRecord("[debug] not ready")
		return m
	}
	m.history = m.history.AppendSystemRecord("[debug]\n" + m.history.DebugSummary())
	return m
}

// syncAfterCompose adjusts the viewport height to match the current compose
// height, preserving the bottom anchor if the viewport was at the bottom.
func (m Model) syncAfterCompose() Model {
	if !m.history.Ready() {
		return m
	}
	newVpH := max(m.lastSize.Height-chromeHeight(m.compose.Height(), m.toolbox.Height()), 1)
	if newVpH == m.history.Height() {
		return m
	}
	wasAtBottom := m.history.AtBottom()
	m.history = m.history.SetHeight(newVpH)
	if wasAtBottom {
		m.history = m.history.GotoBottom()
	}
	return m
}

func (m Model) startEditorCompose() (Model, tea.Cmd) {
	prior := m.compose.Value()
	cmd, err := editor.OpenTempFileInEditor(editor.Request{
		Purpose:          editor.PurposeCompose,
		PriorText:        prior,
		InitialText:      prior,
		TempPattern:      "coderoom-compose-*.md",
		TrimFinalNewline: true,
	})
	if err != nil {
		m.history = m.history.AppendSystemRecord("error: " + err.Error())
		return m, nil
	}
	return m, cmd
}

func (m Model) handleEditorResult(msg editor.Response) Model {
	switch msg.Purpose {
	case editor.PurposeCompose:
		if msg.Canceled || msg.Err != nil {
			m.compose = m.compose.SetValue(msg.PriorText)
		} else {
			m.compose = m.compose.SetValue(msg.NewText)
		}
		return m.syncAfterCompose()
	case editor.PurposeTranscript:
		// Transcript export does not mutate the model; the effect is purely
		// external (opening a read-only temp file in the user's editor).
		return m
	default:
		// Unknown purposes are ignored to avoid corrupting the user's input.
		return m
	}
}

func (m Model) openEditorWithTranscript() (Model, tea.Cmd) {
	content := ansi.Strip(m.history.PlainText())
	cmd, err := editor.OpenTempFileInEditor(editor.Request{
		Purpose:     editor.PurposeTranscript,
		InitialText: content,
		TempPattern: "coderoom-transcript-*.txt",
		ReadOnly:    true,
	})
	if err != nil {
		m.history = m.history.AppendSystemRecord("error: " + err.Error())
		return m, nil
	}
	return m, cmd
}
