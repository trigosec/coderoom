package ui

import "github.com/trigosec/coderoom/internal/ui/room/staging"

func toStagedAction(a Action) staging.Action {
	switch act := a.(type) {
	case Broadcast:
		return staging.Action{Kind: staging.ActionBroadcast, Text: act.Text}
	case Send:
		return staging.Action{Kind: staging.ActionSend, Alias: act.Alias, Text: act.Text}
	default:
		return staging.Action{Kind: staging.ActionUnknown}
	}
}
