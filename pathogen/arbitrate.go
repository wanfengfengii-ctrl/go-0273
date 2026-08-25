package pathogen

import (
	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/measure"
	"strawberry-vitro-acclimation-gate/task"
)

// Trigger identifies an anomaly class that can open a recheck round.
type Trigger string

const (
	TriggerVitrification  Trigger = "vitrification"
	TriggerBrowning       Trigger = "browning"
	TriggerRootViability  Trigger = "root_viability"
	TriggerVirus          Trigger = "virus"
	TriggerEndophyte      Trigger = "endophyte"
	TriggerTripleConflict Trigger = "triple_conflict"
)

// pct returns a*b/100 rounded toward zero, or an error on division by zero.
func pct(part, whole int) (int, error) {
	if whole == 0 {
		return 0, domain.ErrDivideByZero
	}
	return part * 100 / whole, nil
}

// EvaluateMorphology checks the vitrification and browning percentages against
// the locked thresholds, returning the affected positions as ordered reasons.
func EvaluateMorphology(cells []measure.MorphologyCell, th task.Thresholds) []domain.Reason {
	var reasons []domain.Reason
	for _, c := range cells {
		total := c.Normal + c.Vitrified + c.Browned + c.Contaminated
		if total == 0 {
			continue
		}
		if g, err := pct(c.Vitrified, total); err == nil && g > th.VitrificationMaxPct {
			reasons = append(reasons, domain.Reason{BottlePosition: c.Position, Field: string(TriggerVitrification), Detail: "exceeds threshold"})
		}
		if b, err := pct(c.Browned, total); err == nil && b > th.BrowningMaxPct {
			reasons = append(reasons, domain.Reason{BottlePosition: c.Position, Field: string(TriggerBrowning), Detail: "exceeds threshold"})
		}
	}
	return reasons
}

// EvaluateRoot checks the TTC root viability readings against the minimum.
func EvaluateRoot(cells []measure.MeasurementCell, th task.Thresholds) []domain.Reason {
	var reasons []domain.Reason
	for _, c := range cells {
		if c.Metric != measure.MetricTTCViability {
			continue
		}
		if c.Value.Value < th.RootViabilityMin {
			reasons = append(reasons, domain.Reason{BottlePosition: c.Position, Field: string(TriggerRootViability), Detail: "below minimum"})
		}
	}
	return reasons
}

// EvaluatePathogen checks virus Ct and endophyte colony counts against maxima.
func EvaluatePathogen(ev []PathogenEvidence, th task.Thresholds) []domain.Reason {
	var reasons []domain.Reason
	for _, e := range ev {
		if e.DetectionType == "virus" && e.Value.Value > th.VirusCtMax {
			reasons = append(reasons, domain.Reason{TestWell: e.Well, Field: string(TriggerVirus), Detail: "Ct above maximum"})
		}
		if e.DetectionType == "endophyte" && e.Value.Value > th.EndophyteCFUMax {
			reasons = append(reasons, domain.Reason{TestWell: e.Well, Field: string(TriggerEndophyte), Detail: "CFU above maximum"})
		}
	}
	return reasons
}

// Arbitrate combines morphology, root, and pathogen checks into a single
// ordered reason list. A non-empty list means the task cannot acclimate and a
// recheck round (or isolation) is required.
func Arbitrate(mcells []measure.MorphologyCell, cells []measure.MeasurementCell, ev []PathogenEvidence, th task.Thresholds) []domain.Reason {
	reasons := append([]domain.Reason{}, EvaluateMorphology(mcells, th)...)
	reasons = append(reasons, EvaluateRoot(cells, th)...)
	reasons = append(reasons, EvaluatePathogen(ev, th)...)
	return domain.SortReasons(reasons)
}
