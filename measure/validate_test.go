package measure

import "testing"

func TestValidateCountsConserved(t *testing.T) {
	c := MorphologyCell{Normal: 8, Vitrified: 1, Browned: 0, Contaminated: 1}
	if !ValidateCounts(c, 10) {
		t.Fatal("expected conserved counts")
	}
	if ValidateCounts(c, 9) {
		t.Fatal("must reject wrong total")
	}
}

func TestValidateCountsNegative(t *testing.T) {
	c := MorphologyCell{Normal: -1, Vitrified: 0, Browned: 0, Contaminated: 0}
	if ValidateCounts(c, -1) {
		t.Fatal("must reject negative counts")
	}
}

func TestGlassRatePct(t *testing.T) {
	if v, err := GlassRatePct(3, 10); err != nil || v != 30 {
		t.Fatalf("got %d %v", v, err)
	}
	if _, err := GlassRatePct(3, 0); err == nil {
		t.Fatal("expected divide by zero")
	}
}

func TestBrownRatePct(t *testing.T) {
	if v, err := BrownRatePct(1, 10); err != nil || v != 10 {
		t.Fatalf("got %d %v", v, err)
	}
	if _, err := BrownRatePct(0, 0); err == nil {
		t.Fatal("expected divide by zero")
	}
}
