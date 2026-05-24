package history

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/trigosec/coderoom/internal/agent"
)

func TestFormatFileChangeBody(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := FormatFileChangeBody(nil); got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})

	t.Run("single appends trailing newline", func(t *testing.T) {
		got := FormatFileChangeBody([]agent.FileChange{
			{Path: "a.txt", ChangeKind: "update", Diff: "diff --git a/a.txt b/a.txt\n+hi"},
		})
		want := "=== a.txt (update)\ndiff --git a/a.txt b/a.txt\n+hi\n"
		if got != want {
			t.Fatalf("unexpected body:\nwant:\n%q\ngot:\n%q", want, got)
		}
	})

	t.Run("multiple separated by blank line", func(t *testing.T) {
		got := FormatFileChangeBody([]agent.FileChange{
			{Path: "a.txt", Diff: "A\n"},
			{Path: "b.txt", ChangeKind: "add", Diff: "B"},
		})
		want := "=== a.txt\nA\n\n=== b.txt (add)\nB\n"
		if got != want {
			t.Fatalf("unexpected body:\nwant:\n%q\ngot:\n%q", want, got)
		}
	})
}

func TestRenderFileChange(t *testing.T) {
	colors := func(string) string { return "" }

	t.Run("pending shows placeholder", func(t *testing.T) {
		r := Record{Kind: RecordKindFileChange, Alias: "agent"}
		got := ansi.Strip(renderFileChange(r, 80, colors))
		if !strings.Contains(got, "● agent:") {
			t.Fatalf("expected participant header, got %q", got)
		}
		if !strings.Contains(got, "✎ …") {
			t.Fatalf("expected pending placeholder, got %q", got)
		}
	})

	t.Run("renders file list preview and status", func(t *testing.T) {
		fc := agent.Message{
			StreamID: "s1",
			Mode:     agent.ModeStream,
			Content: agent.FileChangeSet{
				Changes: []agent.FileChange{
					{Path: "a.txt", ChangeKind: "update", Diff: "l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\n"},
					{Path: "b.txt"},
				},
				Status: agent.ToolStatusCompleted,
			},
		}
		r := Record{
			Kind:  RecordKindFileChange,
			Alias: "agent",
			Msg:   &fc,
		}
		got := ansi.Strip(renderFileChange(r, 80, colors))

		for _, needle := range []string{
			"● agent:",
			"✎ files:",
			"  - update a.txt",
			"  - b.txt",
			"  === a.txt (update)",
			"Ctrl+O history, Ctrl+G open transcript",
			"  " + string(agent.ToolStatusCompleted),
		} {
			if !strings.Contains(got, needle) {
				t.Fatalf("expected output to contain %q, got:\n%s", needle, got)
			}
		}
	})
}
