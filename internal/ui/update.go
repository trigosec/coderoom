package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/agent/codex"
	"github.com/trigosec/coderoom/internal/participant"
	"github.com/trigosec/coderoom/internal/session"
)

// chromeHeight is the number of terminal rows occupied outside the viewport:
// one separator row and one input row. Adjust here if toolbox rows are added.
const chromeHeight = 2

// Init starts the session event listener; called once by Bubble Tea on startup.
func (m Model) Init() tea.Cmd {
	return awaitEvent(m.queue)
}

// Update handles incoming messages and returns the next model state.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		return m.handleResize(msg), nil
	case sessionEventMsg:
		return m.handleEvent(session.Event(msg)), awaitEvent(m.queue)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		m.sess.Shutdown()
		return m, tea.Quit
	case tea.KeyEnter:
		return m.handleEnter()
	case tea.KeyPgUp:
		m.viewport.HalfPageUp()
		return m, nil
	case tea.KeyPgDown:
		m.viewport.HalfPageDown()
		return m, nil
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

func (m Model) handleResize(msg tea.WindowSizeMsg) Model {
	h := msg.Height - chromeHeight
	if !m.ready {
		m.viewport = viewport.New(msg.Width, h)
		m.viewport.SetContent(strings.Join(m.lines, "\n"))
		m.ready = true
	} else {
		m.viewport.Width = msg.Width
		m.viewport.Height = h
	}
	m.input.Width = msg.Width
	return m
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	raw := m.input.Value()
	m.input.Reset()
	action, err := Parse(raw)
	if err != nil {
		return m.appendLine("error: " + err.Error()), nil
	}
	return m.executeAction(action)
}

func (m Model) handleEvent(e session.Event) Model {
	switch e.Kind {
	case session.KindAgentStarted:
		m.agents = append(m.agents, e.Alias)
		return m.appendLine("[" + e.Alias + " joined]")
	case session.KindAgentStopped:
		m = m.removeAlias(e.Alias)
		return m.appendLine("[" + e.Alias + " left]")
	case session.KindAgentCrashed:
		m = m.removeAlias(e.Alias)
		return m.appendLine("[" + e.Alias + " crashed]")
	case session.KindBroadcast:
		return m.appendLine("[all] " + e.Text)
	case session.KindSharedSend:
		return m.appendLine("[-> " + e.Alias + "] " + e.Text)
	case session.KindSharedNotice:
		return m.appendLine("[notice -> " + e.Alias + "]")
	case session.KindDelta:
		return m.handleDelta(e.Alias, e.Text)
	case session.KindDone:
		return m.endStream(e.Alias)
	}
	return m
}

func (m Model) handleDelta(alias, text string) Model {
	if idx, ok := m.streaming[alias]; ok {
		m.lines[idx] += text
	} else {
		m.streaming[alias] = len(m.lines)
		m.lines = append(m.lines, alias+"> "+text)
	}
	return m.syncViewport()
}

func (m Model) endStream(alias string) Model {
	delete(m.streaming, alias)
	return m
}

func (m Model) executeAction(a Action) (Model, tea.Cmd) {
	switch act := a.(type) {
	case Invite:
		return m.inviteAgent(act.Alias)
	case Stop:
		return m.stopAgent(act.Alias)
	case Send:
		return m.sendToAgent(act.Alias, act.Text)
	case Broadcast:
		return m.broadcastAll(act.Text)
	case Who:
		return m.showWho(), nil
	case Help:
		return m.showHelp(), nil
	case Quit:
		m.sess.Shutdown()
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) inviteAgent(alias string) (Model, tea.Cmd) {
	err := m.sess.Execute(session.InviteCommand{
		Alias:      alias,
		Agent:      codex.New(m.cwd),
		Role:       participant.RoleBuilder,
		Initiative: participant.InitiativeManual,
	})
	if err != nil {
		return m.appendLine(fmt.Sprintf("error: invite %q: %v", alias, err)), nil
	}
	return m, nil
}

func (m Model) stopAgent(alias string) (Model, tea.Cmd) {
	if err := m.sess.Execute(session.StopCommand{Alias: alias}); err != nil {
		return m.appendLine(fmt.Sprintf("error: stop %q: %v", alias, err)), nil
	}
	return m, nil
}

func (m Model) sendToAgent(alias, text string) (Model, tea.Cmd) {
	err := m.sess.Execute(session.SharedSendCommand{
		Alias:         alias,
		TextDirect:    text,
		TextListeners: fmt.Sprintf("@%s: %s", alias, text),
	})
	if err != nil {
		return m.appendLine(fmt.Sprintf("error: send to %q: %v", alias, err)), nil
	}
	return m, nil
}

func (m Model) broadcastAll(text string) (Model, tea.Cmd) {
	if err := m.sess.Execute(session.BroadcastCommand{Text: text}); err != nil {
		return m.appendLine(fmt.Sprintf("error: broadcast: %v", err)), nil
	}
	return m, nil
}

func (m Model) showWho() Model {
	if len(m.agents) == 0 {
		return m.appendLine("[no agents]")
	}
	return m.appendLine("[agents] " + strings.Join(m.agents, ", "))
}

func (m Model) showHelp() Model {
	m.lines = append(m.lines,
		"[help]",
		"  /invite <alias>   start an agent",
		"  /stop <alias>     stop an agent",
		"  /who              list active agents",
		"  /help             show this message",
		"  @<alias> <text>   send to one agent",
		"  <text>            broadcast to all agents",
		"  /quit             exit",
	)
	return m.syncViewport()
}

func (m Model) appendLine(line string) Model {
	m.lines = append(m.lines, line)
	return m.syncViewport()
}

func (m Model) syncViewport() Model {
	if !m.ready {
		return m
	}
	m.viewport.SetContent(strings.Join(m.lines, "\n"))
	m.viewport.GotoBottom()
	return m
}

func (m Model) removeAlias(alias string) Model {
	delete(m.streaming, alias)
	updated := make([]string, 0, len(m.agents))
	for _, a := range m.agents {
		if a != alias {
			updated = append(updated, a)
		}
	}
	m.agents = updated
	return m
}
