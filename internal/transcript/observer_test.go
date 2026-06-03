package transcript

import "testing"

func TestObserver_RecordsDirectionalSteps(t *testing.T) {
	obs := NewObserver()

	obs.OnSend(`{"id":1,"method":"initialize","params":{"clientInfo":{"name":"coderoom"}}}`)
	obs.OnReceive(`{"id":1,"result":{"capabilities":{"experimentalApi":true}}}`)

	steps := obs.Steps()
	if len(steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(steps))
	}
	if steps[0].Kind != "recv" || steps[0].Match == nil {
		t.Fatalf("first step = %#v, want recv with match", steps[0])
	}
	if steps[1].Kind != "send" || steps[1].Message == nil {
		t.Fatalf("second step = %#v, want send with message", steps[1])
	}
}
