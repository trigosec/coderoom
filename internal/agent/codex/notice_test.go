package codex

import (
	"bytes"
	"io"
	"testing"

	"github.com/trigosec/coderoom/internal/agent"
)

// setupNoticeClient creates a client in noticePending state, simulating
// that SendNotice has been called and the turn is in flight.
func setupNoticeClient(t *testing.T, stdout string) *Client {
	t.Helper()
	c := newWithIO(t, nopWriteCloser{io.Discard}, bytes.NewBufferString(stdout), nil)
	c.turn.mu.Lock()
	c.turn.threadID = "t1"
	c.turn.state = turnState{kind: turnInflightUnknownID}
	c.turn.mu.Unlock()
	c.notice.mu.Lock()
	c.notice.state = noticePending
	c.notice.mu.Unlock()
	return c
}

const turnStarted = `{"method":"turn/started","params":{"threadId":"t1","turn":{"id":"u1"}}}` + "\n"
const turnCompleted = `{"method":"turn/completed","params":{}}` + "\n"
const turnFailed = `{"method":"turn/failed","params":{}}` + "\n"

func agentDelta(text string) string {
	return `{"method":"item/agentMessage/delta","params":{"delta":"` + text + `"}}` + "\n"
}

// TestNoticeFilter_compliantAck verifies that a response of {"acknowledge":true}
// is silently discarded, but still produces a turn-level Output+ModeFlush so
// downstream consumers can treat SendNotice as a complete turn lifecycle.
func TestNoticeFilter_compliantAck(t *testing.T) {
	stdout := turnStarted +
		agentDelta(`{\"acknowledge\":true}`) +
		turnCompleted
	c := setupNoticeClient(t, stdout)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Mode != agent.ModeFlush {
		t.Fatalf("expected ModeFlush, got %v", msg.Mode)
	}
	if _, ok := msg.Content.(agent.Output); !ok {
		t.Fatalf("expected Output content, got %T", msg.Content)
	}
}

// TestNoticeFilter_compliantAckWithExtraFields verifies that extra JSON fields
// alongside "acknowledge":true are intentionally ignored — still discarded, but
// still produces a turn-level flush.
func TestNoticeFilter_compliantAckWithExtraFields(t *testing.T) {
	stdout := turnStarted +
		agentDelta(`{\"acknowledge\":true,\"notes\":\"logged\"}`) +
		turnCompleted
	c := setupNoticeClient(t, stdout)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Mode != agent.ModeFlush {
		t.Fatalf("expected ModeFlush, got %v", msg.Mode)
	}
	if _, ok := msg.Content.(agent.Output); !ok {
		t.Fatalf("expected Output content, got %T", msg.Content)
	}
}

// TestNoticeFilter_emptyResponse treats a response with no deltas as an ack and
// still emits a turn-level flush.
func TestNoticeFilter_emptyResponse(t *testing.T) {
	stdout := turnStarted + turnCompleted
	c := setupNoticeClient(t, stdout)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Mode != agent.ModeFlush {
		t.Fatalf("expected ModeFlush, got %v", msg.Mode)
	}
	if _, ok := msg.Content.(agent.Output); !ok {
		t.Fatalf("expected Output content, got %T", msg.Content)
	}
}

// TestNoticeFilter_nonCompliantProse verifies that a prose response (first char
// is not '{') is relayed as Reasoning then a reasoning-stream flush then a
// turn-level flush.
func TestNoticeFilter_nonCompliantProse(t *testing.T) {
	stdout := turnStarted +
		agentDelta(`I think the code looks fine.`) +
		turnCompleted
	c := setupNoticeClient(t, stdout)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, ok := msg.Content.(agent.Reasoning)
	if !ok || r.Text != "I think the code looks fine." {
		t.Errorf("expected Reasoning relay, got mode=%v content=%T", msg.Mode, msg.Content)
	}

	// reasoning-stream flush
	msg, err = c.Read()
	if err != nil {
		t.Fatalf("unexpected error on reasoning flush: %v", err)
	}
	if msg.Mode != agent.ModeFlush {
		t.Errorf("expected ModeFlush for reasoning stream end, got mode=%v", msg.Mode)
	}

	// turn-level flush
	msg, err = c.Read()
	if err != nil {
		t.Fatalf("unexpected error on turn flush: %v", err)
	}
	if msg.Mode != agent.ModeFlush {
		t.Errorf("expected ModeFlush for turn end, got mode=%v", msg.Mode)
	}
}

// TestNoticeFilter_nonCompliantJSON verifies that a JSON response that parses
// but lacks "acknowledge":true is replayed as Reasoning then reasoning-stream
// flush then turn-level flush.
func TestNoticeFilter_nonCompliantJSON(t *testing.T) {
	stdout := turnStarted +
		agentDelta(`{\"status\":\"ok\"}`) +
		turnCompleted
	c := setupNoticeClient(t, stdout)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := msg.Content.(agent.Reasoning); !ok {
		t.Errorf("expected Reasoning for non-ack JSON, got mode=%v content=%T", msg.Mode, msg.Content)
	}

	// reasoning-stream flush
	msg, err = c.Read()
	if err != nil {
		t.Fatalf("unexpected error on reasoning flush: %v", err)
	}
	if msg.Mode != agent.ModeFlush {
		t.Errorf("expected ModeFlush for reasoning stream end, got mode=%v", msg.Mode)
	}

	// turn-level flush
	msg, err = c.Read()
	if err != nil {
		t.Fatalf("unexpected error on turn flush: %v", err)
	}
	if msg.Mode != agent.ModeFlush {
		t.Errorf("expected ModeFlush for turn end, got mode=%v", msg.Mode)
	}
}

