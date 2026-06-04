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

func TestObserver_NormalizesThreadStartMatch(t *testing.T) {
	obs := NewObserver()

	obs.OnSend(`{"id":2,"method":"thread/start","params":{"cwd":"/tmp/work","model":"gpt-5.4"}}`)

	steps := obs.Steps()
	if len(steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(steps))
	}
	match, ok := steps[0].Match.(map[string]any)
	if !ok {
		t.Fatalf("match = %T, want map[string]any", steps[0].Match)
	}
	params, ok := match["params"].(map[string]any)
	if !ok {
		t.Fatalf("params = %T, want map[string]any", match["params"])
	}
	if _, ok := params["cwd"]; ok {
		t.Fatalf("normalized params unexpectedly kept cwd: %#v", params)
	}
	if params["model"] != "gpt-5.4" {
		t.Fatalf("params[model] = %v, want gpt-5.4", params["model"])
	}
}

func TestObserver_NormalizesInitializeMatch(t *testing.T) {
	obs := NewObserver()

	obs.OnSend(`{"id":1,"method":"initialize","params":{"clientInfo":{"name":"coderoom","version":"0.1.0"},"capabilities":{"experimentalApi":true}}}`)

	steps := obs.Steps()
	if len(steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(steps))
	}
	match, ok := steps[0].Match.(map[string]any)
	if !ok {
		t.Fatalf("match = %T, want map[string]any", steps[0].Match)
	}
	params, ok := match["params"].(map[string]any)
	if !ok {
		t.Fatalf("params = %T, want map[string]any", match["params"])
	}
	clientInfo, ok := params["clientInfo"].(map[string]any)
	if !ok {
		t.Fatalf("clientInfo = %T, want map[string]any", params["clientInfo"])
	}
	if _, ok := clientInfo["version"]; ok {
		t.Fatalf("normalized clientInfo unexpectedly kept version: %#v", clientInfo)
	}
	if clientInfo["name"] != "coderoom" {
		t.Fatalf("clientInfo[name] = %v, want coderoom", clientInfo["name"])
	}
}
