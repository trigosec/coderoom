package room

import "slices"

func (r *Room) bumpVersionLocked() Update {
	r.version++
	return Update{RoomID: r.id, Version: r.version}
}

func (r *Room) notify(update Update) {
	if r.observer != nil {
		r.observer.OnRoomUpdate(update)
	}
}

func cloneRecord(record Record) Record {
	cloned := record
	cloned.Routing = slices.Clone(record.Routing)
	if record.Msg != nil {
		msgCopy := *record.Msg
		cloned.Msg = &msgCopy
	}
	return cloned
}
