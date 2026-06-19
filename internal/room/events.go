package room

import (
	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/session"
)

func (r *Room) applyEvent(e session.Event) (Update, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var dirty []int
	switch e.Kind {
	case session.KindAgentStarting:
		dirty = append(dirty, len(r.records))
		r.records = append(r.records, Record{Kind: KindSystem, Text: "[" + e.Alias + " starting]"})
	case session.KindAgentStarted:
		r.members[e.Alias] = struct{}{}
		delete(r.departed, e.Alias)
		dirty = append(dirty, len(r.records))
		r.records = append(r.records, Record{Kind: KindSystem, Text: "[" + e.Alias + " joined]"})
	case session.KindAgentStopped:
		delete(r.members, e.Alias)
		r.clearStreamsLocked(e.Alias)
		r.departed[e.Alias] = true
		dirty = append(dirty, len(r.records))
		r.records = append(r.records, Record{Kind: KindSystem, Text: "[" + e.Alias + " left]"})
	case session.KindAgentCrashed:
		delete(r.members, e.Alias)
		r.clearStreamsLocked(e.Alias)
		r.departed[e.Alias] = true
		dirty = append(dirty, len(r.records))
		r.records = append(r.records, Record{Kind: KindSystem, Text: "[" + e.Alias + " crashed]"})
	case session.KindAgentLog:
		dirty = append(dirty, len(r.records))
		r.records = append(r.records, Record{Kind: KindLog, Alias: e.Alias, Text: e.Text})
	case session.KindAgentMessage:
		if e.Msg == nil {
			return Update{}, false
		}
		dirty = append(dirty, r.handleAgentMessageLocked(e.Alias, *e.Msg)...)
	default:
		return Update{}, false
	}

	return r.bumpVersionLocked(dirty...), true
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
