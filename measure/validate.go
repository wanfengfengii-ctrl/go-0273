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

// RootMetrics is the set of metrics a root-verification batch must supply for
// every locked bottle position and root point before the task may advance.
var RootMetrics = []MetricType{MetricRootLength, MetricTTCViability}

// ValidateRootCoverage checks a root-measurement batch against the locked
// snapshot's coverage matrix. Every locked bottle position paired with every
// locked root point must carry the complete RootMetrics set, with no
// duplicate, unknown, or extra cells. Missing cells are returned as ordered
// reasons so the API can surface the exact gaps rather than treat them as
// closed.
func ValidateRootCoverage(cells []MeasurementCell, positions, points []string) []domain.Reason {
	allowed := map[string]bool{}
	for _, p := range positions {
		allowed[p] = true
	}
	knownPoints := map[string]bool{}
	for _, p := range points {
		knownPoints[p] = true
	}
	needed := MetricSet(RootMetrics)
	seen := map[string]map[string]map[MetricType]bool{}
	var reasons []domain.Reason
	for _, c := range cells {
		// Only root metrics are part of the root coverage matrix; physicochemical
		// metrics are submitted separately and do not gate this advance.
		if !needed[c.Metric] {
			reasons = append(reasons, domain.Reason{BottlePosition: c.Position, Field: string(c.Metric), Detail: "unexpected metric"})
			continue
		}
		if !allowed[c.Position] {
			reasons = append(reasons, domain.Reason{BottlePosition: c.Position, Field: string(c.Metric), Detail: "unknown bottle position"})
			continue
		}
		if !knownPoints[c.Point] {
			reasons = append(reasons, domain.Reason{BottlePosition: c.Position, Field: c.Point, Detail: "unknown root point"})
			continue
		}
		bucket, ok := seen[c.Position]
		if !ok {
			bucket = map[string]map[MetricType]bool{}
			seen[c.Position] = bucket
		}
		metrics, ok := bucket[c.Point]
		if !ok {
			metrics = map[MetricType]bool{}
			bucket[c.Point] = metrics
		}
		if metrics[c.Metric] {
			reasons = append(reasons, domain.Reason{BottlePosition: c.Position, Field: string(c.Metric), Detail: "duplicate reading for " + c.Point})
			continue
		}
		metrics[c.Metric] = true
	}
	for _, pos := range positions {
		bucket := seen[pos]
		for _, point := range points {
			metrics := map[MetricType]bool{}
			if bucket != nil {
				metrics = bucket[point]
			}
			for _, m := range RootMetrics {
				if metrics == nil || !metrics[m] {
					reasons = append(reasons, domain.Reason{BottlePosition: pos, Field: point + "/" + string(m), Detail: "missing reading"})
				}
			}
		}
	}
	return domain.SortReasons(reasons)
}

// MetricSet reports whether a metric belongs to the given set.
func MetricSet(ms []MetricType) map[MetricType]bool {
	out := make(map[MetricType]bool, len(ms))
	for _, m := range ms {
		out[m] = true
	}
	return out
}
