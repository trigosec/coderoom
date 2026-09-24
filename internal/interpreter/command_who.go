package interpreter

import "github.com/trigosec/coderoom/internal/participant"

func (i *Interpreter) executeWho(raw string) {
	i.room.AppendUserInputRecord(raw, nil)
	i.publish(InputAccepted{Raw: raw})
	snapshot := i.captureSnapshot()
	i.publish(StateChanged{Snapshot: snapshot})
	i.publish(RosterListed{Participants: append([]participant.View(nil), snapshot.Participants...)})
	i.publish(SubmissionSucceeded{Raw: raw})
}
