package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
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
	case editor.Response:
		return m.handleEditorResult(msg), nil
	case sessionEventMsg:
		next, cmd := m.handleEvent(session.Event(msg))
		return next, tea.Batch(cmd, awaitEvent(m.queue))
	default:
		var inputCmd tea.Cmd
		m.input, inputCmd = m.input.Update(msg)
		var toolboxCmd tea.Cmd
		m.toolbox, toolboxCmd = m.toolbox.Update(msg)
		return m, tea.Batch(inputCmd, toolboxCmd)
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
	case tea.KeyUp:
		m.viewport.ScrollUp(1)
		return m, nil
	case tea.KeyDown:
		m.viewport.ScrollDown(1)
		return m, nil
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
	inner := max(msg.Width-2*marginH, 1)
	m.toolbox = m.toolbox.SetWidth(inner)
	h := msg.Height - chromeHeight(inputH, m.toolbox.Height())
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
		routing = routingFor(action, m.sess.RoutableParticipants())
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
		out := m.inviteAgent(act.Alias)
		return out, true
	case Remove:
		out := m.removeAgent(act.Alias)
		return out, true
	case Cancel:
		out := m.cancelAgent(act.Alias)
		return out, true
	case Send:
		out := m.sendToAgent(act.Alias, act.Text)
		return out, true
	case Broadcast:
		out := m.broadcastAll(act.Text)
		return out, true
	default:
		return m, false
	}
}

func (m Model) executeDebugAction(a Action) (Model, bool) {
	switch a.(type) {
	case DebugView:
		if !m.debug {
			return m.appendRecord(record{kind: recordKindSystem, body: "error: debug commands disabled (set CODEROOM_DEBUG=1)"}), true
		}
		return m.debugView(), true
	case DebugRows:
		if !m.debug {
			return m.appendRecord(record{kind: recordKindSystem, body: "error: debug commands disabled (set CODEROOM_DEBUG=1)"}), true
		}
		m.debugRowNums = !m.debugRowNums
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
		return m.appendRecord(record{kind: recordKindSystem, body: fmt.Sprintf("error: invite %q: %v", alias, err)})
	}
	m.palette = nextPalette
	return m
}

func (m Model) removeAgent(alias string) Model {
	if err := m.sess.Execute(session.RemoveCommand{Alias: alias}); err != nil {
		return m.appendRecord(record{kind: recordKindSystem, body: fmt.Sprintf("error: remove %q: %v", alias, err)})
	}
	return m
}

func (m Model) cancelAgent(alias string) Model {
	if err := m.sess.Execute(session.CancelCommand{Alias: alias}); err != nil {
		return m.appendRecord(record{kind: recordKindSystem, body: fmt.Sprintf("error: cancel %q: %v", alias, err)})
	}
	return m.appendRecord(record{kind: recordKindSystem, body: "[→ " + alias + "] cancel requested"})
}

func (m Model) sendToAgent(alias, text string) Model {
	err := m.sess.Execute(session.SharedSendCommand{
		Alias:         alias,
		TextDirect:    text,
		TextListeners: fmt.Sprintf("@%s: %s", alias, text),
	})
	if err != nil {
		return m.appendRecord(record{kind: recordKindSystem, body: fmt.Sprintf("error: send to %q: %v", alias, err)})
	}
	return m
}

func (m Model) broadcastAll(text string) Model {
	if len(m.sess.RoutableParticipants()) == 0 {
		return m.appendRecord(record{kind: recordKindSystem, body: "[no agents — use /invite <alias> to start one]"})
	}
	if err := m.sess.Execute(session.BroadcastCommand{Text: text}); err != nil {
		return m.appendRecord(record{kind: recordKindSystem, body: fmt.Sprintf("error: broadcast: %v", err)})
	}
	return m
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
	return m.appendRecord(record{kind: recordKindSystem, body: b.String()})
}

func (m Model) debugView() Model {
	if !m.ready {
		return m.appendRecord(record{kind: recordKindSystem, body: "[debug] not ready"})
	}
	view := ansi.Strip(strings.TrimSuffix(m.viewport.View(), "\n"))
	lines := []string{}
	if view != "" {
		lines = strings.Split(view, "\n")
	}
	if len(lines) > 8 {
		lines = lines[:8]
	}
	body := "[debug]\n" +
		fmt.Sprintf("  y=%d h=%d rec=%d ln=%d\n", m.viewport.YOffset, m.viewport.Height, len(m.records), len(m.renderedRecords)) +
		"  viewTop:\n"
	for _, line := range lines {
		body += "    " + line + "\n"
	}
	body = strings.TrimSuffix(body, "\n")
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
	// Avoid inserting blank separator lines between records. In small terminals
	// those blank lines can consume the entire viewport and make it appear as if
	// older records (like the first echoed command) are missing.
	m.viewport.SetContent(strings.Join(m.renderedRecords, "\n"))
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
		m.viewport.Height = max(m.lastSize.Height-chromeHeight(inputH, m.toolbox.Height()), 1)
		m = m.syncViewport()
		if wasAtBottom {
			m.viewport.GotoBottom()
		}
	}
	return m
}

func (m Model) startEditorCompose() (Model, tea.Cmd) {
	prior := m.input.Value()
	cmd, err := editor.OpenTempFileInEditor(editor.Request{
		Purpose:          editor.PurposeCompose,
		PriorText:        prior,
		InitialText:      prior,
		TempPattern:      "coderoom-compose-*.md",
		TrimFinalNewline: true,
	})
	if err != nil {
		return m.appendRecord(record{kind: recordKindSystem, body: "error: " + err.Error()}), nil
	}
	return m, cmd
}

func (m Model) handleEditorResult(msg editor.Response) Model {
	switch msg.Purpose {
	case editor.PurposeCompose:
		if msg.Canceled || msg.Err != nil {
			m.input.SetValue(msg.PriorText)
			return m.resizeForInput()
		}
		m.input.SetValue(msg.NewText)
		return m.resizeForInput()
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
	content := strings.Join(m.renderedRecords, "\n\n")
	content = ansi.Strip(content)
	cmd, err := editor.OpenTempFileInEditor(editor.Request{
		Purpose:     editor.PurposeTranscript,
		InitialText: content,
		TempPattern: "coderoom-transcript-*.txt",
		ReadOnly:    true,
	})
	if err != nil {
		return m.appendRecord(record{kind: recordKindSystem, body: "error: " + err.Error()}), nil
	}
	return m, cmd
}
