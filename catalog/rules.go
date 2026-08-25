package catalog

// LineageMatches reports whether a mother plant belongs to the given
// detoxification lineage and is enabled.
func LineageMatches(m MotherPlant, lineageID string) bool {
	return m.Enabled && m.LineageID == lineageID
}

// MediumFresh reports whether a locked medium summary still equals the
// catalog's current normalized summary for the formula.
func MediumFresh(current MediumRevision, lockedSummary string) bool {
	return current.Summary == lockedSummary
}

// PersonnelEnabled reports whether p is enabled and holds the given
// qualification.
func PersonnelEnabled(p Personnel, qualification string) bool {
	if !p.Enabled {
		return false
	}
	for _, q := range p.Qualifications {
		if q == qualification {
			return true
		}
	}
	return false
}
