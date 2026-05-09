package ui

import (
	"os"
	"testing"
)

func TestCtrlG_withoutEditorAddsSystemRecordAndPreservesBuffer(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")

	m := makeReadyModel(t)
	m.input.SetValue("draft")

	m2, _ := m.startEditorCompose()
	if got := m2.input.Value(); got != "draft" {
		t.Fatalf("expected input preserved when editor is unset, got %q", got)
	}
	if !hasRecord(m2, recordKindSystem, "no editor configured") {
		t.Fatalf("expected system record about editor configuration; records: %v", m2.records)
	}
}

func TestEditorCompose_cancelRestoresPriorBuffer(t *testing.T) {
	m := makeReadyModel(t)
	m.input.SetValue("before")

	next, _ := m.Update(editorComposeMsg{
		prior:    "before",
		canceled: true,
		err:      os.ErrInvalid,
	})
	m2 := next.(Model)
	if got := m2.input.Value(); got != "before" {
		t.Fatalf("expected cancel to restore prior buffer, got %q", got)
	}
}

func TestEditorCompose_successReplacesBuffer(t *testing.T) {
	m := makeReadyModel(t)
	m.input.SetValue("before")

	next, _ := m.Update(editorComposeMsg{
		prior:   "before",
		content: "after",
	})
	m2 := next.(Model)
	if got := m2.input.Value(); got != "after" {
		t.Fatalf("expected success to replace buffer, got %q", got)
	}
}
