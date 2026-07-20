package promptlang

import "fmt"

// Registry stores command definitions for one running room.
type Registry struct {
	definitions map[string]CommandDefinition
}

// NewRegistry creates an empty command registry.
func NewRegistry() *Registry {
	return &Registry{definitions: make(map[string]CommandDefinition)}
}

// Define stores a command definition without evaluating its body.
func (r *Registry) Define(definition CommandDefinition) error {
	if !isIdentifier(definition.Name) {
		return InvalidCommandNameError{Name: definition.Name}
	}
	if isReservedCommand(definition.Name) {
		return ReservedCommandNameError{Name: definition.Name}
	}
	if _, exists := r.definitions[definition.Name]; exists {
		return CommandAlreadyDefinedError{Name: definition.Name}
	}
	if r.definitions == nil {
		r.definitions = make(map[string]CommandDefinition)
	}
	r.definitions[definition.Name] = definition
	return nil
}

// Resolve returns the unevaluated shell body for a command invocation.
func (r *Registry) Resolve(invocation CommandInvocation) (Shell, error) {
	definition, exists := r.definitions[invocation.Name]
	if !exists {
		return Shell{}, UndefinedCommandError(invocation)
	}
	return definition.Body, nil
}

// InvalidCommandNameError reports a name that is not a valid identifier.
type InvalidCommandNameError struct{ Name string }

func (e InvalidCommandNameError) Error() string {
	return fmt.Sprintf("invalid command name %q", e.Name)
}

// ReservedCommandNameError reports an attempted built-in redefinition.
type ReservedCommandNameError struct{ Name string }

func (e ReservedCommandNameError) Error() string {
	return fmt.Sprintf("command name %q is reserved", e.Name)
}

// CommandAlreadyDefinedError reports a duplicate definition.
type CommandAlreadyDefinedError struct{ Name string }

func (e CommandAlreadyDefinedError) Error() string {
	return fmt.Sprintf("command %q is already defined", e.Name)
}

// UndefinedCommandError reports an invocation without a matching definition.
type UndefinedCommandError struct{ Name string }

func (e UndefinedCommandError) Error() string {
	return fmt.Sprintf("command %q is not defined", e.Name)
}
