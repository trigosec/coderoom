package room

import (
	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/session"
)

func (r *Room) applyEvent(e session.Event) (Update, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch e.Kind {
	case session.KindAgentStarting:
		r.records = append(r.records, Record{Kind: KindSystem, Text: "[" + e.Alias + " starting]"})
	case session.KindAgentStarted:
		r.members[e.Alias] = struct{}{}
		delete(r.departed, e.Alias)
		r.records = append(r.records, Record{Kind: KindSystem, Text: "[" + e.Alias + " joined]"})
	case session.KindAgentStopped:
		delete(r.members, e.Alias)
		r.clearStreamsLocked(e.Alias)
		r.departed[e.Alias] = true
		r.records = append(r.records, Record{Kind: KindSystem, Text: "[" + e.Alias + " left]"})
	case session.KindAgentCrashed:
		delete(r.members, e.Alias)
		r.clearStreamsLocked(e.Alias)
		r.departed[e.Alias] = true
		r.records = append(r.records, Record{Kind: KindSystem, Text: "[" + e.Alias + " crashed]"})
	case session.KindAgentLog:
		r.records = append(r.records, Record{Kind: KindLog, Alias: e.Alias, Text: e.Text})
	case session.KindAgentMessage:
		if e.Msg == nil {
			return Update{}, false
		}
		r.handleAgentMessageLocked(e.Alias, *e.Msg)
	default:
		return Update{}, false
	}

	return r.bumpVersionLocked(), true
}

func (r *Room) handleAgentMessageLocked(alias string, msg agent.Message) {
	switch msg.Content.(type) {
	case agent.Output, agent.Reasoning, agent.Command, agent.FileChangeSet:
		if msg.Mode == agent.ModeFlush {
			r.sealStreamLocked(msg.StreamID)
			return
		}
	}

	if slot, ok := r.streaming[msg.StreamID]; ok {
		updated, err := r.records[slot.recordIdx].Accumulate(msg)
		if err == nil {
			r.records[slot.recordIdx] = updated
			return
		}
	}

	idx := len(r.records)
	r.records = append(r.records, NewAgentRecord(alias, msg))
	r.streaming[msg.StreamID] = streamSlot{
		recordIdx: idx,
		alias:     alias,
		kind:      openStreamKind(msg),
	}
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
		if slot.alias == alias {
			delete(r.streaming, streamID)
		}
	}
}

func (r *Room) sealStreamLocked(streamID agent.StreamID) {
	delete(r.streaming, streamID)
}
