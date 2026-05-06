package participant

import "fmt"

// Registry tracks all participants in a session by alias.
// It is not safe for concurrent use; the caller is responsible for synchronisation.
type Registry struct {
	participants map[string]*Participant
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{participants: make(map[string]*Participant)}
}

// Add registers a participant. Returns an error if p is nil, the alias is
// empty, or the alias is already taken.
func (r *Registry) Add(p *Participant) error {
	if p == nil {
		return fmt.Errorf("participant must not be nil")
	}
	if p.Alias == "" {
		return fmt.Errorf("participant alias must not be empty")
	}
	if _, exists := r.participants[p.Alias]; exists {
		return fmt.Errorf("participant %q already registered", p.Alias)
	}
	r.participants[p.Alias] = p
	return nil
}

// Get returns the participant with the given alias.
func (r *Registry) Get(alias string) (*Participant, bool) {
	p, ok := r.participants[alias]
	return p, ok
}

// Remove removes the participant with the given alias.
// Returns an error if the alias is not found.
func (r *Registry) Remove(alias string) error {
	if _, exists := r.participants[alias]; !exists {
		return fmt.Errorf("participant %q not found", alias)
	}
	delete(r.participants, alias)
	return nil
}

// List returns all registered participants in unspecified order.
func (r *Registry) List() []*Participant {
	result := make([]*Participant, 0, len(r.participants))
	for _, p := range r.participants {
		result = append(result, p)
	}
	return result
}
