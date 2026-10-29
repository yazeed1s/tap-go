package tap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// FlexNumber is a number that Tap sends either as a JSON number or as a
// numeric string. Timestamps are the common case: a token's "created" is
// 1698392202943 in one response and "1698392202943" in another.
//
// It holds the number's text exactly as received.
type FlexNumber string

// UnmarshalJSON accepts a JSON number, a string, or null.
func (n *FlexNumber) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	switch {
	case bytes.Equal(data, []byte("null")):
		*n = ""
	case len(data) > 0 && data[0] == '"':
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*n = FlexNumber(s)
	default:
		var num json.Number
		if err := json.Unmarshal(data, &num); err != nil {
			return fmt.Errorf("tap: FlexNumber: %w", err)
		}
		*n = FlexNumber(num)
	}
	return nil
}

// MarshalJSON writes a JSON number when the value is numeric and a string
// otherwise.
func (n FlexNumber) MarshalJSON() ([]byte, error) {
	if n == "" {
		return []byte("null"), nil
	}
	if _, err := strconv.ParseFloat(string(n), 64); err == nil {
		return []byte(n), nil
	}
	return json.Marshal(string(n))
}

// String returns the number as received.
func (n FlexNumber) String() string { return string(n) }

// Int64 parses the number as an integer.
func (n FlexNumber) Int64() (int64, error) { return strconv.ParseInt(string(n), 10, 64) }

// Float64 parses the number as a float.
func (n FlexNumber) Float64() (float64, error) { return strconv.ParseFloat(string(n), 64) }

// Time reads the number as a Unix timestamp in milliseconds, the format Tap
// uses for created, updated, and similar fields.
func (n FlexNumber) Time() (time.Time, error) {
	ms, err := n.Int64()
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(ms), nil
}
