package service

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/example/cs-pulse/backend/internal/model"
)

type NewsHeat struct {
	Freshness float64  `json:"freshness"`
	Topics    float64  `json:"topics"`
	Reading   float64  `json:"reading"`
	Reads     int      `json:"reads"`
	Matches   []string `json:"matches"`
}

type RankedArticle struct {
	model.Article
	Category  string   `json:"category"`
	HeatScore float64  `json:"heatScore"`
	Heat      NewsHeat `json:"heat"`
}

// RankNews is a site estimate, not an upstream audience measurement. Signals
// share one scale across categories: freshness / 55, topics / 30, reads / 15.
// Topic rank is compared within each platform; their raw heat values are not
// comparable. Cached trends older than two hours cannot boost an article.
func RankNews(articles []model.Article, trends []model.Trend, reads map[uint]int, now time.Time) []RankedArticle {
	ranked := make([]RankedArticle, 0, len(articles))
	seen := make(map[uint]bool)
	for _, article := range articles {
		if seen[article.ID] || article.PublishedAt.IsZero() || article.PublishedAt.After(now.Add(5*time.Minute)) {
			continue
		}
		seen[article.ID] = true
		category := "cs2"
		if article.Source == "bbc" {
			category = "world"
		}
		ageHours := math.Max(0, now.Sub(article.PublishedAt).Hours())
		freshness := 55 * math.Pow(.5, ageHours/24)
		reading := 15 * (1 - math.Exp(-float64(reads[article.ID])/10)) * math.Pow(.5, ageHours/72)
		best := map[string]float64{}
		matches := map[string]string{}
		for _, trend := range trends {
			if trend.Rank < 1 || trend.FetchedAt.IsZero() || now.Sub(trend.FetchedAt) > 2*time.Hour || trend.FetchedAt.After(now.Add(5*time.Minute)) {
				continue
			}
			if trend.Source != "baidu" && trend.Source != "weibo" {
				continue
			}
			confidence := math.Max(topicConfidence(article.Title+" "+article.TitleZH, trend.Keyword), .6*topicConfidence(article.Summary+" "+article.SummaryZH, trend.Keyword))
			strength := confidence / math.Log2(float64(trend.Rank)+1)
			if strength > best[trend.Source] {
				best[trend.Source] = strength
				matches[trend.Source] = trend.Keyword
			}
		}
		topics := 30 * math.Min(1, math.Max(best["baidu"], best["weibo"])+.25*math.Min(best["baidu"], best["weibo"]))
		labels := make([]string, 0, 2)
		for _, source := range []string{"baidu", "weibo"} {
			if label := matches[source]; label != "" && (len(labels) == 0 || labels[0] != label) {
				labels = append(labels, label)
			}
		}
		ranked = append(ranked, RankedArticle{
			Article: article, Category: category, HeatScore: roundHeat(freshness + topics + reading),
			Heat: NewsHeat{Freshness: roundHeat(freshness), Topics: roundHeat(topics), Reading: roundHeat(reading), Reads: reads[article.ID], Matches: labels},
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].HeatScore != ranked[j].HeatScore {
			return ranked[i].HeatScore > ranked[j].HeatScore
		}
		if !ranked[i].PublishedAt.Equal(ranked[j].PublishedAt) {
			return ranked[i].PublishedAt.After(ranked[j].PublishedAt)
		}
		return ranked[i].ID > ranked[j].ID
	})
	return ranked
}

func roundHeat(value float64) float64 { return math.Round(value*10) / 10 }

// Match explicit shared phrases conservatively. Three consecutive Han
// characters or a non-generic Latin word are required; lone characters and
// common two-character words cannot turn unrelated articles into hot news.
func topicConfidence(text, topic string) float64 {
	text = strings.ToLower(text)
	topic = strings.ToLower(topic)
	if len([]rune(topic)) >= 3 && strings.Contains(text, topic) {
		return 1
	}
	confidence := 0.0
	hanRuns := strings.FieldsFunc(topic, func(r rune) bool { return !unicode.Is(unicode.Han, r) })
	for _, run := range hanRuns {
		runes := []rune(run)
		for size := min(8, len(runes)); size >= 3; size-- {
			for start := 0; start+size <= len(runes); start++ {
				if strings.Contains(text, string(runes[start:start+size])) {
					confidence = math.Max(confidence, float64(size)/math.Max(4, math.Min(8, float64(len(runes)))))
				}
			}
		}
	}
	words := strings.FieldsFunc(topic, func(r rune) bool { return !((r >= 'a' && r <= 'z') || unicode.IsDigit(r)) })
	textWords := " " + strings.Join(strings.FieldsFunc(text, func(r rune) bool { return !((r >= 'a' && r <= 'z') || unicode.IsDigit(r)) }), " ") + " "
	stopWords := " the and for with from that this have has was will are its new more news world says said report update live "
	for _, word := range words {
		if len(word) < 3 || strings.Contains(stopWords, " "+word+" ") {
			continue
		}
		if strings.Contains(textWords, " "+word+" ") {
			confidence = math.Max(confidence, .65)
		}
	}
	return math.Min(1, confidence)
}
