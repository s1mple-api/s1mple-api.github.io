package collector

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/example/cs-pulse/backend/internal/model"
)

const (
	baiduTrendsURL = "https://top.baidu.com/board?tab=realtime"
	hotboardURL    = "https://uapis.cn/api/v1/misc/hotboard"
)

var heatPattern = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*([亿万wk]?)`)

type TrendCollector struct{ client *http.Client }

func NewTrendCollector() *TrendCollector {
	return &TrendCollector{client: &http.Client{Timeout: 20 * time.Second}}
}

func (c *TrendCollector) FetchBaidu() ([]model.Trend, error) {
	doc, _, err := c.getPage(baiduTrendsURL)
	if err != nil {
		return nil, fmt.Errorf("baidu hot search: %w", err)
	}
	rows := make([]model.Trend, 0, 30)
	now := time.Now().UTC()
	doc.Find(`[class*="category-wrap_"]`).EachWithBreak(func(_ int, card *goquery.Selection) bool {
		link := card.Find(`a[class*="title_"]`).First()
		keyword := cleanText(link.Find(`.c-single-text-ellipsis`).First().Text())
		href := strings.TrimSpace(link.AttrOr("href", ""))
		parsed, parseErr := url.Parse(href)
		if keyword == "" || parseErr != nil || parsed.Scheme != "https" || !strings.HasSuffix(parsed.Hostname(), ".baidu.com") {
			return true
		}
		rows = append(rows, model.Trend{
			Source: "baidu", Provider: "top.baidu.com", Rank: len(rows) + 1, Keyword: keyword,
			Heat:      parseHeat(card.Find(`[class*="hot-index_"]`).First().Text()),
			Tag:       cleanText(link.Find(`[class*="hot-tag_"]`).First().Text()),
			SourceURL: href, FetchedAt: now,
		})
		return len(rows) < 30
	})
	if len(rows) == 0 {
		return nil, fmt.Errorf("baidu hot search page contained no ranking rows")
	}
	return rows, nil
}

func (c *TrendCollector) FetchWeibo() ([]model.Trend, error) {
	return c.fetchHotboard("weibo")
}

func (c *TrendCollector) FetchCommunity(source string) ([]model.Trend, error) {
	if !model.IsCommunitySource(source) {
		return nil, fmt.Errorf("unsupported community source %q", source)
	}
	return c.fetchHotboard(source)
}

func (c *TrendCollector) fetchHotboard(source string) ([]model.Trend, error) {
	response, err := c.client.Get(hotboardURL + "?type=" + url.QueryEscape(source))
	if err != nil {
		return nil, fmt.Errorf("%s hotboard aggregator: %w", source, err)
	}
	defer closeResponseBody(response.Body)
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s hotboard aggregator returned %s", source, response.Status)
	}
	rows, err := decodeHotboard(response.Body, source, time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("%s hotboard aggregator: %w", source, err)
	}
	return rows, nil
}

func decodeHotboard(body io.Reader, source string, now time.Time) ([]model.Trend, error) {
	hosts := map[string]string{"weibo": "s.weibo.com", "tieba": "tieba.baidu.com", "hupu": "bbs.hupu.com", "bilibili": "www.bilibili.com", "xiaohongshu": "www.xiaohongshu.com", "douyin": "www.douyin.com"}
	host, supported := hosts[source]
	if !supported {
		return nil, fmt.Errorf("unsupported hotboard source %q", source)
	}
	var payload struct {
		Type       string `json:"type"`
		UpdateTime string `json:"update_time"`
		List       []struct {
			Index    int             `json:"index"`
			Title    string          `json:"title"`
			URL      string          `json:"url"`
			HotValue string          `json:"hot_value"`
			Extra    json.RawMessage `json:"extra"`
		} `json:"list"`
	}
	if err := json.NewDecoder(io.LimitReader(body, 2<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if payload.Type != source {
		return nil, fmt.Errorf("unexpected platform")
	}
	updatedAt, err := time.Parse(time.RFC3339, payload.UpdateTime)
	if err != nil || now.Sub(updatedAt) > 2*time.Hour || updatedAt.After(now.Add(5*time.Minute)) {
		return nil, fmt.Errorf("no recent valid update time")
	}
	for i := range payload.List {
		if payload.List[i].Index < 1 {
			payload.List[i].Index = i + 1
		}
	}
	sort.SliceStable(payload.List, func(i, j int) bool { return payload.List[i].Index < payload.List[j].Index })
	rows := make([]model.Trend, 0, 30)
	seen := make(map[string]bool)
	for _, item := range payload.List {
		absolute, err := url.Parse(strings.TrimSpace(item.URL))
		keyword := cleanText(item.Title)
		if err != nil || absolute.Scheme != "https" || absolute.Host != host || absolute.User != nil || keyword == "" || len([]rune(keyword)) > 255 {
			continue
		}
		if seen[keyword] || seen[absolute.String()] {
			continue
		}
		seen[keyword], seen[absolute.String()] = true, true
		var extra struct {
			Description string `json:"desc"`
		}
		if len(item.Extra) > 0 {
			// Other platforms may use a different optional extra payload.
			_ = json.Unmarshal(item.Extra, &extra)
		}
		rows = append(rows, model.Trend{
			Source: source, Provider: "uapis.cn", Rank: item.Index, Keyword: keyword,
			Heat: parseHeat(item.HotValue), HeatLabel: truncateText(cleanText(item.HotValue), 100),
			Summary:   truncateText(cleanText(extra.Description), 4000),
			SourceURL: absolute.String(), FetchedAt: updatedAt.UTC(),
		})
		if len(rows) == 30 {
			break
		}
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("response contained no valid ranking rows")
	}
	return rows, nil
}

func (c *TrendCollector) getPage(pageURL string) (*goquery.Document, *url.URL, error) {
	req, err := http.NewRequest(http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, nil, err
	}
	response, err := c.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer closeResponseBody(response.Body)
	if response.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("returned %s", response.Status)
	}
	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		return nil, nil, err
	}
	return doc, response.Request.URL, nil
}

func parseHeat(value string) int64 {
	parts := heatPattern.FindStringSubmatch(strings.ReplaceAll(value, ",", ""))
	if len(parts) != 3 {
		return 0
	}
	valueNumber, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0
	}
	switch strings.ToLower(parts[2]) {
	case "亿":
		valueNumber *= 100_000_000
	case "万", "w":
		valueNumber *= 10_000
	case "k":
		valueNumber *= 1_000
	}
	if valueNumber >= float64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(math.Round(valueNumber))
}

func truncateText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
