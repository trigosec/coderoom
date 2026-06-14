// Package editor encapsulates opening a temp file in the user's configured
// editor ($EDITOR/$VISUAL) and returning the edited content back to the UI.
package editor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Response is the Bubble Tea message emitted after the editor process exits.
type Response struct {
	Purpose   Purpose
	PriorText string
	NewText   string
	Canceled  bool
	Err       error
}

// Purpose identifies which UI flow opened the editor.
type Purpose string

const (
	// PurposeCompose is used for editing the composer buffer.
	PurposeCompose Purpose = "compose"
	// PurposeTranscript is used for viewing/exporting a transcript snapshot.
	PurposeTranscript Purpose = "transcript"
)

// Request describes how to open and seed a temp file in the user's editor.
type Request struct {
	// Purpose identifies the caller so the UI can route the result.
	// Examples: PurposeCompose, PurposeTranscript.
	Purpose Purpose
	// PriorText is the value to restore if the editor is canceled/returns non-zero.
	PriorText string
	// InitialText is written to the temp file before launching the editor.
	InitialText string
	// TempPattern is passed to os.CreateTemp (e.g. "coderoom-compose-*.md").
	TempPattern string
	// ReadOnly makes the file read-only before opening (best-effort).
	ReadOnly bool
	// TrimFinalNewline removes a single trailing \n from the file content.
	TrimFinalNewline bool
}

// Resolve returns the configured editor command string.
func Resolve() (string, error) {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("VISUAL"))
	}
	if editor == "" {
		return "", fmt.Errorf("no editor configured (set $EDITOR or $VISUAL)")
	}
	return editor, nil
}

func normalizeContent(s string, trimFinalNewline bool) string {
	if !trimFinalNewline {
		return s
	}
	return strings.TrimSuffix(s, "\n")
}

// OpenTempFileInEditor runs $EDITOR/$VISUAL over a temp file and returns a
// Bubble Tea Cmd producing Response.
func OpenTempFileInEditor(req Request) (tea.Cmd, error) {
	editor, err := Resolve()
	if err != nil {
		return nil, err
	}
	if filepath.Ext(req.TempPattern) == "" {
		return nil, fmt.Errorf("temp pattern must include a file extension (e.g. *.md)")
	}
	return openWithEditor(editor, req)
}

// openWithEditor is split for unit tests (inject editor string).
func openWithEditor(editor string, req Request) (tea.Cmd, error) {
	f, err := os.CreateTemp("", req.TempPattern)
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	path := f.Name()
	if _, err := f.WriteString(req.InitialText); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("close temp file: %w", err)
	}
	if req.ReadOnly {
		_ = os.Chmod(path, 0o400) // best-effort; some filesystems may not support
	}

	args := strings.Fields(editor)
	//nolint:gosec // $EDITOR/$VISUAL is explicitly user-configured; we execute it with a temp file path.
	cmd := exec.Command(args[0], append(args[1:], path)...)

	return tea.ExecProcess(cmd, func(runErr error) tea.Msg {
		defer func() { _ = os.Remove(path) }()
		if runErr != nil {
			return Response{Purpose: req.Purpose, PriorText: req.PriorText, Canceled: true, Err: runErr}
		}
		b, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return Response{Purpose: req.Purpose, PriorText: req.PriorText, Canceled: true, Err: err}
		}
		content := normalizeContent(string(b), req.TrimFinalNewline)
		return Response{Purpose: req.Purpose, PriorText: req.PriorText, NewText: content}
	}), nil
}
