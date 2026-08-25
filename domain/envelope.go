package domain

import "encoding/json"

// Envelope is the canonical response body returned by every business write and
// read. It carries a stable business code, an optional message, a canonically
// sorted reason list, and optional payload data. The HTTP layer derives the
// protocol status from Code and otherwise forwards the body verbatim, so tests
// assert on Code and Reasons rather than protocol details.
type Envelope struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Reasons []Reason  `json:"reasons"`
	Data    any       `json:"data,omitempty"`
}

// OKEnvelope builds a success envelope carrying data.
func OKEnvelope(data any) Envelope {
	return Envelope{Code: CodeOK, Message: "ok", Reasons: []Reason{}, Data: data}
}

// ErrorEnvelope builds a rejection envelope with canonically sorted reasons.
func ErrorEnvelope(code ErrorCode, message string, reasons []Reason) Envelope {
	return Envelope{Code: code, Message: message, Reasons: SortReasons(reasons)}
}

// Bytes serializes the envelope with canonically sorted reasons.
func (e Envelope) Bytes() ([]byte, error) {
	e.Reasons = SortReasons(e.Reasons)
	return json.Marshal(e)
}