// TestNoticeFilter_nonCompliantMultiDeltaRelay verifies that multiple deltas
// are all relayed as reasoning when the first char is not '{'.
//
//nolint:cyclop
func TestNoticeFilter_nonCompliantMultiDeltaRelay(t *testing.T) {
	stdout := turnStarted +
		agentDelta(`step one`) +
		agentDelta(` step two`) +
		turnCompleted
	c := setupNoticeClient(t, stdout)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, ok := msg.Content.(agent.Reasoning)
	if !ok || r.Text != "step one" {
		t.Errorf("expected first Reasoning delta, got mode=%v content=%T", msg.Mode, msg.Content)
	}

	msg, err = c.Read()
	if err != nil {
		t.Fatalf("unexpected error on second delta: %v", err)
	}
	r, ok = msg.Content.(agent.Reasoning)
	if !ok || r.Text != " step two" {
		t.Errorf("expected second Reasoning delta, got mode=%v content=%T", msg.Mode, msg.Content)
	}

	// reasoning-stream flush
	msg, err = c.Read()
	if err != nil {
		t.Fatalf("unexpected error on reasoning flush: %v", err)
	}
	if msg.Mode != agent.ModeFlush {
		t.Errorf("expected ModeFlush for reasoning stream end, got mode=%v", msg.Mode)
	}

	// turn-level flush
	msg, err = c.Read()
	if err != nil {
		t.Fatalf("unexpected error on turn flush: %v", err)
	}
	if msg.Mode != agent.ModeFlush {
		t.Errorf("expected ModeFlush for turn end, got mode=%v", msg.Mode)
	}
}

// TestNoticeFilter_turnFailed_buffering verifies that a failed notice turn
// while buffering is silently discarded, but still emits a turn-level flush so
// downstream consumers can treat SendNotice as a complete lifecycle.
func TestNoticeFilter_turnFailed_buffering(t *testing.T) {
	stdout := turnStarted +
		agentDelta(`{\"partial`) +
		turnFailed
	c := setupNoticeClient(t, stdout)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Mode != agent.ModeFlush {
		t.Fatalf("expected ModeFlush, got %v", msg.Mode)
	}
	if _, ok := msg.Content.(agent.Output); !ok {
		t.Fatalf("expected Output content, got %T", msg.Content)
	}
}

// TestNoticeFilter_turnFailed_relaying verifies that a failed notice turn while
// relaying emits a reasoning-stream flush then a turn-level flush.
func TestNoticeFilter_turnFailed_relaying(t *testing.T) {
	stdout := turnStarted +
		agentDelta(`some prose`) +
		turnFailed
	c := setupNoticeClient(t, stdout)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := msg.Content.(agent.Reasoning); !ok {
		t.Errorf("expected Reasoning before failed turn, got mode=%v content=%T", msg.Mode, msg.Content)
	}

	// reasoning-stream flush
	msg, err = c.Read()
	if err != nil {
		t.Fatalf("unexpected error on reasoning flush: %v", err)
	}
	if msg.Mode != agent.ModeFlush {
		t.Errorf("expected ModeFlush for reasoning stream end, got mode=%v", msg.Mode)
	}

	// turn-level flush
	msg, err = c.Read()
	if err != nil {
		t.Fatalf("unexpected error on turn flush: %v", err)
	}
	if msg.Mode != agent.ModeFlush {
		t.Errorf("expected ModeFlush after failed relaying notice, got mode=%v", msg.Mode)
	}
}

// TestNoticeFilter_noNoticeState verifies that normal (non-notice) turns are
// unaffected when no notice is active.
func TestNoticeFilter_noNoticeState(t *testing.T) {
	stdout := `{"method":"item/agentMessage/delta","params":{"itemId":"msg1","turnId":"turn1","delta":"hello"}}` + "\n" +
		`{"method":"turn/completed","params":{"threadId":"th1","turn":{"id":"turn1","status":"completed","items":[{"type":"agentMessage","id":"msg1"}]}}}` + "\n"
	c := newWithIO(t, nopWriteCloser{io.Discard}, bytes.NewBufferString(stdout), nil)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, ok := msg.Content.(agent.Output)
	if !ok || out.Text != "hello" {
		t.Errorf("expected Output{hello}, got mode=%v content=%T", msg.Mode, msg.Content)
	}
	msg, err = c.Read()
	if err != nil {
		t.Fatalf("unexpected error on turn flush: %v", err)
	}
	if msg.Mode != agent.ModeFlush {
		t.Errorf("expected ModeFlush for turn/completed, got mode=%v", msg.Mode)
	}
}
