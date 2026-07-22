package extend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/injoyai/conv"
	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/protocol"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	UrlTHSDayKline         = "http://d.10jqka.com.cn/v6/line/hs_%s/0%d/all.js"
	UrlTHSTodayKline       = "http://d.10jqka.com.cn/v6/line/hs_%s/0%d/today.js"
	THS_BFQ          uint8 = 0 //不复权
	THS_QFQ          uint8 = 1 //前复权
	THS_HFQ          uint8 = 2 //后复权
)

var thsHTTPClient = &http.Client{Timeout: 8 * time.Second}

// GetTHSDayKlineFactorFull 增加计算复权因子
func GetTHSDayKlineFactorFull(code string, c *tdx.Client) ([3][]*Kline, []*THSFactor, error) {
	ks, err := GetTHSDayKlineFull(code, c)
	if err != nil {
		return [3][]*Kline{}, nil, err
	}
	mQPrice := make(map[int64]float64)
	for _, v := range ks[1] {
		mQPrice[v.Date] = v.Close.Float64()
	}
	mHPrice := make(map[int64]float64)
	for _, v := range ks[2] {
		mHPrice[v.Date] = v.Close.Float64()
	}
	fs := make([]*THSFactor, 0, len(ks[0]))
	for _, v := range ks[0] {
		fs = append(fs, &THSFactor{
			Date:    v.Date,
			QFactor: mQPrice[v.Date] / v.Close.Float64(),
			HFactor: mHPrice[v.Date] / v.Close.Float64(),
		})
	}
	return ks, fs, nil
}

/*
GetTHSDayKlineFull
获取[不复权,前复权,后复权]数据,并补充成交金额数据
前复权,和通达信对的上,和东方财富对不上
后复权,和通达信,东方财富都对不上
*/
func GetTHSDayKlineFull(code string, c *tdx.Client) ([3][]*Kline, error) {
	resp, err := c.GetKlineDayAll(code)
	if err != nil {
		return [3][]*Kline{}, err
	}
	mAmount := make(map[int64]protocol.Price)
	bfq := []*Kline(nil)
	for _, v := range resp.List {
		mAmount[v.Time.Unix()] = v.Amount
		bfq = append(bfq, &Kline{
			Code:   code,
			Date:   v.Time.Unix(),
			Open:   v.Open,
			High:   v.High,
			Low:    v.Low,
			Close:  v.Close,
			Volume: v.Volume,
			Amount: v.Amount,
		})
	}
	//前复权
	qfq, err := GetTHSDayKline(code, THS_QFQ)
	if err != nil {
		return [3][]*Kline{}, err
	}
	for i := range qfq {
		qfq[i].Amount = mAmount[qfq[i].Date]
	}
	//后复权
	hfq, err := GetTHSDayKline(code, THS_HFQ)
	if err != nil {
		return [3][]*Kline{}, err
	}
	for i := range hfq {
		hfq[i].Amount = mAmount[hfq[i].Date]
	}
	return [3][]*Kline{bfq, qfq, hfq}, nil
}

/*
GetTHSDayKline
前复权,和通达信对的上,和东方财富对不上
后复权,和通达信,东方财富都对不上
*/
func GetTHSDayKline(code string, _type uint8) ([]*Kline, error) {
	if _type != THS_BFQ && _type != THS_QFQ && _type != THS_HFQ {
		return nil, fmt.Errorf("数据类型错误,例如:不复权0或前复权1或后复权2")
	}

	code = protocol.AddPrefix(code)
	if len(code) != 8 {
		return nil, fmt.Errorf("股票代码错误,例如:SZ000001或000001")
	}

	u := fmt.Sprintf(UrlTHSDayKline, code[2:], _type)
	bs, err := fetchTHSJSONP(u)
	if err != nil {
		return nil, err
	}

	m := map[string]any{}
	err = json.Unmarshal(bs, &m)
	if err != nil {
		return nil, err
	}

	total := conv.Int(m["total"])
	sortYears := conv.Interfaces(m["sortYear"])
	priceFactor := conv.Float64(m["priceFactor"])
	prices := strings.Split(conv.String(m["price"]), ",")
	dates := strings.Split(conv.String(m["dates"]), ",")
	volumes := strings.Split(conv.String(m["volumn"]), ",")

	//好像到了22点,总数量会比实际多1
	if total == len(dates)+1 && total == len(volumes)+1 {
		total -= 1
	}
	//判断数量是否对应
	if total*4 != len(prices) || total != len(dates) || total != len(volumes) {
		return nil, fmt.Errorf("total=%d prices=%d dates=%d volumns=%d", total, len(prices), len(dates), len(volumes))
	}

	mYear := make(map[int][]string)
	index := 0
	for i, v := range sortYears {
		if ls := conv.Ints(v); len(ls) == 2 {
			year := conv.Int(ls[0])
			length := conv.Int(ls[1])
			if i == len(sortYears)-1 {
				mYear[year] = dates[index:]
				break
			}
			mYear[year] = dates[index : index+length]
			index += length
		}
	}

	ls := []*Kline(nil)
	i := 0
	nowYear := time.Now().In(shanghaiLocation).Year()
	for year := 1990; year <= nowYear; year++ {
		for _, d := range mYear[year] {
			x, err := time.Parse("0102", d)
			if err != nil {
				return nil, err
			}
			x = time.Date(year, x.Month(), x.Day(), 15, 0, 0, 0, shanghaiLocation)
			low := protocol.Price(math.Round(conv.Float64(prices[i*4+0]) * 1000 / priceFactor))
			ls = append(ls, &Kline{
				Code:   protocol.AddPrefix(code),
				Date:   x.Unix(),
				Open:   protocol.Price(math.Round(conv.Float64(prices[i*4+1])*1000/priceFactor)) + low,
				High:   protocol.Price(math.Round(conv.Float64(prices[i*4+2])*1000/priceFactor)) + low,
				Low:    low,
				Close:  protocol.Price(math.Round(conv.Float64(prices[i*4+3])*1000/priceFactor)) + low,
				Volume: (conv.Int64(volumes[i]) + 50) / 100,
			})
			i++
		}
	}

	return ls, nil
}

