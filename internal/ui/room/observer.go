package room

import (
	tea "charm.land/bubbletea/v2"
	"github.com/trigosec/coderoom/internal/queue"
	roomstate "github.com/trigosec/coderoom/internal/room"
)

// UpdateMsg reports that a canonical room update is ready to apply.
type UpdateMsg struct{ update roomstate.Update }

// Trigger returns the applied room input that triggered the update.
func (m UpdateMsg) Trigger() roomstate.UpdateTrigger { return m.update.Trigger }

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
		return UpdateMsg{update: update}
	}
}

func (m Model) applyRoomUpdate(update roomstate.Update) Model {
	if update.RoomID != roomstate.SharedRoomID {
		return m
	}
	if update.Version <= m.roomVersion {
		return m
	}
	return m.applyChatDelta()
}
