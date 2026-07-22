package extend

import (
	"errors"
	"testing"
	"time"

	"github.com/injoyai/tdx/protocol"
)

func TestCompleteTHSQFQDailyTail(t *testing.T) {
	previous := testTHSKline("2026-07-21", 1200, 1230, 1190, 1210)
	current := testTHSKline("2026-07-22", 1210, 1250, 1200, 1240)
	rawPrevious := testRawKline("2026-07-21", 1200, 1230, 1190, 1210)
	rawCurrent := testRawKline("2026-07-22", 1210, 1250, 1200, 1240)

	tests := []struct {
		name        string
		all         []*Kline
		raw         []*protocol.Kline
		now         time.Time
		today       *Kline
		todayErr    error
		wantSource  string
		wantDate    string
		wantLen     int
		wantFetches int
		wantErr     bool
	}{
		{
			name:       "before cutoff excludes current date",
			all:        []*Kline{previous, current},
			raw:        []*protocol.Kline{rawPrevious, rawCurrent},
			now:        testShanghaiTime("2026-07-22 15:09"),
			wantSource: "ths_all",
			wantDate:   "2026-07-21",
			wantLen:    1,
		},
		{
			name:       "market close still excludes current date",
			all:        []*Kline{previous, current},
			raw:        []*protocol.Kline{rawPrevious, rawCurrent},
			now:        testShanghaiTime("2026-07-22 15:00"),
			wantSource: "ths_all",
			wantDate:   "2026-07-21",
			wantLen:    1,
		},
		{
			name:       "at cutoff uses complete all tail",
			all:        []*Kline{previous, current},
			raw:        []*protocol.Kline{rawPrevious, rawCurrent},
			now:        testShanghaiTime("2026-07-22 15:10"),
			wantSource: "ths_all",
			wantDate:   "2026-07-22",
			wantLen:    2,
		},
		{
			name:        "after cutoff appends today tail",
			all:         []*Kline{previous},
			raw:         []*protocol.Kline{rawPrevious, rawCurrent},
			now:         testShanghaiTime("2026-07-22 16:00"),
			today:       current,
			wantSource:  "ths_today",
			wantDate:    "2026-07-22",
			wantLen:     2,
			wantFetches: 1,
		},
		{
			name:       "weekend verifies latest closed day",
			all:        []*Kline{previous},
			raw:        []*protocol.Kline{rawPrevious},
			now:        testShanghaiTime("2026-07-25 10:00"),
			wantSource: "ths_all",
			wantDate:   "2026-07-21",
			wantLen:    1,
		},
		{
			name:        "today fetch failure is explicit",
			all:         []*Kline{previous},
			raw:         []*protocol.Kline{rawPrevious, rawCurrent},
			now:         testShanghaiTime("2026-07-22 16:00"),
			todayErr:    errors.New("timeout"),
			wantErr:     true,
			wantFetches: 1,
		},
		{
			name:        "today date mismatch fails",
			all:         []*Kline{previous},
			raw:         []*protocol.Kline{rawPrevious, rawCurrent},
			now:         testShanghaiTime("2026-07-22 16:00"),
			today:       testTHSKline("2026-07-21", 1210, 1250, 1200, 1240),
			wantErr:     true,
			wantFetches: 1,
		},
		{
			name:    "price mismatch fails",
			all:     []*Kline{previous, testTHSKline("2026-07-22", 1211, 1250, 1200, 1240)},
			raw:     []*protocol.Kline{rawPrevious, rawCurrent},
			now:     testShanghaiTime("2026-07-22 16:00"),
			wantErr: true,
		},
		{
			name:    "empty raw ohlc fails",
			all:     []*Kline{previous, current},
			raw:     []*protocol.Kline{rawPrevious, testRawKline("2026-07-22", 0, 0, 0, 0)},
			now:     testShanghaiTime("2026-07-22 16:00"),
			wantErr: true,
		},
		{
			name:    "duplicate all date fails",
			all:     []*Kline{previous, previous},
			raw:     []*protocol.Kline{rawPrevious},
			now:     testShanghaiTime("2026-07-25 10:00"),
			wantErr: true,
		},
		{
			name: "ex-right history does not compare older raw prices",
			all: []*Kline{
				testTHSKline("2026-07-20", 600, 620, 590, 610),
				previous,
			},
			raw: []*protocol.Kline{
				testRawKline("2026-07-20", 1200, 1240, 1180, 1220),
				rawPrevious,
			},
			now:        testShanghaiTime("2026-07-21 16:00"),
			wantSource: "ths_all",
			wantDate:   "2026-07-21",
			wantLen:    2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fetches := 0
			result, err := CompleteTHSQFQDailyTail(test.all, test.raw, func() (*Kline, error) {
				fetches++
				return test.today, test.todayErr
			}, test.now)
			if test.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %#v", result)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result.Source != test.wantSource || result.TradeDate != test.wantDate || len(result.Klines) != test.wantLen {
					t.Fatalf("unexpected result: %#v", result)
				}
				if result.VerifiedBy != "" && result.VerifiedBy != "tdx_raw_daily" {
					t.Fatalf("unexpected verifier: %q", result.VerifiedBy)
				}
			}
			if fetches != test.wantFetches {
				t.Fatalf("today fetch count=%d, want %d", fetches, test.wantFetches)
			}
		})
	}
}

func TestCompleteTHSQFQDailyTailUsesRawVolumeAndAmount(t *testing.T) {
	all := testTHSKline("2026-07-22", 1210, 1250, 1200, 1240)
	all.Volume = 1
	all.Amount = 2
	raw := testRawKline("2026-07-22", 1210, 1250, 1200, 1240)
	raw.Volume = 998877
	raw.Amount = 776655

	result, err := CompleteTHSQFQDailyTail(
		[]*Kline{all},
		[]*protocol.Kline{raw},
		nil,
		testShanghaiTime("2026-07-22 16:00"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Klines[0].Volume != raw.Volume || result.Klines[0].Amount != raw.Amount {
		t.Fatalf("tail did not use raw volume/amount: %#v", result.Klines[0])
	}
}

func testTHSKline(date string, open, high, low, close protocol.Price) *Kline {
	return &Kline{
		Code:  "SZ159786",
		Date:  testDate(date).Unix(),
		Open:  open,
		High:  high,
		Low:   low,
		Close: close,
	}
}

func testRawKline(date string, open, high, low, close protocol.Price) *protocol.Kline {
	return &protocol.Kline{
		Time:  testDate(date),
		Open:  open,
		High:  high,
		Low:   low,
		Close: close,
	}
}

func testDate(value string) time.Time {
	parsed, err := time.ParseInLocation("2006-01-02", value, shanghaiLocation)
	if err != nil {
		panic(err)
	}
	return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 15, 0, 0, 0, shanghaiLocation)
}

func testShanghaiTime(value string) time.Time {
	parsed, err := time.ParseInLocation("2006-01-02 15:04", value, shanghaiLocation)
	if err != nil {
		panic(err)
	}
	return parsed
}
