package ui

import "github.com/trigosec/coderoom/internal/ui/room/staging"

func toStagedAction(a Action) staging.Action {
	switch act := a.(type) {
	case Broadcast:
		return staging.Action{Kind: staging.ActionBroadcast, Text: act.Text}
	case Send:
		return staging.Action{Kind: staging.ActionSend, Alias: act.Alias, Text: act.Text}
	case Handoff:
		return staging.Action{Kind: staging.ActionHandoff, FromAlias: act.FromAlias, ToAlias: act.ToAlias}
	default:
		return staging.Action{Kind: staging.ActionUnknown}
	}
}
