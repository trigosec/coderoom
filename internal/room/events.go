package room

import (
	"fmt"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/session"
)

func (r *Room) applyEvent(e session.Event) (Update, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if e.Kind == session.KindAgentMessage {
		if e.Msg == nil {
			return Update{}, false
		}
		dirty := r.handleAgentMessageLocked(e.Alias, *e.Msg)
		return r.bumpVersionLocked(dirty...), true
	}

	dirty, ok := r.handleLifecycleEventLocked(e)
	if !ok {
		return Update{}, false
	}
	return r.bumpVersionLocked(dirty...), true
}

func (r *Room) handleLifecycleEventLocked(e session.Event) ([]int, bool) {
	switch e.Kind {
	case session.KindAgentStarting:
		return r.appendSystemRecordLocked("[" + e.Alias + " starting]"), true
	case session.KindAgentStarted:
		r.members[e.Alias] = struct{}{}
		delete(r.departed, e.Alias)
		return r.appendSystemRecordLocked("[" + e.Alias + " joined]"), true
	case session.KindAgentStopped:
		r.handleDepartureLocked(e.Alias)
		return r.appendSystemRecordLocked("[" + e.Alias + " left]"), true
	case session.KindAgentCrashed:
		r.handleDepartureLocked(e.Alias)
		return r.appendSystemRecordLocked("[" + e.Alias + " crashed]"), true
	case session.KindAgentLog:
		return r.appendRecordLocked(Record{Kind: KindLog, Alias: e.Alias, Text: e.Text}), true
	case session.KindContextHandoff:
		return r.appendSystemRecordLocked(handoffPreview(e)), true
	default:
		return nil, false
	}
}

func (r *Room) handleDepartureLocked(alias string) {
	delete(r.members, alias)
	r.clearStreamsLocked(alias)
	r.departed[alias] = true
}

func (r *Room) appendSystemRecordLocked(text string) []int {
	return r.appendRecordLocked(Record{Kind: KindSystem, Text: text})
}

func (r *Room) appendRecordLocked(record Record) []int {
	idx := len(r.records)
	r.records = append(r.records, record)
	return []int{idx}
}

func handoffPreview(e session.Event) string {
	if e.Preview != "" {
		return e.Preview
	}
	return fmt.Sprintf("[handoff %s -> %s]\n\n%s", e.FromAlias, e.ToAlias, e.Text)
}

func (r *Room) handleAgentMessageLocked(alias string, msg agent.Message) []int {
	switch msg.Content.(type) {
	case agent.Output, agent.Reasoning, agent.Command, agent.FileChangeSet:
		if msg.Mode == agent.ModeFlush {
			r.sealStreamLocked(msg.StreamID)
			return nil
		}
	}

	if slot, ok := r.streaming[msg.StreamID]; ok {
		updated, err := r.records[slot.RecordIdx].Accumulate(msg)
		if err == nil {
			r.records[slot.RecordIdx] = updated
			return []int{slot.RecordIdx}
		}
	}

	idx := len(r.records)
	r.records = append(r.records, NewAgentRecord(alias, msg))
	r.streaming[msg.StreamID] = OpenStream{
		Alias:     alias,
		RecordIdx: idx,
		StreamID:  msg.StreamID,
		Kind:      openStreamKind(msg),
	}
	return []int{idx}
}

func openStreamKind(msg agent.Message) OpenStreamKind {
	switch msg.Content.(type) {
	case agent.Reasoning:
		return OpenStreamReasoning
	case agent.Command:
		return OpenStreamCommand
	case agent.FileChangeSet:
		return OpenStreamFileChange
	default:
		return OpenStreamOutput
	}
}

func (r *Room) clearStreamsLocked(alias string) {
	for streamID, slot := range r.streaming {
		if slot.Alias == alias {
			delete(r.streaming, streamID)
		}
	}
}

func (r *Room) sealStreamLocked(streamID agent.StreamID) {
	delete(r.streaming, streamID)
}
