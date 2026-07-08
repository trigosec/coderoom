package room

import "testing"

func newTestModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, "")
	t.Cleanup(m.Close)
	return m
}
