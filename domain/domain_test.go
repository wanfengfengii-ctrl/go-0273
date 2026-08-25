package domain

import "testing"

func TestTaskStateTerminal(t *testing.T) {
	terminals := []TaskState{StateAcclimated, StateIsolated, StateCancelled}
	for _, s := range terminals {
		if !s.IsTerminal() {
			t.Fatalf("%v should be terminal", s)
		}
	}
	if StatePendingReview.IsTerminal() {
		t.Fatal("pending review must not be terminal")
	}
}

func TestSortReasonsCanonical(t *testing.T) {
	in := []Reason{
		{MotherPlantID: "B"},
		{MotherPlantID: "A", SubcultureBatch: "2"},
		{MotherPlantID: "A", SubcultureBatch: "1"},
	}
	out := SortReasons(in)
	if out[0].SubcultureBatch != "1" || out[1].SubcultureBatch != "2" || out[2].MotherPlantID != "B" {
		t.Fatalf("unexpected order: %+v", out)
	}
}

func TestLogicalClockAdvance(t *testing.T) {
	c := NewLogicalClock(0)
	if c.Now() != 0 {
		t.Fatalf("expected 0, got %d", c.Now())
	}
	if c.Advance() != 1 || c.Now() != 1 {
		t.Fatalf("expected 1, got %d", c.Now())
	}
	c.Set(10)
	if c.Now() != 10 {
		t.Fatalf("expected 10, got %d", c.Now())
	}
}
