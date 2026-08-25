package catalog

import "testing"

func TestLineageMatches(t *testing.T) {
	m := MotherPlant{ID: "m1", LineageID: "L1", Enabled: true}
	if !LineageMatches(m, "L1") {
		t.Fatal("expected lineage match")
	}
	if LineageMatches(m, "L2") {
		t.Fatal("unexpected lineage match")
	}
}

func TestLineageMatchesDisabled(t *testing.T) {
	m := MotherPlant{ID: "m1", LineageID: "L1", Enabled: false}
	if LineageMatches(m, "L1") {
		t.Fatal("disabled mother must not match")
	}
}

func TestMediumFresh(t *testing.T) {
	cur := MediumRevision{FormulaID: "f1", Summary: "v2"}
	if !MediumFresh(cur, "v2") {
		t.Fatal("expected fresh summary")
	}
	if MediumFresh(cur, "v1") {
		t.Fatal("stale summary must not be fresh")
	}
}

func TestPersonnelEnabled(t *testing.T) {
	p := Personnel{ID: "p1", Qualifications: []string{"subculture"}, Enabled: true}
	if !PersonnelEnabled(p, "subculture") {
		t.Fatal("expected qualification")
	}
	if PersonnelEnabled(p, "review") {
		t.Fatal("unexpected qualification")
	}
}
