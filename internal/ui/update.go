package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/participant"
	"github.com/trigosec/coderoom/internal/session"
	"github.com/trigosec/coderoom/internal/ui/room"
	"github.com/trigosec/coderoom/internal/ui/room/staging"
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
	default:
		return m.handleNonSessionMessage(msg)
	}
}

func (m Model) handleNonSessionMessage(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case room.SubmitMsg:
		return m.handleSubmit(msg.Text)
	case room.ApprovalDecisionMsg:
		return m.handleApprovalDecision(msg)
	case room.StagedEditMsg, room.StagedClearMsg:
		m.room = m.room.ClearComposerStaged()
		return m, nil
	case room.StagedInterruptMsg:
		next := m.handleStagedInterrupt()
		return next, nil
	default:
		return m.forwardMessage(msg)
	}
}

func (m Model) handleApprovalDecision(msg room.ApprovalDecisionMsg) (tea.Model, tea.Cmd) {
	cmd := session.ResolveApprovalCommand{ApprovalID: m.activeApprovalID, Choice: msg.Choice}
	if err := m.sess.Execute(cmd); err != nil {
		m.room = m.room.AppendSystem(fmt.Sprintf("error: resolve approval: %v", err))
		return m, nil
	}
	m.activeApprovalID = 0
	return m, nil
}

func (m Model) forwardMessage(msg tea.Msg) (tea.Model, tea.Cmd) {
	var roomCmd tea.Cmd
	m.room, roomCmd = m.room.Update(msg)
	var toolboxCmd tea.Cmd
	m.toolbox, toolboxCmd = m.toolbox.Update(msg)
	return m, tea.Batch(roomCmd, toolboxCmd)
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
	if m.showStartupHelpTip && m.room.Ready() && len(m.room.HistoryRecords()) == 0 {
		m.room = m.room.AppendSystem("tip: type /help for commands and shortcuts")
		m.showStartupHelpTip = false
	}
	return m
}

func (m Model) handleSubmit(raw string) (Model, tea.Cmd) {
	if strings.TrimSpace(raw) == "" {
		return m, nil
	}
	if m.room.HasStagedBatch() {
		// This should be prevented by the room, but keep it defensive.
		m.room = m.room.AppendSystem("error: message already staged (Esc to edit, Ctrl+X to send)")
		return m, nil
	}
	action, err := Parse(raw)
	if err != nil {
		var unknown UnknownCommandError
		if errors.As(err, &unknown) {
			m.room = m.room.AppendSystem("error: " + err.Error() + " (type /help)")
			m.room = m.room.SetComposeValue("")
			return m, nil
		}
		m.room = m.room.AppendSystem("error: " + err.Error())
		m.room = m.room.SetComposeValue("")
		return m, nil
	}

	// Barrier-batch applies to user-authored Send/Broadcast only.
	switch action.(type) {
	case Send, Broadcast:
		return m.handleBarrierBatchSubmit(raw, action), nil
	default:
	}

	routing := routingFor(action, m.sess.RoutableParticipants())
	m.room = m.room.AppendUserInput(raw, routing)
	m.room = m.room.SetComposeValue("")
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

func (m Model) dispatchRoomStagedBatch() Model {
	act, targets, ok := m.room.StagedDispatchCandidate()
	if !ok {
		m.room = m.room.AppendSystem("error: internal: no staged batch to dispatch")
		return m
	}
	if len(targets) == 0 {
		m.room = m.room.ClearComposerStaged()
		m.room = m.room.AppendSystem("staged message discarded: no active targets")
		return m
	}

	var delivered []string
	var err error
	switch act.Kind {
	case staging.ActionBroadcast:
		m, delivered, err = m.executeBroadcastAll(act.Text)
	case staging.ActionSend:
		if !slices.Contains(targets, act.Alias) {
			m.room = m.room.ClearComposerStaged()
			m.room = m.room.AppendSystem(fmt.Sprintf("staged message discarded: %q is no longer available", act.Alias))
			return m
		}
		m, delivered, err = m.executeSendToAgent(act.Alias, act.Text)
	default:
		m.room = m.room.AppendSystem("error: internal: staged action invalid")
		return m
	}
	if err != nil {
		if len(delivered) > 0 {
			m.room = m.room.CommitStagedBatchDispatch(delivered)
		} else {
			m.room = m.room.ClearComposerStaged()
		}
		return m
	}
	m.room = m.room.CommitStagedBatchDispatch(targets)
	return m
}

func (m Model) handleEvent(e session.Event) (Model, tea.Cmd) {
	var next Model
	if out, ok := m.handleAgentLifecycleEvent(e); ok {
		next = out
	} else {
		next = m.handleMessageEvent(e)
	}
	next = next.maybeAdvanceStagedBatch(e)
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
	case session.KindParticipantStatusChanged:
		// No transcript entry for status; used to drive staged dispatch.
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
	case session.KindApprovalRequested:
		m = m.handleApprovalRequested(e)
	case session.KindApprovalCleared:
		m = m.handleApprovalCleared(e)
	case session.KindSharedNotice:
		m.room = m.room.AppendSystem("[notice → " + e.Alias + "]")
	case session.KindAgentLog:
		m.room = m.room.AppendLog(e.Alias, e.Text)
	case session.KindAgentMessage:
		if e.Msg != nil {
			m.room = m.room.HandleAgentMessage(e.Alias, *e.Msg)
		}
	default:
	}
	return m
}

