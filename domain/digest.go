package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// Digest computes a stable, canonical SHA-256 digest of a request payload so
// that idempotency records can compare request contents independently of JSON
// key order or formatting. Map keys are sorted and structs are marshalled by
// encoding/json, giving a deterministic byte stream for the same logical input.
func Digest(v any) string {
	h := sha256.New()
	enc := json.NewEncoder(h)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(canonical(v))
	return hex.EncodeToString(h.Sum(nil))
}

// canonical deep-normalizes a value so that every map is sorted by key and
// every nested value is recursively normalized before hashing.
func canonical(v any) any {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(t))
		for _, k := range keys {
			out[k] = canonical(t[k])
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = canonical(e)
		}
		return out
	default:
		return v
	}
}
