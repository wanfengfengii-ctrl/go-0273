package measure

import "strawberry-vitro-acclimation-gate/domain"

// ValidateCounts checks that a morphology cell's four counts are non-negative
// and sum to the locked seedling count.
func ValidateCounts(c MorphologyCell, lockedSeedlings int) bool {
	if c.Normal < 0 || c.Vitrified < 0 || c.Browned < 0 || c.Contaminated < 0 {
		return false
	}
	return c.Normal+c.Vitrified+c.Browned+c.Contaminated == lockedSeedlings
}

// GlassRatePct returns the vitrification percentage rounded toward zero, or an
// error on a zero total.
func GlassRatePct(vitrified, total int) (int, error) {
	if total == 0 {
		return 0, domain.ErrDivideByZero
	}
	return vitrified * 100 / total, nil
}

// BrownRatePct returns the browning percentage rounded toward zero, or an
// error on a zero total.
func BrownRatePct(browned, total int) (int, error) {
	if total == 0 {
		return 0, domain.ErrDivideByZero
	}
	return browned * 100 / total, nil
}
