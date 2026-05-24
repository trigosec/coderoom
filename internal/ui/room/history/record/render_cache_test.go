package record

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/trigosec/coderoom/internal/agent"
)

func TestRenderCached_cacheHitReturnsSameString(t *testing.T) {
	r := NewAgent("bot", agent.Message{
		StreamID: "out1",
		Mode:     agent.ModeStream,
		Content:  agent.Output{Text: "hello"},
	})
	ctx := RenderContext{Key: RenderKey{Mode: RenderViewport, Width: 80, ColorVersion: 1}}

	first, r1 := r.RenderCached(ctx)
	second, r2 := r1.RenderCached(ctx)
	if first != second {
		t.Fatalf("expected cached render to be identical:\nfirst=%q\nsecond=%q", first, second)
	}

	third, _ := r2.RenderCached(ctx)
	if second != third {
		t.Fatalf("expected repeated cached render to be identical:\nsecond=%q\nthird=%q", second, third)
	}
}

func TestRenderCached_widthChangeInvalidatesCache(t *testing.T) {
	msg := agent.Message{StreamID: "out1", Mode: agent.ModeStream, Content: agent.Output{Text: strings.Repeat("x", 40)}}
	r := NewAgent("bot", msg)

	ctx80 := RenderContext{Key: RenderKey{Mode: RenderViewport, Width: 80, ColorVersion: 1}}
	ctx10 := RenderContext{Key: RenderKey{Mode: RenderViewport, Width: 10, ColorVersion: 1}}

	out80, r1 := r.RenderCached(ctx80)
	out10, _ := r1.RenderCached(ctx10)

	if ansi.Strip(out80) == ansi.Strip(out10) {
		t.Fatalf("expected width change to affect rendering, got identical output %q", ansi.Strip(out10))
	}
}

func TestRenderCached_colorVersionInvalidatesCache(t *testing.T) {
	withANSIProfile(t, func() {
		r := Record{
			Kind:  KindReasoning,
			Alias: "alice",
			Msg: &agent.Message{
				StreamID: "r1",
				Mode:     agent.ModeStream,
				Content:  agent.Reasoning{Text: "**bold**"},
			},
		}

		ctxV1 := RenderContext{
			Key: RenderKey{
				Mode:         RenderViewport,
				Width:        80,
				ColorVersion: 1,
			},
			ColorForAlias: func(string) string { return "1" },
		}
		ctxV2 := ctxV1
		ctxV2.Key.ColorVersion = 2

		_, r1 := r.RenderCached(ctxV1)
		r1.renderCache.rendered = "sentinel"
		out2, r2 := r1.RenderCached(ctxV2)

		if !r1.renderCache.valid || r1.renderCache.key.ColorVersion != 1 {
			t.Fatalf("unexpected cache state after first render: %+v", r1.renderCache)
		}
		if !r2.renderCache.valid || r2.renderCache.key.ColorVersion != 2 {
			t.Fatalf("unexpected cache state after second render: %+v", r2.renderCache)
		}
		if out2 == "sentinel" {
			t.Fatalf("expected cache miss after version bump, got cached sentinel output")
		}
	})
}

func TestRenderCached_transcriptIgnoresWidthInCacheKey(t *testing.T) {
	r := NewAgent("bot", agent.Message{
		StreamID: "out1",
		Mode:     agent.ModeStream,
		Content:  agent.Output{Text: "hello"},
	})

	ctxWide := RenderContext{Key: RenderKey{Mode: RenderTranscript, Width: 200, ColorVersion: 1}}
	ctxNarrow := RenderContext{Key: RenderKey{Mode: RenderTranscript, Width: 10, ColorVersion: 1}}

	outWide, r1 := r.RenderCached(ctxWide)
	outNarrow, _ := r1.RenderCached(ctxNarrow)
	if outWide != outNarrow {
		t.Fatalf("expected transcript mode to ignore width; outputs differ:\nwide=%q\nnarrow=%q", outWide, outNarrow)
	}
}
