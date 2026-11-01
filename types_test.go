package safetrade_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/anuketfluxus/safetrade-api-client"
)

func TestPriceLevelUnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		json   string
		price  string
		amount string
	}{
		{json: `["67010.5","0.25"]`, price: "67010.5", amount: "0.25"},
		{json: `[67010.5,0.25]`, price: "67010.5", amount: "0.25"},
		{json: `["0.00000001",1e3]`, price: "0.00000001", amount: "1000"},
		{json: `["1","2",3]`, price: "1", amount: "2"},
	}
	for _, tt := range tests {
		var level safetrade.PriceLevel
		if err := json.Unmarshal([]byte(tt.json), &level); err != nil {
			t.Errorf("Unmarshal(%s) error = %v", tt.json, err)
			continue
		}
		expectDecimal(t, tt.json+" price", level.Price, tt.price)
		expectDecimal(t, tt.json+" amount", level.Amount, tt.amount)
	}
}

func TestPriceLevelUnmarshalJSONRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	for _, input := range []string{`{}`, `"1"`, `[]`, `["1"]`, `["abc","1"]`, `["1",true]`} {
		var level safetrade.PriceLevel
		if err := json.Unmarshal([]byte(input), &level); err == nil {
			t.Errorf("Unmarshal(%s) = %+v, want error", input, level)
		}
	}
}

func TestPriceLevelMarshalJSON(t *testing.T) {
	t.Parallel()

	level := safetrade.PriceLevel{Price: dec(t, "67010.5"), Amount: dec(t, "0.25")}
	encoded, err := json.Marshal(level)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	if want := `["67010.5","0.25"]`; string(encoded) != want {
		t.Errorf("Marshal = %s, want %s", encoded, want)
	}
}

func TestKLineUnmarshalJSONRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   string
		wantErr string
	}{
		{input: `{}`, wantErr: "k-line must be"},
		{input: `[1761436800,1,2,3,4]`, wantErr: "k-line must have 6 elements, got 5"},
		{input: `["yesterday",1,2,3,4,5]`, wantErr: "k-line time"},
		{input: `[1761436800,1,2,"x",4,5]`, wantErr: "k-line low"},
	}
	for _, tt := range tests {
		var kline safetrade.KLine
		err := json.Unmarshal([]byte(tt.input), &kline)
		if err == nil {
			t.Errorf("Unmarshal(%s) = %+v, want error", tt.input, kline)
			continue
		}
		if !strings.Contains(err.Error(), tt.wantErr) {
			t.Errorf("Unmarshal(%s) error = %q, want it to contain %q", tt.input, err, tt.wantErr)
		}
	}
}

func TestKLineJSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := safetrade.KLine{
		Time:   safetrade.Timestamp{Time: time.Unix(1761436800, 0).UTC()},
		Open:   dec(t, "67000"),
		High:   dec(t, "67100.5"),
		Low:    dec(t, "66900"),
		Close:  dec(t, "67050"),
		Volume: dec(t, "12.5"),
	}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	if want := `[1761436800,"67000","67100.5","66900","67050","12.5"]`; string(encoded) != want {
		t.Errorf("Marshal = %s, want %s", encoded, want)
	}

	var decoded safetrade.KLine
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if !decoded.Time.Equal(original.Time.Time) || !decoded.High.Equal(original.High) || !decoded.Volume.Equal(original.Volume) {
		t.Errorf("round trip = %+v, want %+v", decoded, original)
	}
}

func TestOrderDecodesNullFieldsAsZero(t *testing.T) {
	t.Parallel()

	var order safetrade.Order
	err := json.Unmarshal([]byte(`{"id":1,"price":null,"stop_price":null,"triggered_at":null,"created_at":"2026-10-26T18:11:41Z"}`), &order)
	if err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if !order.Price.IsZero() || !order.StopPrice.IsZero() || !order.TriggeredAt.IsZero() {
		t.Errorf("null fields decoded to %v, %v, %v; want zero values", order.Price, order.StopPrice, order.TriggeredAt.Time)
	}
	expectTime(t, "CreatedAt", order.CreatedAt, "2026-10-26T18:11:41Z")
}
