package safetrade

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
)

type Timestamp struct {
	time.Time
}

const unixMilliThreshold = 1e12

var timestampLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999Z0700",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999",
	time.DateOnly,
}

func (t *Timestamp) UnmarshalJSON(data []byte) error {
	parsed, err := parseTimestamp(data)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

func (t Timestamp) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return t.Time.MarshalJSON()
}

func parseTimestamp(data []byte) (time.Time, error) {
	if string(data) == "null" {
		return time.Time{}, nil
	}
	if len(data) == 0 || data[0] != '"' {
		return parseUnix(string(data))
	}

	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return time.Time{}, fmt.Errorf("safetrade: invalid timestamp %s: %w", data, err)
	}
	if text == "" {
		return time.Time{}, nil
	}
	if isNumber(text) {
		return parseUnix(text)
	}
	for _, layout := range timestampLayouts {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("safetrade: unsupported timestamp %q", text)
}

func parseUnix(text string) (time.Time, error) {
	if n, err := strconv.ParseInt(text, 10, 64); err == nil {
		if n == 0 {
			return time.Time{}, nil
		}
		if n >= unixMilliThreshold || n <= -unixMilliThreshold {
			return time.UnixMilli(n).UTC(), nil
		}
		return time.Unix(n, 0).UTC(), nil
	}

	value, err := decimal.NewFromString(text)
	if err != nil {
		return time.Time{}, fmt.Errorf("safetrade: invalid timestamp %q", text)
	}
	if value.IsZero() {
		return time.Time{}, nil
	}
	nanosPerUnit := decimal.NewFromInt(int64(time.Second))
	if value.Abs().GreaterThanOrEqual(decimal.NewFromInt(unixMilliThreshold)) {
		nanosPerUnit = decimal.NewFromInt(int64(time.Millisecond))
	}
	nanos := value.Mul(nanosPerUnit).Round(0)
	if nanos.GreaterThan(maxUnixNanos) || nanos.LessThan(minUnixNanos) {
		return time.Time{}, fmt.Errorf("safetrade: timestamp %q out of range", text)
	}
	return time.Unix(0, nanos.IntPart()).UTC(), nil
}

var (
	maxUnixNanos = decimal.NewFromInt(math.MaxInt64)
	minUnixNanos = decimal.NewFromInt(math.MinInt64)
)

func isNumber(text string) bool {
	_, err := decimal.NewFromString(text)
	return err == nil
}
