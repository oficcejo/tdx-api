package extend

import (
	"bytes"
	"testing"
	"time"

	"github.com/injoyai/tdx/protocol"
)

func TestParseTHSJSONPBody(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{name: "empty", body: "", wantErr: true},
		{name: "whitespace", body: " \n\t", wantErr: true},
		{name: "missing opener", body: `callback{"status":"ok"})`, wantErr: true},
		{name: "missing closer", body: `callback({"status":"ok"}`, wantErr: true},
		{name: "empty payload", body: "callback()", wantErr: true},
		{
			name: "valid with trailing semicolon",
			body: "  callback({\"status\":\"ok\"});\n",
			want: `{"status":"ok"}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseTHSJSONPBody([]byte(test.body))
			if test.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, []byte(test.want)) {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseTHSTodayKline(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    *Kline
		wantErr bool
	}{
		{
			name: "valid three decimal prices",
			body: `{"hs_159786":{"1":"20260722","7":"1.237","8":"1.250","9":"1.220","11":"1.245","13":"123456","19":"789012"}}`,
			want: &Kline{
				Code:  "SZ159786",
				Date:  time.Date(2026, 7, 22, 15, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60)).Unix(),
				Open:  protocol.Price(1237),
				High:  protocol.Price(1250),
				Low:   protocol.Price(1220),
				Close: protocol.Price(1245),
			},
		},
		{
			name:    "missing symbol",
			body:    `{"hs_510300":{"1":"20260722","7":"4.0","8":"4.1","9":"3.9","11":"4.0"}}`,
			wantErr: true,
		},
		{
			name:    "missing close",
			body:    `{"hs_159786":{"1":"20260722","7":"1.237","8":"1.250","9":"1.220"}}`,
			wantErr: true,
		},
		{
			name:    "invalid ohlc",
			body:    `{"hs_159786":{"1":"20260722","7":"1.237","8":"1.230","9":"1.220","11":"1.245"}}`,
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseTHSTodayKline([]byte(test.body), "SZ159786")
			if test.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if *got != *test.want {
				t.Fatalf("got %#v, want %#v", got, test.want)
			}
			if got.Volume != 0 || got.Amount != 0 {
				t.Fatalf("today.js volume/amount must not be trusted: %#v", got)
			}
		})
	}
}
