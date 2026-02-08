package api

import (
	"encoding/json/v2"
	"fmt"
	"strconv"
	"time"
)

// FlexTime is a time type that can unmarshal from either:
// - RFC3339 string: "2024-01-15T10:30:00Z"
// - Epoch milliseconds (number): 1705314600000
// - Epoch milliseconds (string): "1705314600000"
//
// It always marshals to RFC3339 format for consistency.
type FlexTime struct {
	time.Time
}

// UnmarshalJSON handles flexible time parsing from JSON.
func (ft *FlexTime) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		// Try RFC3339 first
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			ft.Time = t
			return nil
		}
		// Try RFC3339Nano
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			ft.Time = t
			return nil
		}
		// Try as epoch milliseconds string
		if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
			ft.Time = time.UnixMilli(ms)
			return nil
		}
		return fmt.Errorf("cannot parse time string: %s", s)
	}

	// Try as number (epoch milliseconds)
	var ms int64
	if err := json.Unmarshal(data, &ms); err == nil {
		ft.Time = time.UnixMilli(ms)
		return nil
	}

	// Try as float (some JSON encoders use float for large numbers)
	var msFloat float64
	if err := json.Unmarshal(data, &msFloat); err == nil {
		ft.Time = time.UnixMilli(int64(msFloat))
		return nil
	}

	return fmt.Errorf("cannot unmarshal %s into FlexTime", string(data))
}

// MarshalJSON outputs time in RFC3339 format.
func (ft FlexTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(ft.Format(time.RFC3339))
}

// ToTime returns the underlying time.Time value.
func (ft FlexTime) ToTime() time.Time {
	return ft.Time
}
