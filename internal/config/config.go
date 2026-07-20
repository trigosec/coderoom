// Package config loads repo-local coderoom configuration.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	configDirName       = ".coderoom"
	participantsDirName = "participants"
	rolePromptsDirName  = "prompts/roles"
)

var (
	errParticipantConfigNotFound = errors.New("participant config not found")
	errInvalidParticipantAlias   = errors.New("invalid participant alias")
	errInvalidParticipantRole    = errors.New("invalid participant role")
	errInvalidParticipantYAML    = errors.New("invalid participant yaml")
	errRolePromptNotFound        = errors.New("role prompt not found")
)

// Config resolves repo-local participant configuration relative to a repo root.
type Config struct {
	root string
}

// ParticipantConfig is the resolved invite-time configuration for a participant.
type ParticipantConfig struct {
	Alias          string
	Role           string
	Prompt         string
	LoadedFromDisk bool
}

type participantDefinition struct {
	Alias string
	Role  string
}

type rawParticipantDefinition struct {
	Alias string  `yaml:"alias"`
	Role  *string `yaml:"role"`
}

// New returns a repo-local configuration loader rooted at root.
func New(root string) *Config {
	return &Config{root: root}
}

// ForParticipant resolves participant configuration for alias. If no matching
// YAML file exists, it returns identification-only configuration.
func (c *Config) ForParticipant(alias string) (ParticipantConfig, error) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return ParticipantConfig{}, fmt.Errorf("%w: participant alias must not be empty", errInvalidParticipantAlias)
	}
	if err := validatePathSegment(alias, errInvalidParticipantAlias); err != nil {
		return ParticipantConfig{}, err
	}

	def, err := c.loadParticipantDefinition(alias)
	switch {
	case err == nil:
	case errors.Is(err, errParticipantConfigNotFound):
		return ParticipantConfig{
			Alias:          alias,
			Prompt:         buildPrompt(alias, "", ""),
			LoadedFromDisk: false,
		}, nil
	default:
		return ParticipantConfig{}, err
	}

	rolePrompt, err := c.loadRolePrompt(def.Role)
	if err != nil {
		return ParticipantConfig{}, err
	}

	return ParticipantConfig{
		Alias:          def.Alias,
		Role:           def.Role,
		Prompt:         buildPrompt(def.Alias, def.Role, rolePrompt),
		LoadedFromDisk: true,
	}, nil
}

func (c *Config) loadParticipantDefinition(alias string) (participantDefinition, error) {
	path, err := c.participantPath(alias)
	if err != nil {
		return participantDefinition{}, err
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return participantDefinition{}, errParticipantConfigNotFound
		}
		return participantDefinition{}, fmt.Errorf("read participant config %q: %w", path, err)
	}

	def, err := parseParticipantDefinition(data)
	if err != nil {
		return participantDefinition{}, fmt.Errorf("load participant config %q: %w", path, err)
	}
	if def.Alias == "" {
		return participantDefinition{}, fmt.Errorf("load participant config %q: %w: participant alias must not be empty", path, errInvalidParticipantYAML)
	}
	if err := validatePathSegment(def.Alias, errInvalidParticipantAlias); err != nil {
		return participantDefinition{}, fmt.Errorf("load participant config %q: %w", path, err)
	}
	if def.Alias != alias {
		return participantDefinition{}, fmt.Errorf("load participant config %q: %w: alias %q does not match filename %q", path, errInvalidParticipantYAML, def.Alias, alias)
	}
	if def.Role != "" {
		if err := validatePathSegment(def.Role, errInvalidParticipantRole); err != nil {
			return participantDefinition{}, fmt.Errorf("load participant config %q: %w", path, err)
		}
	}
	return def, nil
}

func parseParticipantDefinition(data []byte) (participantDefinition, error) {
	var raw rawParticipantDefinition

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&raw); err != nil {
		return participantDefinition{}, fmt.Errorf("%w: %v", errInvalidParticipantYAML, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != nil {
		if !errors.Is(err, io.EOF) {
			return participantDefinition{}, fmt.Errorf("%w: %v", errInvalidParticipantYAML, err)
		}
	} else {
		return participantDefinition{}, fmt.Errorf("%w: multiple yaml documents are not supported", errInvalidParticipantYAML)
	}

	def := participantDefinition{
		Alias: strings.TrimSpace(raw.Alias),
	}
	if raw.Role == nil {
		return def, nil
	}

	role := strings.TrimSpace(*raw.Role)
	def.Role = role
	return def, nil
}

func (c *Config) loadRolePrompt(role string) (string, error) {
	role = strings.TrimSpace(role)
	if role == "" {
		return "", nil
	}
	path, err := c.rolePromptPath(role)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: %q for role %q", errRolePromptNotFound, path, role)
		}
		return "", fmt.Errorf("read role prompt %q: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func (c *Config) participantPath(alias string) (string, error) {
	if err := validatePathSegment(alias, errInvalidParticipantAlias); err != nil {
		return "", err
	}
	return filepath.Join(c.root, configDirName, participantsDirName, alias+".yaml"), nil
}

func (c *Config) rolePromptPath(role string) (string, error) {
	if err := validatePathSegment(role, errInvalidParticipantRole); err != nil {
		return "", err
	}
	return filepath.Join(c.root, configDirName, rolePromptsDirName, role+".md"), nil
}

func validatePathSegment(value string, kind error) error {
	value = strings.TrimSpace(value)
	switch {
	case value == "":
		return fmt.Errorf("%w: value must not be empty", kind)
	case value == "." || value == "..":
		return fmt.Errorf("%w: %q is not a valid path segment", kind, value)
	case strings.Contains(value, "/"), strings.Contains(value, `\`):
		return fmt.Errorf("%w: %q is not a valid path segment", kind, value)
	}
	return nil
}

func buildPrompt(alias, role, rolePrompt string) string {
	parts := []string{
		fmt.Sprintf("Your name is %s.\nYou will be referred to as %s or @%s.", alias, alias, alias),
	}
	if role != "" {
		parts = append(parts, fmt.Sprintf("You are a %s.", role))
	}
	if rolePrompt != "" {
		parts = append(parts, rolePrompt)
	}
	return strings.Join(parts, "\n\n")
}
