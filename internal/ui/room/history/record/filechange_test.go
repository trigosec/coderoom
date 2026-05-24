package record

import (
	"regexp"
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

func TestRenderFileChangePending(t *testing.T) {
	colors := func(string) string { return "" }

	r := Record{Kind: KindFileChange, Alias: "agent"}
	got := renderFileChangeStripped(t, r, colors)
	if !strings.Contains(got, "● agent:") {
		t.Fatalf("expected participant header, got %q", got)
	}
	if !strings.Contains(got, "✎ …") {
		t.Fatalf("expected pending placeholder, got %q", got)
	}
}

func TestRenderFileChangeRendersListPreviewAndStatus(t *testing.T) {
	colors := func(string) string { return "" }

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
	r := Record{Kind: KindFileChange, Alias: "agent", Msg: &fc}
	got := renderFileChangeStripped(t, r, colors)

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
}

func TestRenderFileChangeDeduplicatesEntries(t *testing.T) {
	colors := func(string) string { return "" }

	fc := agent.Message{
		StreamID: "s1",
		Mode:     agent.ModeStream,
		Content: agent.FileChangeSet{
			Changes: []agent.FileChange{
				{Path: "a.txt", ChangeKind: "add"},
				{Path: "a.txt", ChangeKind: "add"},
				{Path: "a.txt", ChangeKind: "update"},
				{Path: "b.txt"},
				{Path: "b.txt"},
			},
			Status: agent.ToolStatusCompleted,
		},
	}
	r := Record{Kind: KindFileChange, Alias: "agent", Msg: &fc}
	got := renderFileChangeStripped(t, r, colors)

	if c := strings.Count(got, "- add a.txt"); c != 1 {
		t.Fatalf("expected one entry for '- add a.txt', got %d:\n%s", c, got)
	}
	if c := strings.Count(got, "- update a.txt"); c != 1 {
		t.Fatalf("expected one entry for '- update a.txt', got %d:\n%s", c, got)
	}
	if c := strings.Count(got, "- b.txt"); c != 1 {
		t.Fatalf("expected one entry for '- b.txt', got %d:\n%s", c, got)
	}
}

func TestRenderFileChangePencilPromptIsColorized(t *testing.T) {
	withANSIProfile(t, func() {
		colors := func(string) string { return "1" }
		fc := agent.Message{
			StreamID: "s1",
			Mode:     agent.ModeStream,
			Content: agent.FileChangeSet{
				Changes: []agent.FileChange{{Path: "a.txt"}},
			},
		}
		r := Record{Kind: KindFileChange, Alias: "agent", Msg: &fc}
		got := r.Render(RenderContext{Key: RenderKey{Mode: RenderViewport, Width: 80}, ColorForAlias: colors})

		re := regexp.MustCompile(`\x1b\[[0-9;]*m✎\s*\x1b\[[0-9;]*m`)
		if !re.MatchString(got) {
			t.Fatalf("expected colorized pencil prompt, got:\n%s", got)
		}
	})
}

func renderFileChangeStripped(t *testing.T, r Record, colors func(string) string) string {
	t.Helper()
	return ansi.Strip(r.Render(RenderContext{Key: RenderKey{Mode: RenderViewport, Width: 80}, ColorForAlias: colors}))
}
