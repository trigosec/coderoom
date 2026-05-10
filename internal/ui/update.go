package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
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
)

func chromeHeight(inputHeight int) int {
	// separator + input + bottom margin
	return 1 + inputHeight + marginV
}

func inputMaxHeight(totalHeight int) int {
	return min(8, max(totalHeight/3, 1))
}

func desiredInputHeight(lineCount, maxHeight int) int {
	return min(max(lineCount, 1), maxHeight)
}

func updateInputDecorations(input textarea.Model) textarea.Model {
	if input.LineCount() >= 2 {
		input.ShowLineNumbers = false
		input.SetPromptFunc(6, func(lineIndex int) string {
			// ❯   1 a
			// ❯   2 b
			return fmt.Sprintf("❯%4d ", lineIndex+1)
		})
		return input
	}
	input.ShowLineNumbers = false
	input.SetPromptFunc(6, func(_ int) string {
		// Keep the text column aligned with the multi-line prompt:
		// single-line: ❯     a
		return "❯     "
	})
	return input
}

// Init starts the session event listener; called once by Bubble Tea on startup.
func (m Model) Init() tea.Cmd {
	return tea.Batch(awaitEvent(m.queue), textarea.Blink)
}

// Update handles incoming messages and returns the next model state.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		return m.handleResize(msg), nil
	case editorComposeMsg:
		return m.handleEditorCompose(msg), nil
	case sessionEventMsg:
		return m.handleEvent(session.Event(msg)), awaitEvent(m.queue)
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlO {
		return m.toggleFocus()
	}

	// PgUp/PgDn always scroll the viewport, regardless of focus.
	if msg.Type == tea.KeyPgUp {
		m.viewport.HalfPageUp()
		return m, nil
	}
	if msg.Type == tea.KeyPgDown {
		m.viewport.HalfPageDown()
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
		m.input.Blur()
		return m, nil
	}
	m.focus = focusComposer
	cmd := m.input.Focus()
	return m, cmd
}

func (m Model) handleComposerKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		if m.input.Value() == "" {
			return m, nil
		}
		m.input.Reset()
		m = m.resizeForInput()
		return m, nil
	case tea.KeyCtrlG:
		return m.startEditorCompose()
	case tea.KeyEnter:
		if msg.Alt {
			m.input.InsertRune('\n')
			m = m.resizeForInput()
			return m, nil
		}
		return m.handleEnter()
	case tea.KeyEsc:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m = m.resizeForInput()
		return m, cmd
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m = m.resizeForInput()
		return m, cmd
	}
}

func (m Model) handleViewportKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, nil
	case tea.KeyCtrlG:
		return m.openEditorWithTranscript()
	case tea.KeyHome:
		m.viewport.GotoTop()
		return m, nil
	case tea.KeyEnd:
		m.viewport.GotoBottom()
		return m, nil
	case tea.KeyEsc:
		m.focus = focusComposer
		cmd := m.input.Focus()
		return m, cmd
	default:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
}

func (m Model) handleResize(msg tea.WindowSizeMsg) Model {
	m.lastSize = msg
	m.input = updateInputDecorations(m.input)
	maxInputH := inputMaxHeight(msg.Height)
	inputH := desiredInputHeight(m.input.LineCount(), maxInputH)
	h := msg.Height - chromeHeight(inputH)
	inner := max(msg.Width-2*marginH, 1)
	h = max(h, 1)
	if !m.ready {
		m.viewport = viewport.New(inner, h)
		m.ready = true
	} else {
		m.viewport.Width = inner
		m.viewport.Height = h
	}
	m.input.SetWidth(inner)
	m.input.SetHeight(inputH)
	colorFor := m.colorFor()
	for i, r := range m.records {
		m.renderedRecords[i] = renderRecord(r, m.viewport.Width, colorFor)
	}
	return m.syncViewport()
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	raw := m.input.Value()
	m.input.Reset()
	m = m.resizeForInput()
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
	if next, ok := m.handleAgentLifecycleEvent(e); ok {
		return next
	}
	return m.handleMessageEvent(e)
}

func (m Model) handleAgentLifecycleEvent(e session.Event) (Model, bool) {
	switch e.Kind {
	case session.KindAgentStarting:
		return m.appendRecord(record{kind: recordKindSystem, body: "[" + e.Alias + " starting]"}), true
	case session.KindAgentStarted:
		return m.appendRecord(record{kind: recordKindSystem, body: "[" + e.Alias + " joined]"}), true
	case session.KindAgentStopped:
		delete(m.streaming, e.Alias)
		m = m.markDeparted(e.Alias)
		return m.appendRecord(record{kind: recordKindSystem, body: "[" + e.Alias + " left]"}), true
	case session.KindAgentCrashed:
		delete(m.streaming, e.Alias)
		m = m.markDeparted(e.Alias)
		return m.appendRecord(record{kind: recordKindSystem, body: "[" + e.Alias + " crashed]"}), true
	default:
		return m, false
	}
}

func (m Model) handleMessageEvent(e session.Event) Model {
	switch e.Kind {
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
	default:
		return m
	}
}

