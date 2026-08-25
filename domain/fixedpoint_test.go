package domain

import "testing"

func TestParseFixedPointBasic(t *testing.T) {
	fp, err := ParseFixedPoint("12.34", 10, 2, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fp.Value != 1234 || fp.Scale != 2 {
		t.Fatalf("got %+v", fp)
	}
}

func TestParseFixedPointInteger(t *testing.T) {
	fp, err := ParseFixedPoint("7", 10, 2, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fp.Value != 700 {
		t.Fatalf("got value %d", fp.Value)
	}
}

func TestParseFixedPointNegative(t *testing.T) {
	fp, err := ParseFixedPoint("-3.5", 10, 1, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fp.Value != -35 {
		t.Fatalf("got value %d", fp.Value)
	}
}

func TestParseFixedPointRejectsNegativeWhenDisallowed(t *testing.T) {
	if _, err := ParseFixedPoint("-3.5", 10, 1, false); err != ErrInvalidFixedPoint {
		t.Fatalf("expected ErrInvalidFixedPoint, got %v", err)
	}
}

func TestParseFixedPointRejectsTooLong(t *testing.T) {
	if _, err := ParseFixedPoint("123456", 5, 0, false); err != ErrInvalidFixedPoint {
		t.Fatalf("expected ErrInvalidFixedPoint, got %v", err)
	}
}

func TestParseFixedPointRejectsExcessDecimals(t *testing.T) {
	if _, err := ParseFixedPoint("1.234", 10, 2, false); err != ErrInvalidFixedPoint {
		t.Fatalf("expected ErrInvalidFixedPoint, got %v", err)
	}
}

func TestMulCheckedOverflow(t *testing.T) {
	if _, ok := MulChecked(1<<62, 4); ok {
		t.Fatal("expected overflow")
	}
	if v, ok := MulChecked(3, 4); !ok || v != 12 {
		t.Fatalf("got %d %v", v, ok)
	}
}

func TestDivCheckedByZero(t *testing.T) {
	if _, ok := DivChecked(10, 0); ok {
		t.Fatal("expected divide by zero")
	}
	if v, ok := DivChecked(10, 2); !ok || v != 5 {
		t.Fatalf("got %d %v", v, ok)
	}
}
