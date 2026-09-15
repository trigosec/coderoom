package session

import (
	"fmt"

	"github.com/trigosec/coderoom/internal/policy"
)

// EnablePolicyCommand enables a room-local runtime policy.
type EnablePolicyCommand struct {
	Name policy.Name
}

func (c EnablePolicyCommand) execute(s *Session) error {
	if err := s.policies.Enable(c.Name); err != nil {
		return fmt.Errorf("enable policy %q: %w", c.Name, err)
	}
	return nil
}
