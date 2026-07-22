package extend

import (
	"fmt"
	"sort"
	"time"

	"github.com/injoyai/tdx/protocol"
)

const THSQFQCloseTailCutoffHour = 15
const THSQFQCloseTailCutoffMinute = 10

var shanghaiLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type THSQFQTailResult struct {
	Klines      []*Kline
	Source      string
	VerifiedBy  string
	TradeDate   string
	CurrentDate bool
}

// CompleteTHSQFQDailyTail verifies the last completed qfq daily bar against
// TDX raw data and appends the strictly validated today.js bar when all.js
// has not published the completed tail yet.
func CompleteTHSQFQDailyTail(
	all []*Kline,
	raw []*protocol.Kline,
	fetchToday func() (*Kline, error),
	now time.Time,
) (*THSQFQTailResult, error) {
	if len(all) == 0 {
		return nil, fmt.Errorf("同花顺前复权 all.js 数据为空")
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("TDX 原始日K线数据为空，无法核验同花顺前复权尾部")
	}
	if err := validateUniqueKlineDates(all); err != nil {
		return nil, err
	}
	if err := validateUniqueRawDates(raw); err != nil {
		return nil, err
	}

	qfq := append([]*Kline(nil), all...)
	sort.Slice(qfq, func(i, j int) bool { return qfq[i].Date < qfq[j].Date })
	rawBars := append([]*protocol.Kline(nil), raw...)
	sort.Slice(rawBars, func(i, j int) bool { return rawBars[i].Time.Before(rawBars[j].Time) })

	nowShanghai := now.In(shanghaiLocation)
	today := dateKey(nowShanghai)
	cutoff := time.Date(
		nowShanghai.Year(), nowShanghai.Month(), nowShanghai.Day(),
		THSQFQCloseTailCutoffHour, THSQFQCloseTailCutoffMinute, 0, 0,
		shanghaiLocation,
	)
	rawLast := rawBars[len(rawBars)-1]
	rawLastDate := dateKey(rawLast.Time.In(shanghaiLocation))

	// TDX may expose an in-progress daily bar during the session. Never return
	// the matching THS row before the close-tail cutoff, even if all.js has it.
	if rawLastDate == today && nowShanghai.Before(cutoff) {
		qfq = filterKlinesBefore(qfq, today)
		if len(qfq) == 0 {
			return nil, fmt.Errorf("15:10 前同花顺前复权数据没有已完成历史日K线")
		}
		return &THSQFQTailResult{
			Klines:    qfq,
			Source:    "ths_all",
			TradeDate: dateKey(time.Unix(qfq[len(qfq)-1].Date, 0).In(shanghaiLocation)),
		}, nil
	}

	qfqLastDate := dateKey(time.Unix(qfq[len(qfq)-1].Date, 0).In(shanghaiLocation))
	if qfqLastDate > rawLastDate {
		return nil, fmt.Errorf("同花顺前复权尾部日期 %s 晚于 TDX 原始日线 %s", qfqLastDate, rawLastDate)
	}

	source := "ths_all"
	if qfqLastDate < rawLastDate {
		if fetchToday == nil {
			return nil, fmt.Errorf("同花顺 all.js 缺少 %s 且 today.js 获取器不可用", rawLastDate)
		}
		todayKline, err := fetchToday()
		if err != nil {
			return nil, fmt.Errorf("同花顺 all.js 缺少 %s，today.js 获取失败: %w", rawLastDate, err)
		}
		if todayKline == nil {
			return nil, fmt.Errorf("同花顺 all.js 缺少 %s，today.js 返回空数据", rawLastDate)
		}
		todayDate := dateKey(time.Unix(todayKline.Date, 0).In(shanghaiLocation))
		if todayDate != rawLastDate {
			return nil, fmt.Errorf("同花顺 today.js 日期 %s 与 TDX 原始日线 %s 不一致", todayDate, rawLastDate)
		}
		qfq = append(qfq, todayKline)
		source = "ths_today"
	}

	tail := qfq[len(qfq)-1]
	if err := verifyTailOHLC(tail, rawLast); err != nil {
		return nil, err
	}
	tail.Volume = rawLast.Volume
	tail.Amount = rawLast.Amount

	return &THSQFQTailResult{
		Klines:      qfq,
		Source:      source,
		VerifiedBy:  "tdx_raw_daily",
		TradeDate:   rawLastDate,
		CurrentDate: rawLastDate == today,
	}, nil
}

func validateUniqueKlineDates(klines []*Kline) error {
	seen := make(map[string]struct{}, len(klines))
	for _, kline := range klines {
		if kline == nil {
			return fmt.Errorf("同花顺前复权 all.js 包含空K线")
		}
		date := dateKey(time.Unix(kline.Date, 0).In(shanghaiLocation))
		if _, exists := seen[date]; exists {
			return fmt.Errorf("同花顺前复权 all.js 包含重复日期: %s", date)
		}
		seen[date] = struct{}{}
	}
	return nil
}

func validateUniqueRawDates(klines []*protocol.Kline) error {
	seen := make(map[string]struct{}, len(klines))
	for _, kline := range klines {
		if kline == nil {
			return fmt.Errorf("TDX 原始日线包含空K线")
		}
		date := dateKey(kline.Time.In(shanghaiLocation))
		if _, exists := seen[date]; exists {
			return fmt.Errorf("TDX 原始日线包含重复日期: %s", date)
		}
		seen[date] = struct{}{}
	}
	return nil
}

func filterKlinesBefore(klines []*Kline, date string) []*Kline {
	filtered := make([]*Kline, 0, len(klines))
	for _, kline := range klines {
		if dateKey(time.Unix(kline.Date, 0).In(shanghaiLocation)) < date {
			filtered = append(filtered, kline)
		}
	}
	return filtered
}

func verifyTailOHLC(qfq *Kline, raw *protocol.Kline) error {
	qfqDate := dateKey(time.Unix(qfq.Date, 0).In(shanghaiLocation))
	rawDate := dateKey(raw.Time.In(shanghaiLocation))
	if qfqDate != rawDate {
		return fmt.Errorf("同花顺前复权尾部日期 %s 与 TDX 原始日线 %s 不一致", qfqDate, rawDate)
	}
	if err := validateOHLC(qfq.Open, qfq.High, qfq.Low, qfq.Close); err != nil {
		return fmt.Errorf("%s 同花顺前复权尾部 OHLC 无效: %w", qfqDate, err)
	}
	if err := validateOHLC(raw.Open, raw.High, raw.Low, raw.Close); err != nil {
		return fmt.Errorf("%s TDX 原始日线 OHLC 无效: %w", rawDate, err)
	}
	if qfq.Open != raw.Open || qfq.High != raw.High || qfq.Low != raw.Low || qfq.Close != raw.Close {
		return fmt.Errorf(
			"%s 同花顺前复权与 TDX 原始日线 OHLC 不一致: THS=%d/%d/%d/%d TDX=%d/%d/%d/%d",
			qfqDate,
			qfq.Open, qfq.High, qfq.Low, qfq.Close,
			raw.Open, raw.High, raw.Low, raw.Close,
		)
	}
	return nil
}

func validateOHLC(open, high, low, close protocol.Price) error {
	if open <= 0 || high <= 0 || low <= 0 || close <= 0 {
		return fmt.Errorf("价格必须为正数")
	}
	if high < low || high < open || high < close || low > open || low > close {
		return fmt.Errorf("OHLC 关系不成立")
	}
	return nil
}

func dateKey(value time.Time) string {
	return value.Format("2006-01-02")
}
