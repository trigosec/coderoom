package ui

import (
	"github.com/trigosec/coderoom/internal/promptlang"
	"github.com/trigosec/coderoom/internal/ui/room/staging"
)

func toStagedAction(a promptlang.Statement) staging.Action {
	switch act := a.(type) {
	case promptlang.Broadcast:
		return staging.Action{Kind: staging.ActionBroadcast, Text: act.Text}
	case promptlang.Send:
		return staging.Action{Kind: staging.ActionSend, Alias: act.Alias, Text: act.Text}
	case promptlang.Handoff:
		return staging.Action{Kind: staging.ActionHandoff, FromAlias: act.FromAlias, ToAlias: act.ToAlias}
	default:
		return staging.Action{Kind: staging.ActionUnknown}
	}
}