func (m Model) handleApprovalRequested(e session.Event) Model {
	if e.ApprovalReq == nil {
		return m
	}
	m.activeApprovalID = e.ApprovalID
	req := *e.ApprovalReq
	if strings.TrimSpace(e.Alias) != "" {
		req.Ask = "[→ " + e.Alias + "] " + req.Ask
	}
	m.room = m.room.ShowApproval(req)
	return m
}

func (m Model) handleApprovalCleared(e session.Event) Model {
	if e.ApprovalID == 0 || e.ApprovalID != m.activeApprovalID {
		return m
	}
	m.activeApprovalID = 0
	m.room, _ = m.room.ClearApproval()
	return m
}

func (m Model) handleBarrierBatchSubmit(raw string, action Action) Model {
	ps := m.sess.RoutableParticipants()
	if len(ps) == 0 {
		m.room = m.room.AppendSystem("[no agents — use /invite <alias> to start one]")
		m.room = m.room.SetComposeValue("")
		return m
	}
	barrier := make([]string, 0, len(ps))
	for _, p := range ps {
		barrier = append(barrier, p.Alias)
	}
	b := staging.NewBatch(raw, toStagedAction(action), barrier)
	nextRoom, shouldDispatch := m.room.StageBatchOrDispatch(b, m.stagedSnapshotStatus)
	m.room = nextRoom
	if shouldDispatch {
		return m.dispatchRoomStagedBatch()
	}
	return m
}

func (m Model) handleStagedInterrupt() Model {
	if !m.room.HasStagedBatch() {
		return m
	}
	nextRoom, blocked, shouldDispatch := m.room.RequestStagedInterrupt(m.stagedSnapshotStatus)
	m.room = nextRoom
	for _, alias := range blocked {
		if err := m.sess.Execute(session.CancelCommand{Alias: alias}); err != nil {
			m.room = m.room.AppendSystem(fmt.Sprintf("error: cancel %q: %v", alias, err))
			continue
		}
		m.room = m.room.AppendSystem("[→ " + alias + "] interrupt requested")
	}
	if shouldDispatch {
		return m.dispatchRoomStagedBatch()
	}
	return m
}

func (m Model) maybeAdvanceStagedBatch(e session.Event) Model {
	if !m.room.HasStagedBatch() {
		return m
	}
	switch e.Kind {
	case session.KindAgentStopped, session.KindAgentCrashed:
		m.room = m.room.MarkStagedDiscarded(e.Alias)
	case session.KindParticipantStatusChanged, session.KindAgentStarted:
		// Status changes that may unblock dispatch.
	default:
		return m
	}

	nextRoom, shouldDispatch := m.room.RefreshStagedStatus(m.stagedSnapshotStatus)
	m.room = nextRoom
	if shouldDispatch {
		return m.dispatchRoomStagedBatch()
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

func (m Model) executeSendToAgent(alias, text string) (Model, []string, error) {
	err := m.sess.Execute(session.SharedSendCommand{
		Alias:         alias,
		TextDirect:    text,
		TextListeners: fmt.Sprintf("@%s: %s", alias, text),
	})
	if err != nil {
		m.room = m.room.AppendSystem(fmt.Sprintf("error: send to %q: %v", alias, err))
		return m, session.DeliveredAliases(err), fmt.Errorf("send to %q: %w", alias, err)
	}
	return m, []string{alias}, nil
}

func (m Model) sendToAgent(alias, text string) Model {
	m, _, _ = m.executeSendToAgent(alias, text)
	return m
}

func (m Model) executeBroadcastAll(text string) (Model, []string, error) {
	if len(m.sess.RoutableParticipants()) == 0 {
		m.room = m.room.AppendSystem("[no agents — use /invite <alias> to start one]")
		return m, nil, fmt.Errorf("no routable agents")
	}
	if err := m.sess.Execute(session.BroadcastCommand{Text: text}); err != nil {
		m.room = m.room.AppendSystem(fmt.Sprintf("error: broadcast: %v", err))
		return m, session.DeliveredAliases(err), fmt.Errorf("broadcast: %w", err)
	}
	return m, routingFor(Broadcast{Text: text}, m.sess.RoutableParticipants()), nil
}

func (m Model) broadcastAll(text string) Model {
	m, _, _ = m.executeBroadcastAll(text)
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
	const tmpl = `[help]

Commands:
  /invite <alias>      start an agent
  /remove <alias>      remove an agent
  /cancel <alias>      interrupt an agent's current turn
  /who                 list agents
%s  /help                show this message
  /quit                exit

Sending messages:
  @<alias> <text>      send to one agent
  <text>               broadcast to all agents

General keys:
  Ctrl+O               toggle focus (compose ⇄ history)
  PgUp / PgDn          scroll transcript (works in any focus)

Compose focus (separator label: compose):
  Enter                submit
  Ctrl+G               open $EDITOR for multi-line compose
  Ctrl+X               (when staged) interrupt + send
  Esc                  (when staged) edit staged message

History focus (separator label: history):
  ↑ / ↓                scroll 1 line
  Home / End           jump to top / jump to bottom
  Esc                  return to compose focus
  Ctrl+G               open transcript in $EDITOR (read-only)

Approval prompt (separator label: approval):
  ↑/↓ or j/k           change selection
  Enter                confirm selection
  Esc                  dismiss prompt
  Ctrl+C               cancel prompt

UI hints:
  The separator label shows the current focus: compose/history/approval
  When history is focused, the first visible history row is highlighted`

	debugBlock := ""
	if m.debug {
		debugBlock = "" +
			"  /debugview           print viewport debug\n" +
			"  /debugrows           toggle row number overlay\n"
	}
	m.room = m.room.AppendSystem(fmt.Sprintf(tmpl, debugBlock))
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
