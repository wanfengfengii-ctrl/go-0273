package domain

import (
	"errors"
	"strings"
)

// FixedPoint is a bounded integer with a declared number of decimal places.
// Value is the scaled integer; Scale is the number of decimal places.
type FixedPoint struct {
	Value int64
	Scale int
}

// Sentinel errors for fixed-point parsing and arithmetic. They map to stable
// error codes via ErrorCodeOf.
var (
	ErrInvalidFixedPoint  = errors.New("invalid fixed point")
	ErrArithmeticOverflow = errors.New("arithmetic overflow")
	ErrDivideByZero       = errors.New("divide by zero")
)

// ErrorCodeOf maps a fixed-point error to its stable business code.
func ErrorCodeOf(err error) ErrorCode {
	switch {
	case errors.Is(err, ErrInvalidFixedPoint):
		return CodeInvalidFixedPoint
	case errors.Is(err, ErrArithmeticOverflow):
		return CodeArithmeticOverflow
	case errors.Is(err, ErrDivideByZero):
		return CodeDivideByZero
	default:
		return CodeInvalidInput
	}
}

// ParseFixedPoint parses raw as a fixed-point number with at most scale decimal
// places. maxLen bounds the raw input length and allowNegative permits a
// leading minus sign. It rejects over-length input, illegal signs, excess
// decimal places and scaling overflow before any valid evidence is written.
func ParseFixedPoint(raw string, maxLen, scale int, allowNegative bool) (FixedPoint, error) {
	if raw == "" || len(raw) > maxLen {
		return FixedPoint{}, ErrInvalidFixedPoint
	}
	neg := false
	s := raw
	if s[0] == '-' {
		if !allowNegative {
			return FixedPoint{}, ErrInvalidFixedPoint
		}
		neg = true
		s = s[1:]
	}
	if s == "" || s[0] == '+' || s == "." {
		return FixedPoint{}, ErrInvalidFixedPoint
	}

	intPart, fracPart := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, fracPart = s[:i], s[i+1:]
		if strings.IndexByte(fracPart, '.') >= 0 {
			return FixedPoint{}, ErrInvalidFixedPoint
		}
	}
	if intPart == "" {
		intPart = "0"
	}
	if len(fracPart) > scale {
		return FixedPoint{}, ErrInvalidFixedPoint
	}
	if !isDigits(intPart) {
		return FixedPoint{}, ErrInvalidFixedPoint
	}
	if fracPart != "" && !isDigits(fracPart) {
		return FixedPoint{}, ErrInvalidFixedPoint
	}

	pow := pow10(scale)
	whole, ok := atoiChecked(intPart)
	if !ok {
		return FixedPoint{}, ErrInvalidFixedPoint
	}
	scaled, ok := MulChecked(whole, pow)
	if !ok {
		return FixedPoint{}, ErrArithmeticOverflow
	}
	if fracPart != "" {
		pad := fracPart + strings.Repeat("0", scale-len(fracPart))
		frac, ok := atoiChecked(pad)
		if !ok {
			return FixedPoint{}, ErrInvalidFixedPoint
		}
		sum, ok := AddChecked(scaled, frac)
		if !ok {
			return FixedPoint{}, ErrArithmeticOverflow
		}
		scaled = sum
	}
	if neg {
		scaled = -scaled
	}
	return FixedPoint{Value: scaled, Scale: scale}, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

const maxInt = int64(^uint64(0) >> 1)

func atoiChecked(s string) (int64, bool) {
	var v int64
	for i := 0; i < len(s); i++ {
		d := int64(s[i] - '0')
		if v > (maxInt-d)/10 {
			return 0, false
		}
		v = v*10 + d
	}
	return v, true
}

func pow10(n int) int64 {
	v := int64(1)
	for i := 0; i < n; i++ {
		v *= 10
	}
	return v
}

// AddChecked returns a+b and reports overflow.
func AddChecked(a, b int64) (int64, bool) {
	c := a + b
	if (b > 0 && c < a) || (b < 0 && c > a) {
		return 0, false
	}
	return c, true
}

// MulChecked returns a*b and reports overflow.
func MulChecked(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	c := a * b
	if c/b != a {
		return 0, false
	}
	return c, true
}

// DivChecked returns a/b and reports divide-by-zero.
func DivChecked(a, b int64) (int64, bool) {
	if b == 0 {
		return 0, false
	}
	return a / b, true
}