func (m Model) handleDelta(alias, text string) Model {
	wasAtBottom := m.viewport.AtBottom()
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
	if wasAtBottom {
		m.viewport.GotoBottom()
	}
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
		Agent:      m.agentFactory(alias, m.cwd),
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
	wasAtBottom := m.viewport.AtBottom()
	m.records = append(m.records, r)
	m.renderedRecords = append(m.renderedRecords, renderRecord(r, m.viewport.Width, m.colorFor()))
	m = m.syncViewport()
	if wasAtBottom {
		m.viewport.GotoBottom()
	}
	return m
}

func (m Model) syncViewport() Model {
	if !m.ready {
		return m
	}
	m.viewport.SetContent(strings.Join(m.renderedRecords, "\n\n"))
	return m
}

func (m Model) resizeForInput() Model {
	if !m.ready {
		return m
	}
	m.input = updateInputDecorations(m.input)
	wasAtBottom := m.viewport.AtBottom()
	maxInputH := inputMaxHeight(m.lastSize.Height)
	inputH := desiredInputHeight(m.input.LineCount(), maxInputH)
	if inputH != m.input.Height() {
		m.input.SetHeight(inputH)
		m.viewport.Height = max(m.lastSize.Height-chromeHeight(inputH), 1)
		m = m.syncViewport()
		if wasAtBottom {
			m.viewport.GotoBottom()
		}
	}
	return m
}

type editorComposeMsg struct {
	prior    string
	content  string
	err      error
	canceled bool
}

func (m Model) startEditorCompose() (Model, tea.Cmd) {
	editor := os.Getenv("EDITOR")
	if strings.TrimSpace(editor) == "" {
		editor = os.Getenv("VISUAL")
	}
	if strings.TrimSpace(editor) == "" {
		return m.appendRecord(record{
			kind: recordKindSystem,
			body: "error: no editor configured (set $EDITOR or $VISUAL)",
		}), nil
	}

	prior := m.input.Value()
	f, err := os.CreateTemp("", "coderoom-compose-*.md")
	if err != nil {
		return m.appendRecord(record{kind: recordKindSystem, body: fmt.Sprintf("error: compose: %v", err)}), nil
	}
	path := f.Name()
	if _, err := f.WriteString(prior); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return m.appendRecord(record{kind: recordKindSystem, body: fmt.Sprintf("error: compose: %v", err)}), nil
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return m.appendRecord(record{kind: recordKindSystem, body: fmt.Sprintf("error: compose: %v", err)}), nil
	}

	args := strings.Fields(editor)
	//nolint:gosec // $EDITOR/$VISUAL is explicitly user-configured; we execute it with a temp file path.
	cmd := exec.Command(args[0], append(args[1:], path)...)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer func() { _ = os.Remove(path) }()
		if err != nil {
			return editorComposeMsg{prior: prior, canceled: true, err: err}
		}
		b, readErr := os.ReadFile(filepath.Clean(path))
		if readErr != nil {
			return editorComposeMsg{prior: prior, canceled: true, err: readErr}
		}
		content := string(b)
		content = strings.TrimSuffix(content, "\n")
		return editorComposeMsg{prior: prior, content: content}
	})
}

func (m Model) handleEditorCompose(msg editorComposeMsg) Model {
	if msg.canceled || msg.err != nil {
		m.input.SetValue(msg.prior)
		m = m.resizeForInput()
		return m
	}
	m.input.SetValue(msg.content)
	m = m.resizeForInput()
	return m
}

func (m Model) openEditorWithTranscript() (Model, tea.Cmd) {
	editor := os.Getenv("EDITOR")
	if strings.TrimSpace(editor) == "" {
		editor = os.Getenv("VISUAL")
	}
	if strings.TrimSpace(editor) == "" {
		return m.appendRecord(record{
			kind: recordKindSystem,
			body: "error: no editor configured (set $EDITOR or $VISUAL)",
		}), nil
	}

	content := strings.Join(m.renderedRecords, "\n\n")
	content = ansi.Strip(content)

	f, err := os.CreateTemp("", "coderoom-transcript-*.txt")
	if err != nil {
		return m.appendRecord(record{kind: recordKindSystem, body: fmt.Sprintf("error: transcript export: %v", err)}), nil
	}
	path := f.Name()
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return m.appendRecord(record{kind: recordKindSystem, body: fmt.Sprintf("error: transcript export: %v", err)}), nil
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return m.appendRecord(record{kind: recordKindSystem, body: fmt.Sprintf("error: transcript export: %v", err)}), nil
	}
	if err := os.Chmod(path, 0o400); err != nil {
		_ = os.Remove(path)
		return m.appendRecord(record{kind: recordKindSystem, body: fmt.Sprintf("error: transcript export: %v", err)}), nil
	}

	args := strings.Fields(editor)
	//nolint:gosec // $EDITOR/$VISUAL is explicitly user-configured; we execute it with a temp file path.
	cmd := exec.Command(args[0], append(args[1:], path)...)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer func() { _ = os.Remove(path) }()
		_ = err
		return nil
	})
}
