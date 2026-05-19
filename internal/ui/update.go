package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/participant"
	"github.com/trigosec/coderoom/internal/session"
	"github.com/trigosec/coderoom/internal/ui/room"
)

const (
	// marginH is the number of columns reserved on each horizontal side. Only a
	// left prefix is applied in View(); the right margin is implicit because
	// viewport, separator, and input are all sized to inner = width-2*marginH.
	marginH = 2
	// marginV is the number of empty rows below the input.
	marginV = 1
)

// Init starts the session event listener; called once by Bubble Tea on startup.
func (m Model) Init() tea.Cmd {
	return tea.Batch(awaitEvent(m.queue), m.room.Init())
}

// Update handles incoming messages and returns the next model state.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		return m.handleResize(msg), nil
	case sessionEventMsg:
		next, cmd := m.handleEvent(session.Event(msg))
		return next, tea.Batch(cmd, awaitEvent(m.queue))
	case room.SubmitMsg:
		return m.handleSubmit(msg.Text)
	default:
		var roomCmd tea.Cmd
		m.room, roomCmd = m.room.Update(msg)
		var toolboxCmd tea.Cmd
		m.toolbox, toolboxCmd = m.toolbox.Update(msg)
		return m, tea.Batch(roomCmd, toolboxCmd)
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.room, cmd = m.room.Update(msg)
	return m, cmd
}

func (m Model) handleResize(msg tea.WindowSizeMsg) Model {
	m.lastSize = msg
	inner := max(msg.Width-2*marginH, 1)
	m.toolbox = m.toolbox.SetWidth(inner)
	roomH := max(msg.Height-(m.toolbox.Height()+marginV), 1)
	m.room = m.room.HandleResize(inner, roomH)
	m.room = m.room.SetDebug(m.debug)
	return m
}

func (m Model) handleSubmit(raw string) (Model, tea.Cmd) {
	if strings.TrimSpace(raw) == "" {
		return m, nil
	}
	action, err := Parse(raw)
	var routing []string
	if err == nil {
		routing = routingFor(action, m.sess.RoutableParticipants())
	}
	m.room = m.room.AppendUserInput(raw, routing)
	if err != nil {
		m.room = m.room.AppendSystem("error: " + err.Error())
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
		m.room = m.room.AppendSystem("[" + e.Alias + " starting]")
		return m, true
	case session.KindAgentStarted:
		m.room = m.room.AppendSystem("[" + e.Alias + " joined]")
		return m, true
	case session.KindAgentStopped:
		m.room = m.room.MarkDeparted(e.Alias)
		m.room = m.room.AppendSystem("[" + e.Alias + " left]")
		return m, true
	case session.KindAgentCrashed:
		m.room = m.room.MarkDeparted(e.Alias)
		m.room = m.room.AppendSystem("[" + e.Alias + " crashed]")
		return m, true
	default:
		return m, false
	}
}

func (m Model) handleMessageEvent(e session.Event) Model {
	switch e.Kind {
	case session.KindSharedNotice:
		m.room = m.room.AppendSystem("[notice → " + e.Alias + "]")
	case session.KindAgentLog:
		m.room = m.room.AppendLog(e.Alias, e.Text)
	case session.KindDelta:
		m.room = m.room.HandleDelta(e.Alias, e.Text)
	case session.KindReasoningDelta:
		m.room = m.room.HandleReasoningDelta(e.Alias, e.Text)
	case session.KindReasoningContinue:
		m.room = m.room.HandleReasoningContinue(e.Alias)
	case session.KindDone:
		m.room = m.room.HandleDone(e.Alias)
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
			m.room = m.room.AppendSystem("error: debug commands disabled (set CODEROOM_DEBUG=1)")
			return m, true
		}
		return m.debugView(), true
	case DebugRows:
		if !m.debug {
			m.room = m.room.AppendSystem("error: debug commands disabled (set CODEROOM_DEBUG=1)")
			return m, true
		}
		m.room = m.room.ToggleDebugRowNums()
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
		m.room = m.room.AppendSystem(fmt.Sprintf("error: invite %q: %v", alias, err))
		return m
	}
	m.palette = nextPalette
	return m
}

func (m Model) removeAgent(alias string) Model {
	if err := m.sess.Execute(session.RemoveCommand{Alias: alias}); err != nil {
		m.room = m.room.AppendSystem(fmt.Sprintf("error: remove %q: %v", alias, err))
	}
	return m
}

func (m Model) cancelAgent(alias string) Model {
	if err := m.sess.Execute(session.CancelCommand{Alias: alias}); err != nil {
		m.room = m.room.AppendSystem(fmt.Sprintf("error: cancel %q: %v", alias, err))
		return m
	}
	m.room = m.room.AppendSystem("[→ " + alias + "] cancel requested")
	return m
}

func (m Model) sendToAgent(alias, text string) Model {
	err := m.sess.Execute(session.SharedSendCommand{
		Alias:         alias,
		TextDirect:    text,
		TextListeners: fmt.Sprintf("@%s: %s", alias, text),
	})
	if err != nil {
		m.room = m.room.AppendSystem(fmt.Sprintf("error: send to %q: %v", alias, err))
	}
	return m
}

func (m Model) broadcastAll(text string) Model {
	if len(m.sess.RoutableParticipants()) == 0 {
		m.room = m.room.AppendSystem("[no agents — use /invite <alias> to start one]")
		return m
	}
	if err := m.sess.Execute(session.BroadcastCommand{Text: text}); err != nil {
		m.room = m.room.AppendSystem(fmt.Sprintf("error: broadcast: %v", err))
	}
	return m
}

func (m Model) showWho() Model {
	ps := m.sess.Participants()
	if len(ps) == 0 {
		m.room = m.room.AppendSystem("[no agents]")
		return m
	}
	aliases := make([]string, len(ps))
	for i, p := range ps {
		aliases[i] = p.Alias
	}
	slices.Sort(aliases)
	m.room = m.room.AppendSystem("[agents] " + strings.Join(aliases, ", "))
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
	m.room = m.room.AppendSystem(b.String())
	return m
}

func (m Model) debugView() Model {
	if !m.room.Ready() {
		m.room = m.room.AppendSystem("[debug] not ready")
		return m
	}
	m.room = m.room.AppendSystem("[debug]\n" + m.room.HistoryDebugSummary())
	return m
}
