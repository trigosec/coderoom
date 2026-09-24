package interpreter

func (i *Interpreter) executeQuit(raw string) {
	i.room.AppendUserInputRecord(raw, nil)
	i.publish(InputAccepted{Raw: raw})
	i.publish(StateChanged{Snapshot: i.captureSnapshot()})
	i.publish(SubmissionSucceeded{Raw: raw})
	i.requestClose()
	i.shutdownSession()
	i.drainSessionEvents(false)
	i.publish(ExitRequested{})
}
