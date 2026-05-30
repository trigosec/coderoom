package ui

import "github.com/trigosec/coderoom/internal/participant"

func (m Model) stagedSnapshotStatus(alias string) (participant.Status, bool) {
	p, ok := m.sess.Participant(alias)
	if !ok {
		return "", false
	}
	return p.Status, true
}
