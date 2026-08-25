// Package measure is the morphology, root, and physicochemical collection
// ledger: a coverage matrix keyed by bottle position and measurement point,
// count conservation, and bounded fixed-point integer evidence.
package measure

import (
	"strawberry-vitro-acclimation-gate/domain"
)

// MetricType enumerates the collected metric kinds.
type MetricType string

const (
	MetricRootLength     MetricType = "root_length"
	MetricTTCViability   MetricType = "ttc_viability"
	MetricSPAD           MetricType = "spad"
	MetricTemperature    MetricType = "temperature"
	MetricLightIntensity MetricType = "light_intensity"
)

// MorphologyCell holds the per-position morphology counts; the four counts must
// be non-negative and sum to the locked seedling count.
type MorphologyCell struct {
	TaskID          domain.TaskID
	Position        string
	Normal          int
	Vitrified       int
	Browned         int
	Contaminated    int
	ScoringPlatePos string
	Supplement      bool
	Version         int64
}

// MeasurementCell is one fixed-point reading at a position and point.
type MeasurementCell struct {
	TaskID   domain.TaskID
	Position string
	Point    string
	Metric   MetricType
	Value    domain.FixedPoint
	Version  int64
}
