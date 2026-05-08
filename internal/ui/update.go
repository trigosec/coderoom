package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/agent/codex"
	"github.com/trigosec/coderoom/internal/participant"
	"github.com/trigosec/coderoom/internal/session"
)

const (
	// marginH is the number of columns reserved on each horizontal side. Only a
	// left prefix is applied in View(); the right margin is implicit because
	// viewport, separator, and input are all sized to inner = width-2*marginH.
	marginH = 2
	// marginV is the number of empty rows below the input.
	marginV = 1
	// chromeHeight is the number of terminal rows occupied outside the viewport:
	// separator + input + bottom margin. Adjust here if toolbox rows are added.
	chromeHeight = 2 + marginV
)

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
	inner := max(msg.Width-2*marginH, 1)
	h = max(h, 1)
	if !m.ready {
		m.viewport = viewport.New(inner, h)
		m.ready = true
	} else {
		m.viewport.Width = inner
		m.viewport.Height = h
	}
	m.input.Width = inner
	colorFor := m.colorFor()
	for i, r := range m.records {
		m.renderedRecords[i] = renderRecord(r, m.viewport.Width, colorFor)
	}
	return m.syncViewport()
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	raw := m.input.Value()
	m.input.Reset()
	if strings.TrimSpace(raw) == "" {
		return m, nil
	}
	action, err := Parse(raw)
	var routing []string
	if err == nil {
		routing = routingFor(action, m.sess.Participants())
	}
	m = m.appendRecord(record{kind: recordKindUserInput, body: raw, routing: routing})
	if err != nil {
		return m.appendRecord(record{kind: recordKindSystem, body: "error: " + err.Error()}), nil
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

// colorFor returns a lookup function that resolves an alias to its assigned
// colour. Active agents are looked up in the session registry. Departed agents
// return ColorDeparted (a muted grey) so their historical records dim instead
// of losing colour entirely on resize.
func (m Model) colorFor() func(string) string {
	return func(alias string) string {
		if p, ok := m.sess.Participant(alias); ok {
			return p.Color
		}
		if m.departed[alias] {
			return ColorDeparted
		}
		return ""
	}
}

func (m Model) handleEvent(e session.Event) Model {
	switch e.Kind {
	case session.KindAgentStarted:
		return m.appendRecord(record{kind: recordKindSystem, body: "[" + e.Alias + " joined]"})
	case session.KindAgentStopped:
		delete(m.streaming, e.Alias)
		m = m.markDeparted(e.Alias)
		return m.appendRecord(record{kind: recordKindSystem, body: "[" + e.Alias + " left]"})
	case session.KindAgentCrashed:
		delete(m.streaming, e.Alias)
		m = m.markDeparted(e.Alias)
		return m.appendRecord(record{kind: recordKindSystem, body: "[" + e.Alias + " crashed]"})
	case session.KindBroadcast:
		return m.appendRecord(record{kind: recordKindSystem, body: "[all] " + e.Text})
	case session.KindSharedSend:
		return m.appendRecord(record{kind: recordKindSystem, body: "[→ " + e.Alias + "] " + e.Text})
	case session.KindSharedNotice:
		return m.appendRecord(record{kind: recordKindSystem, body: "[notice → " + e.Alias + "]"})
	case session.KindAgentLog:
		return m.appendRecord(record{kind: recordKindLog, alias: e.Alias, body: e.Text})
	case session.KindDelta:
		return m.handleDelta(e.Alias, e.Text)
	case session.KindDone:
		delete(m.streaming, e.Alias)
		return m
	}
	return m
}

func (m Model) handleDelta(alias, text string) Model {
	colorFor := m.colorFor()
	if idx, ok := m.streaming[alias]; ok {
		m.records[idx].body += text
		m.renderedRecords[idx] = renderRecord(m.records[idx], m.viewport.Width, colorFor)
	} else {
		idx = len(m.records)
		m.streaming[alias] = idx
		r := record{kind: recordKindAgentOutput, alias: alias, body: text}
		m.records = append(m.records, r)
		m.renderedRecords = append(m.renderedRecords, renderRecord(r, m.viewport.Width, colorFor))
	}
	m = m.syncViewport()
	m.viewport.GotoBottom()
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
	color, nextPalette := m.palette.Next()
	err := m.sess.Execute(session.InviteCommand{
		Alias:      alias,
		Agent:      codex.New(m.cwd),
		Role:       participant.RoleBuilder,
		Initiative: participant.InitiativeManual,
		Color:      color,
	})
	if err != nil {
		return m.appendRecord(record{kind: recordKindSystem, body: fmt.Sprintf("error: invite %q: %v", alias, err)}), nil
	}
	m.palette = nextPalette
	return m, nil
}

func (m Model) stopAgent(alias string) (Model, tea.Cmd) {
	if err := m.sess.Execute(session.StopCommand{Alias: alias}); err != nil {
		return m.appendRecord(record{kind: recordKindSystem, body: fmt.Sprintf("error: stop %q: %v", alias, err)}), nil
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
		return m.appendRecord(record{kind: recordKindSystem, body: fmt.Sprintf("error: send to %q: %v", alias, err)}), nil
	}
	return m, nil
}

func (m Model) broadcastAll(text string) (Model, tea.Cmd) {
	if len(m.sess.Participants()) == 0 {
		return m.appendRecord(record{kind: recordKindSystem, body: "[no agents — use /invite <alias> to start one]"}), nil
	}
	if err := m.sess.Execute(session.BroadcastCommand{Text: text}); err != nil {
		return m.appendRecord(record{kind: recordKindSystem, body: fmt.Sprintf("error: broadcast: %v", err)}), nil
	}
	return m, nil
}

func (m Model) showWho() Model {
	ps := m.sess.Participants()
	if len(ps) == 0 {
		return m.appendRecord(record{kind: recordKindSystem, body: "[no agents]"})
	}
	aliases := make([]string, len(ps))
	for i, p := range ps {
		aliases[i] = p.Alias
	}
	slices.Sort(aliases)
	return m.appendRecord(record{kind: recordKindSystem, body: "[agents] " + strings.Join(aliases, ", ")})
}

func (m Model) showHelp() Model {
	body := "[help]\n" +
		"  /invite <alias>   start an agent\n" +
		"  /stop <alias>     stop an agent\n" +
		"  /who              list active agents\n" +
		"  /help             show this message\n" +
		"  @<alias> <text>   send to one agent\n" +
		"  <text>            broadcast to all agents\n" +
		"  /quit             exit"
	return m.appendRecord(record{kind: recordKindSystem, body: body})
}

// markDeparted records alias as departed and re-renders every record that
// references it, so that historical output dims to grey immediately (and
// stays grey on subsequent resizes).
func (m Model) markDeparted(alias string) Model {
	m.departed[alias] = true
	colorFor := m.colorFor()
	for i, r := range m.records {
		if r.alias == alias || slices.Contains(r.routing, alias) {
			m.renderedRecords[i] = renderRecord(r, m.viewport.Width, colorFor)
		}
	}
	return m
}

func (m Model) appendRecord(r record) Model {
	m.records = append(m.records, r)
	m.renderedRecords = append(m.renderedRecords, renderRecord(r, m.viewport.Width, m.colorFor()))
	m = m.syncViewport()
	m.viewport.GotoBottom()
	return m
}

func (m Model) syncViewport() Model {
	if !m.ready {
		return m
	}
	m.viewport.SetContent(strings.Join(m.renderedRecords, "\n\n"))
	return m
}
