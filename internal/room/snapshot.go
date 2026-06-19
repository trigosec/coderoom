package room

import (
	"slices"

	"github.com/trigosec/coderoom/internal/agent"
)

// Snapshot returns a detached, point-in-time copy of the room's current
// state, safe for the caller to read without further synchronization.
func (r *Room) Snapshot() Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	members := make([]string, 0, len(r.members))
	for alias := range r.members {
		members = append(members, alias)
	}
	slices.Sort(members)

	departed := make(map[string]bool, len(r.departed))
	for alias, isDeparted := range r.departed {
		departed[alias] = isDeparted
	}

	records := make([]Record, len(r.records))
	for i, record := range r.records {
		records[i] = cloneRecord(record)
	}

	streamIDs := make([]agent.StreamID, 0, len(r.streaming))
	for streamID := range r.streaming {
		streamIDs = append(streamIDs, streamID)
	}
	slices.Sort(streamIDs)

	openStreams := make([]OpenStream, 0, len(streamIDs))
	for _, streamID := range streamIDs {
		slot := r.streaming[streamID]
		openStreams = append(openStreams, OpenStream{
			Alias:     slot.alias,
			RecordIdx: slot.recordIdx,
			StreamID:  streamID,
			Kind:      slot.kind,
		})
	}

	return Snapshot{
		RoomID:      r.id,
		Version:     r.version,
		Members:     members,
		Departed:    departed,
		Records:     records,
		OpenStreams: openStreams,
	}
}
