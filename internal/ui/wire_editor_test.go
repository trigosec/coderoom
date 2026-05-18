package ui

import (
	"os"
	"testing"

	"github.com/trigosec/coderoom/internal/ui/editor"
)

func TestCtrlG_withoutEditorAddsSystemRecordAndPreservesBuffer(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")

	m := makeReadyModel(t)
	m.compose = m.compose.SetValue("draft")

	m2, _ := m.startEditorCompose()
	if got := m2.compose.Value(); got != "draft" {
		t.Fatalf("expected input preserved when editor is unset, got %q", got)
	}
	if !hasRecord(m2, recordKindSystem, "no editor configured") {
		t.Fatalf("expected system record about editor configuration; records: %v", m2.records)
	}
}

func TestEditorCompose_cancelRestoresPriorBuffer(t *testing.T) {
	m := makeReadyModel(t)
	m.compose = m.compose.SetValue("before")

	next, _ := m.Update(editor.Response{
		Purpose:   editor.PurposeCompose,
		PriorText: "before",
		Canceled:  true,
		Err:       os.ErrInvalid,
	})
	m2 := next.(Model)
	if got := m2.compose.Value(); got != "before" {
		t.Fatalf("expected cancel to restore prior buffer, got %q", got)
	}
}

func TestEditorCompose_successReplacesBuffer(t *testing.T) {
	m := makeReadyModel(t)
	m.compose = m.compose.SetValue("before")

	next, _ := m.Update(editor.Response{
		Purpose:   editor.PurposeCompose,
		PriorText: "before",
		NewText:   "after",
	})
	m2 := next.(Model)
	if got := m2.compose.Value(); got != "after" {
		t.Fatalf("expected success to replace buffer, got %q", got)
	}
}

func TestEditorResponse_transcriptDoesNotMutateComposer(t *testing.T) {
	m := makeReadyModel(t)
	m.compose = m.compose.SetValue("draft")

	next, _ := m.Update(editor.Response{
		Purpose:  editor.PurposeTranscript,
		NewText:  "ignored",
		Err:      nil,
		Canceled: false,
	})
	m2 := next.(Model)
	if got := m2.compose.Value(); got != "draft" {
		t.Fatalf("expected transcript response to not mutate composer, got %q", got)
	}
}
