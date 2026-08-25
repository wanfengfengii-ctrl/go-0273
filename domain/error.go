package domain

import "fmt"

// BusinessError is a stable, structured rejection carrying an error code and a
// canonically sortable list of reasons. It is the single error type returned
// across service and HTTP boundaries so that tests assert on code and reasons,
// never on protocol details.
type BusinessError struct {
	Code    ErrorCode
	Message string
	Reasons []Reason
}

// Error implements the error interface.
func (e *BusinessError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewError builds a BusinessError with no reasons.
func NewError(code ErrorCode, message string) *BusinessError {
	return &BusinessError{Code: code, Message: message}
}

// WithReason appends a reason and returns the error for chaining.
func (e *BusinessError) WithReason(r Reason) *BusinessError {
	e.Reasons = append(e.Reasons, r)
	return e
}

// WithReasons appends multiple reasons and returns the error for chaining.
func (e *BusinessError) WithReasons(rs ...Reason) *BusinessError {
	e.Reasons = append(e.Reasons, rs...)
	return e
}

// AsBusinessError unwraps err into a BusinessError if possible, otherwise
// returns a generic internal error.
func AsBusinessError(err error) *BusinessError {
	if err == nil {
		return nil
	}
	if be, ok := err.(*BusinessError); ok {
		return be
	}
	return &BusinessError{Code: CodeInvalidInput, Message: err.Error()}
}

// SortStrings returns a sorted copy of a string slice (stable, ascending).
func SortStrings(in []string) []string {
	out := append([]string(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
