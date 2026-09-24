package collector

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/example/cs-pulse/backend/internal/model"
)

const (
	baiduTrendsURL     = "https://top.baidu.com/board?tab=realtime"
	weiboAggregatorURL = "https://uapis.cn/api/v1/misc/hotboard?type=weibo"
)

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
	response, err := c.client.Get(weiboAggregatorURL)
	if err != nil {
		return nil, fmt.Errorf("weibo hot search aggregator: %w", err)
	}
	defer closeResponseBody(response.Body)
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weibo hot search aggregator returned %s", response.Status)
	}
	var payload struct {
		Type       string `json:"type"`
		UpdateTime string `json:"update_time"`
		List       []struct {
			Index    int    `json:"index"`
			Title    string `json:"title"`
			URL      string `json:"url"`
			HotValue string `json:"hot_value"`
		} `json:"list"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode weibo hot search aggregator: %w", err)
	}
	if payload.Type != "weibo" {
		return nil, fmt.Errorf("weibo hot search aggregator returned unexpected platform")
	}
	updatedAt, err := time.Parse(time.RFC3339, payload.UpdateTime)
	if err != nil || time.Since(updatedAt) > 2*time.Hour {
		return nil, fmt.Errorf("weibo hot search aggregator has no recent update")
	}
	rows := make([]model.Trend, 0, 30)
	for _, item := range payload.List {
		absolute, err := url.Parse(strings.TrimSpace(item.URL))
		if err != nil || absolute.Scheme != "https" || absolute.Hostname() != "s.weibo.com" || strings.TrimSpace(item.Title) == "" {
			continue
		}
		rank := item.Index
		if rank < 1 {
			rank = len(rows) + 1
		}
		rows = append(rows, model.Trend{
			Source: "weibo", Provider: "uapis.cn", Rank: rank, Keyword: strings.TrimSpace(item.Title),
			Heat: parseHeat(item.HotValue), SourceURL: absolute.String(), FetchedAt: updatedAt.UTC(),
		})
		if len(rows) == 30 {
			break
		}
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("weibo hot search page contained no ranking rows")
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
	digits := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, value)
	result, _ := strconv.ParseInt(digits, 10, 64)
	return result
}
