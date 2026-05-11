package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/trigosec/coderoom/internal/participant"
)

const aliasMax = 10

func activityTier(k participant.Status) int {
	switch k {
	case participant.StatusWorking:
		return 1
	case participant.StatusStarting:
		return 2
	case participant.StatusCrashed:
		return 3
	case participant.StatusIdle:
		return 4
	default:
		return 99
	}
}

func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	secs := int(d.Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	mins := secs / 60
	sec := secs % 60
	if mins < 60 {
		return fmt.Sprintf("%dm%02ds", mins, sec)
	}
	h := mins / 60
	m := mins % 60
	return fmt.Sprintf("%dh%02dm", h, m)
}

func padOrTruncateToWidth(s string, width int) string {
	w := ansi.StringWidth(s)
	if w == width {
		return s
	}
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	// Truncate by runes; good enough for aliases (mostly ASCII) and simple glyphs.
	var b strings.Builder
	curW := 0
	for _, r := range s {
		rw := ansi.StringWidth(string(r))
		if curW+rw > width {
			break
		}
		b.WriteRune(r)
		curW += rw
	}
	out := b.String()
	if ansi.StringWidth(out) < width {
		out += strings.Repeat(" ", width-ansi.StringWidth(out))
	}
	return out
}

func renderAliasCell(alias string) string {
	if ansi.StringWidth(alias) > aliasMax {
		// Best-effort ellipsis truncate.
		runes := []rune(alias)
		if len(runes) > 0 {
			alias = string(runes[:max(aliasMax-1, 0)]) + "…"
		}
	}
	return padOrTruncateToWidth(alias, aliasMax)
}

func cellWidth() int {
	// Conceptual width:
	// glyphWidth + 1 + aliasMax + 1 + len("(59m59s)")
	// We treat the glyph as width 2 for a stable conservative layout, since many
	// terminals render these as wide characters.
	glyphW := 2
	return glyphW + 1 + aliasMax + 1 + len("(59m59s)")
}

func (m Model) renderParticipantCells(innerWidth int, now time.Time, ps []participant.Participant) string {
	w := cellWidth()
	if innerWidth <= 0 {
		return ""
	}
	n := max(innerWidth/w, 1)

	entries := make([]participant.Participant, 0, len(ps))
	for _, p := range ps {
		if p.Alias == "" {
			continue
		}
		entries = append(entries, p)
	}
	sort.Slice(entries, func(i, j int) bool {
		ti := activityTier(entries[i].Status)
		tj := activityTier(entries[j].Status)
		if ti != tj {
			return ti < tj
		}
		return entries[i].Alias < entries[j].Alias
	})

	visibleSlots := n
	overflow := 0
	if len(entries) > n {
		overflow = len(entries) - (n - 1)
		visibleSlots = n - 1
	}

	var cells []string
	for i := 0; i < min(visibleSlots, len(entries)); i++ {
		cells = append(cells, m.renderCell(entries[i], now, w))
	}
	if overflow > 0 {
		ov := fmt.Sprintf("+%d", overflow)
		cells = append(cells, padOrTruncateToWidth(ov, w))
	}
	for len(cells) < n {
		cells = append(cells, strings.Repeat(" ", w))
	}

	return strings.TrimRight(strings.Join(cells, ""), " ")
}

func (m Model) renderCell(p participant.Participant, now time.Time, width int) string {
	glyph := "●"
	showElapsed := false
	switch p.Status {
	case participant.StatusStarting:
		glyph = "◌"
		showElapsed = true
	case participant.StatusWorking:
		showElapsed = true
		sec := int(now.Unix()) % 2
		if sec == 0 {
			glyph = "⏹"
		} else {
			glyph = "◆"
		}
	case participant.StatusCrashed:
		glyph = "✖"
		showElapsed = true
	case participant.StatusIdle:
		glyph = "●"
	}

	base := glyph + " " + renderAliasCell(p.Alias)
	if showElapsed {
		base += " (" + formatElapsed(now.Sub(p.Since)) + ")"
	}
	return padOrTruncateToWidth(base, width)
}