// GetTHSTodayKline obtains the current daily bar from the THS qfq route.
func GetTHSTodayKline(code string, _type uint8) (*Kline, error) {
	if _type != THS_BFQ && _type != THS_QFQ && _type != THS_HFQ {
		return nil, fmt.Errorf("数据类型错误,例如:不复权0或前复权1或后复权2")
	}

	code = protocol.AddPrefix(code)
	if len(code) != 8 {
		return nil, fmt.Errorf("股票代码错误,例如:SZ000001或000001")
	}

	u := fmt.Sprintf(UrlTHSTodayKline, code[2:], _type)
	bs, err := fetchTHSJSONP(u)
	if err != nil {
		return nil, err
	}
	return parseTHSTodayKline(bs, code)
}

func fetchTHSJSONP(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	/*
	 'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) '
	                      'Chrome/90.0.4430.212 Safari/537.36',
	        'Referer': 'http://stockpage.10jqka.com.cn/',
	        'DNT': '1',
	*/
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/89.0.4389.90 Safari/537.36 Edg/89.0.774.54")
	req.Header.Set("Referer", "http://stockpage.10jqka.com.cn/")
	req.Header.Set("DNT", "1")
	resp, err := thsHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("同花顺日K线HTTP状态异常: %s", resp.Status)
	}
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	bs, err = parseTHSJSONPBody(bs)
	if err != nil {
		return nil, err
	}
	return bs, nil
}

func parseTHSJSONPBody(bs []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(bs)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("同花顺日K线返回空响应")
	}
	open := bytes.IndexByte(trimmed, '(')
	close := bytes.LastIndexByte(trimmed, ')')
	if open < 0 || close <= open {
		return nil, fmt.Errorf("同花顺日K线返回无效JSONP: bytes=%d", len(trimmed))
	}
	payload := bytes.TrimSpace(trimmed[open+1 : close])
	if len(payload) == 0 {
		return nil, fmt.Errorf("同花顺日K线JSONP内容为空")
	}
	return payload, nil
}

func parseTHSTodayKline(bs []byte, code string) (*Kline, error) {
	payload := map[string]map[string]any{}
	if err := json.Unmarshal(bs, &payload); err != nil {
		return nil, err
	}
	key := "hs_" + code[2:]
	row, ok := payload[key]
	if !ok {
		return nil, fmt.Errorf("同花顺当日日K线缺少标的: %s", code)
	}

	dateText := conv.String(row["1"])
	day, err := time.ParseInLocation("20060102", dateText, time.FixedZone("Asia/Shanghai", 8*60*60))
	if err != nil {
		return nil, fmt.Errorf("同花顺当日日K线日期无效: %q", dateText)
	}
	open, err := parseTHSMilliPrice(row["7"])
	if err != nil {
		return nil, fmt.Errorf("同花顺当日日K线开盘价无效: %w", err)
	}
	high, err := parseTHSMilliPrice(row["8"])
	if err != nil {
		return nil, fmt.Errorf("同花顺当日日K线最高价无效: %w", err)
	}
	low, err := parseTHSMilliPrice(row["9"])
	if err != nil {
		return nil, fmt.Errorf("同花顺当日日K线最低价无效: %w", err)
	}
	closePrice, err := parseTHSMilliPrice(row["11"])
	if err != nil {
		return nil, fmt.Errorf("同花顺当日日K线收盘价无效: %w", err)
	}
	if high < low || high < open || high < closePrice || low > open || low > closePrice {
		return nil, fmt.Errorf("同花顺当日日K线OHLC关系无效")
	}

	return &Kline{
		Code:  protocol.AddPrefix(code),
		Date:  time.Date(day.Year(), day.Month(), day.Day(), 15, 0, 0, 0, day.Location()).Unix(),
		Open:  open,
		High:  high,
		Low:   low,
		Close: closePrice,
	}, nil
}

func parseTHSMilliPrice(value any) (protocol.Price, error) {
	text := strings.TrimSpace(conv.String(value))
	if text == "" {
		return 0, fmt.Errorf("价格为空")
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed <= 0 {
		return 0, fmt.Errorf("价格=%q", text)
	}
	return protocol.Price(math.Round(parsed * 1000)), nil
}
