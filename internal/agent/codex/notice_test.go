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
// is silently discarded — no messages arrive before the EOF error.
func TestNoticeFilter_compliantAck(t *testing.T) {
	stdout := turnStarted +
		agentDelta(`{\"acknowledge\":true}`) +
		turnCompleted
	c := setupNoticeClient(t, stdout)

	msg, err := c.Read()
	if err == nil {
		t.Fatalf("expected no message to be emitted for acknowledged notice; got kind=%q text=%q", msg.Kind, msg.Text)
	}
}

// TestNoticeFilter_compliantAckWithExtraFields verifies that extra JSON fields
// alongside "acknowledge":true are intentionally ignored — still discarded.
func TestNoticeFilter_compliantAckWithExtraFields(t *testing.T) {
	stdout := turnStarted +
		agentDelta(`{\"acknowledge\":true,\"notes\":\"logged\"}`) +
		turnCompleted
	c := setupNoticeClient(t, stdout)

	msg, err := c.Read()
	if err == nil {
		t.Fatalf("expected no message for ack with extra fields; got kind=%q text=%q", msg.Kind, msg.Text)
	}
}

// TestNoticeFilter_emptyResponse treats a response with no deltas as an ack.
func TestNoticeFilter_emptyResponse(t *testing.T) {
	stdout := turnStarted + turnCompleted
	c := setupNoticeClient(t, stdout)

	msg, err := c.Read()
	if err == nil {
		t.Fatalf("expected no message for empty notice response; got kind=%q text=%q", msg.Kind, msg.Text)
	}
}

// TestNoticeFilter_nonCompliantProse verifies that a prose response (first char
// is not '{') is relayed as MessageReasoning followed by MessageDone.
func TestNoticeFilter_nonCompliantProse(t *testing.T) {
	stdout := turnStarted +
		agentDelta(`I think the code looks fine.`) +
		turnCompleted
	c := setupNoticeClient(t, stdout)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Kind != agent.MessageReasoning || msg.Text != "I think the code looks fine." {
		t.Errorf("expected reasoning relay, got kind=%q text=%q", msg.Kind, msg.Text)
	}

	msg, err = c.Read()
	if err != nil {
		t.Fatalf("unexpected error on done: %v", err)
	}
	if msg.Kind != agent.MessageDone {
		t.Errorf("expected MessageDone after relayed reasoning, got kind=%q", msg.Kind)
	}
}

// TestNoticeFilter_nonCompliantJSON verifies that a JSON response that parses
// but lacks "acknowledge":true is replayed as one MessageReasoning + MessageDone.
func TestNoticeFilter_nonCompliantJSON(t *testing.T) {
	stdout := turnStarted +
		agentDelta(`{\"status\":\"ok\"}`) +
		turnCompleted
	c := setupNoticeClient(t, stdout)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Kind != agent.MessageReasoning {
		t.Errorf("expected MessageReasoning for non-ack JSON, got kind=%q", msg.Kind)
	}

	msg, err = c.Read()
	if err != nil {
		t.Fatalf("unexpected error on done: %v", err)
	}
	if msg.Kind != agent.MessageDone {
		t.Errorf("expected MessageDone, got kind=%q", msg.Kind)
	}
}

// TestNoticeFilter_nonCompliantMultiDeltaRelay verifies that multiple deltas
// are all relayed as reasoning when the first char is not '{'.
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
	if msg.Kind != agent.MessageReasoning || msg.Text != "step one" {
		t.Errorf("expected first reasoning delta, got kind=%q text=%q", msg.Kind, msg.Text)
	}

	msg, err = c.Read()
	if err != nil {
		t.Fatalf("unexpected error on second delta: %v", err)
	}
	if msg.Kind != agent.MessageReasoning || msg.Text != " step two" {
		t.Errorf("expected second reasoning delta, got kind=%q text=%q", msg.Kind, msg.Text)
	}

	msg, err = c.Read()
	if err != nil {
		t.Fatalf("unexpected error on done: %v", err)
	}
	if msg.Kind != agent.MessageDone {
		t.Errorf("expected MessageDone, got kind=%q", msg.Kind)
	}
}

// TestNoticeFilter_turnFailed_buffering verifies that a failed notice turn
// while buffering is silently discarded (no messages emitted).
func TestNoticeFilter_turnFailed_buffering(t *testing.T) {
	stdout := turnStarted +
		agentDelta(`{\"partial`) +
		turnFailed
	c := setupNoticeClient(t, stdout)

	msg, err := c.Read()
	if err == nil {
		t.Fatalf("expected no message for failed buffered notice; got kind=%q text=%q", msg.Kind, msg.Text)
	}
}

// TestNoticeFilter_turnFailed_relaying verifies that a failed notice turn
// while relaying emits MessageDone to return the participant to idle.
func TestNoticeFilter_turnFailed_relaying(t *testing.T) {
	stdout := turnStarted +
		agentDelta(`some prose`) +
		turnFailed
	c := setupNoticeClient(t, stdout)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Kind != agent.MessageReasoning {
		t.Errorf("expected MessageReasoning before failed turn, got kind=%q", msg.Kind)
	}

	msg, err = c.Read()
	if err != nil {
		t.Fatalf("unexpected error on done: %v", err)
	}
	if msg.Kind != agent.MessageDone {
		t.Errorf("expected MessageDone after failed relaying notice, got kind=%q", msg.Kind)
	}
}

// TestNoticeFilter_noNoticeState verifies that normal (non-notice) turns are
// unaffected when no notice is active.
func TestNoticeFilter_noNoticeState(t *testing.T) {
	stdout := `{"method":"item/agentMessage/delta","params":{"delta":"hello"}}` + "\n" +
		`{"method":"turn/completed","params":{}}` + "\n"
	c := newWithIO(t, nopWriteCloser{io.Discard}, bytes.NewBufferString(stdout), nil)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Kind != agent.MessageDelta || msg.Text != "hello" {
		t.Errorf("expected normal delta, got kind=%q text=%q", msg.Kind, msg.Text)
	}
	msg, err = c.Read()
	if err != nil {
		t.Fatalf("unexpected error on done: %v", err)
	}
	if msg.Kind != agent.MessageDone {
		t.Errorf("expected MessageDone, got kind=%q", msg.Kind)
	}
}
