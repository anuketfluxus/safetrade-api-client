package safetrade_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/anuketfluxus/safetrade-api-client"
)

func TestTimestampUnmarshalJSON(t *testing.T) {
	t.Parallel()

	utc := func(year int, month time.Month, day, hour, minute, sec, nsec int) time.Time {
		return time.Date(year, month, day, hour, minute, sec, nsec, time.UTC)
	}
	tests := []struct {
		name string
		json string
		want time.Time
	}{
		{name: "null", json: `null`, want: time.Time{}},
		{name: "empty string", json: `""`, want: time.Time{}},
		{name: "RFC 3339", json: `"2026-10-26T18:11:41Z"`, want: utc(2026, 10, 26, 18, 11, 41, 0)},
		{name: "RFC 3339 nanoseconds", json: `"2026-10-26T18:11:41.123456789Z"`, want: utc(2026, 10, 26, 18, 11, 41, 123456789)},
		{name: "RFC 3339 offset", json: `"2026-10-27T01:11:41+07:00"`, want: utc(2026, 10, 26, 18, 11, 41, 0)},
		{name: "offset without colon", json: `"2026-10-27T01:11:41.5+0700"`, want: utc(2026, 10, 26, 18, 11, 41, 500000000)},
		{name: "no zone", json: `"2026-10-26T18:11:41.25"`, want: utc(2026, 10, 26, 18, 11, 41, 250000000)},
		{name: "space separator", json: `"2026-10-26 18:11:41"`, want: utc(2026, 10, 26, 18, 11, 41, 0)},
		{name: "space separator with zone", json: `"2026-10-26 18:11:41Z"`, want: utc(2026, 10, 26, 18, 11, 41, 0)},
		{name: "date", json: `"2026-10-26"`, want: utc(2026, 10, 26, 0, 0, 0, 0)},
		{name: "unix seconds", json: `1691239429`, want: utc(2023, 8, 5, 12, 43, 49, 0)},
		{name: "unix milliseconds", json: `1691239429123`, want: utc(2023, 8, 5, 12, 43, 49, 123000000)},
		{name: "unix seconds as string", json: `"1691239429"`, want: utc(2023, 8, 5, 12, 43, 49, 0)},
		{name: "fractional unix seconds", json: `1691239429.5`, want: utc(2023, 8, 5, 12, 43, 49, 500000000)},
		{name: "fractional unix milliseconds", json: `1691239429123.0`, want: utc(2023, 8, 5, 12, 43, 49, 123000000)},
		{name: "unix seconds with nanoseconds", json: `1691239429.123456789`, want: utc(2023, 8, 5, 12, 43, 49, 123456789)},
		{name: "unix seconds with exponent", json: `1.691239429e9`, want: utc(2023, 8, 5, 12, 43, 49, 0)},
		{name: "zero means no time", json: `0`, want: time.Time{}},
		{name: "zero string means no time", json: `"0"`, want: time.Time{}},
		{name: "fractional zero means no time", json: `0.0`, want: time.Time{}},
		{name: "first second after the epoch", json: `1`, want: utc(1970, 1, 1, 0, 0, 1, 0)},
	}
	for _, tt := range tests {
		var ts safetrade.Timestamp
		if err := json.Unmarshal([]byte(tt.json), &ts); err != nil {
			t.Errorf("%s: Unmarshal(%s) error = %v", tt.name, tt.json, err)
			continue
		}
		if !ts.Equal(tt.want) {
			t.Errorf("%s: Unmarshal(%s) = %v, want %v", tt.name, tt.json, ts.Time, tt.want)
		}
	}
}

func TestTimestampUnmarshalJSONFromUnixIsUTC(t *testing.T) {
	t.Parallel()

	var ts safetrade.Timestamp
	if err := json.Unmarshal([]byte(`1691239429`), &ts); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if ts.Location() != time.UTC {
		t.Errorf("Location() = %v, want UTC", ts.Location())
	}
}

func TestTimestampUnmarshalJSONRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		`"yesterday"`,
		`"2026-13-45T00:00:00Z"`,
		`"NaN"`,
		`true`,
		`{}`,
		`[]`,
		`1e400`,
	} {
		var ts safetrade.Timestamp
		if err := json.Unmarshal([]byte(input), &ts); err == nil {
			t.Errorf("Unmarshal(%s) = %v, want error", input, ts.Time)
		}
	}
}

func TestTimestampMarshalJSON(t *testing.T) {
	t.Parallel()

	zero, err := json.Marshal(safetrade.Timestamp{})
	if err != nil {
		t.Fatalf("Marshal(zero) error = %v", err)
	}
	if string(zero) != "null" {
		t.Errorf("Marshal(zero) = %s, want null", zero)
	}

	original := safetrade.Timestamp{Time: time.Date(2026, 10, 26, 18, 11, 41, 123000000, time.UTC)}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	if want := `"2026-10-26T18:11:41.123Z"`; string(encoded) != want {
		t.Errorf("Marshal = %s, want %s", encoded, want)
	}
	var decoded safetrade.Timestamp
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if !decoded.Equal(original.Time) {
		t.Errorf("round trip = %v, want %v", decoded.Time, original.Time)
	}
}
