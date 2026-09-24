package service

import (
	"testing"
	"time"

	"github.com/example/cs-pulse/backend/internal/model"
)

func TestNewsHeatUsesComparableCategoryScores(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	rows := RankNews([]model.Article{
		{ID: 1, Source: "hltv", Title: "Team wins final", PublishedAt: now.Add(-time.Hour)},
		{ID: 2, Source: "bbc", Title: "Local news report", PublishedAt: now.Add(-time.Hour)},
	}, nil, map[uint]int{1: 3, 2: 3}, now)
	if len(rows) != 2 || rows[0].HeatScore != rows[1].HeatScore {
		t.Fatalf("category-dependent scores: %+v", rows)
	}
}

func TestNewsHeatMatchesTopicsAndExpiresCachedTrends(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	article := model.Article{ID: 1, Source: "bbc", TitleZH: "OpenAI 发布新模型", PublishedAt: now}
	trend := model.Trend{Source: "weibo", Keyword: "OpenAI 新模型", Rank: 1, FetchedAt: now}
	current := RankNews([]model.Article{article}, []model.Trend{trend}, nil, now)[0]
	trend.FetchedAt = now.Add(-3 * time.Hour)
	stale := RankNews([]model.Article{article}, []model.Trend{trend}, nil, now)[0]
	if current.Heat.Topics <= 0 || stale.Heat.Topics != 0 {
		t.Fatalf("current/stale topics = %v/%v", current.Heat, stale.Heat)
	}
	if topicConfidence("中国队赢得比赛", "中国游客游览城市") != 0 {
		t.Fatal("a generic two-character match must not boost a story")
	}
}

func TestNewsHeatDecaysOldPopularArticlesAndDeduplicates(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	fresh := model.Article{ID: 1, Source: "hltv", PublishedAt: now.Add(-time.Hour)}
	rows := RankNews([]model.Article{fresh, fresh, {ID: 2, Source: "bbc", PublishedAt: now.Add(-7 * 24 * time.Hour)}}, nil, map[uint]int{2: 100000}, now)
	if len(rows) != 2 || rows[0].ID != 1 {
		t.Fatalf("old lifetime traffic dominated current news: %+v", rows)
	}
	if rows[0].HeatScore > 100 || rows[1].HeatScore < 0 {
		t.Fatal("score outside its published scale")
	}
}
