package room

import "github.com/trigosec/coderoom/internal/agent"

// LatestCompletedOutput returns the latest completed user-visible output text
// for alias from canonical room state.
func (r *Room) LatestCompletedOutput(alias string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i := len(r.records) - 1; i >= 0; i-- {
		record := r.records[i]
		if record.Alias != alias || record.Msg == nil {
			continue
		}
		output, ok := record.Msg.Content.(agent.Output)
		if !ok {
			continue
		}
		if record.Msg.Mode != agent.ModeSingle {
			if _, open := r.streaming[record.Msg.StreamID]; open {
				continue
			}
		}
		if output.Text == "" {
			continue
		}
		return output.Text, true
	}
	return "", false
}
