package room

import (
	tea "charm.land/bubbletea/v2"
	"github.com/trigosec/coderoom/internal/queue"
	roomstate "github.com/trigosec/coderoom/internal/room"
)

type roomUpdateMsg roomstate.Update

type roomUpdateObserver struct {
	queue *queue.Queue[roomstate.Update]
}

func (o roomUpdateObserver) OnRoomUpdate(update roomstate.Update) {
	o.queue.Push(update)
}

func awaitRoomUpdate(q *queue.Queue[roomstate.Update]) tea.Cmd {
	return func() tea.Msg {
		update, ok := q.Pull()
		if !ok {
			return nil
		}
		return roomUpdateMsg(update)
	}
}

func (m Model) applyRoomUpdate(update roomstate.Update) Model {
	if update.RoomID != roomstate.SharedRoomID {
		return m
	}
	return m.SetHistorySnapshot(m.chat.Snapshot())
}
