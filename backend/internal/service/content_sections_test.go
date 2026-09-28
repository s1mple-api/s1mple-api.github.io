package service

import (
	"testing"
	"time"

	"github.com/example/cs-pulse/backend/internal/model"
)

func TestContentClassification(t *testing.T) {
	for _, tc := range []struct{ category, title, summary, section string }{
		{"world", "OpenAI announces new AI technology", "", "world-technology"},
		{"world", "He said the trail was beautiful", "", "world-general"},
		{"world", "央行：经济增长与贸易展望", "", "world-business"},
		{"world", "选举结果公布", "", "world-politics"},
		{"cs2", "FaZe signs new player", "The team prepares for the major", "cs2-roster"},
		{"cs2", "Counter-Strike 2 update released", "", "cs2-updates"},
		{"cs2", "Vitality win IEM final", "", "cs2-events"},
		{"cs2", "选手专访：保持专注", "", "cs2-interviews"},
		{"cs2", "An unexpected announcement", "", "cs2-general"},
		{"community", "NBA 湖人新赛季展望", "", "community-sports"},
		{"community", "华为手机新款体验", "", "community-tech"},
		{"community", "CS2 决赛你看好谁", "", "community-esports"},
		{"community", "今天随便聊聊", "", "community-general"},
	} {
		t.Run(tc.title, func(t *testing.T) {
			if got := classifyContent(tc.category, tc.title, tc.summary); got.ID != tc.section {
				t.Errorf("got %s, want %s", got.ID, tc.section)
			}
		})
	}
}

func TestSectionsAreUniqueAndIncludedInAPIModels(t *testing.T) {
	seen := make(map[string]bool)
	for _, section := range append(NewsSections(), CommunitySections()...) {
		if section.ID == "" || section.Label == "" || seen[section.ID] {
			t.Fatalf("invalid section: %+v", section)
		}
		seen[section.ID] = true
	}
	news := RankNews([]model.Article{{ID: 1, Source: "hltv", Title: "Player transfer confirmed", PublishedAt: time.Now()}}, nil, nil, time.Now())
	if len(news) != 1 || news[0].Section != "cs2-roster" || news[0].SectionLabel != "转会阵容" {
		t.Fatalf("missing news section: %+v", news)
	}
	topics := CategorizeCommunity([]model.Trend{{Source: "hupu", Keyword: "NBA 湖人近况", Rank: 4}})
	if len(topics) != 1 || topics[0].Section != "community-sports" || topics[0].Rank != 4 {
		t.Fatalf("missing community section: %+v", topics)
	}
	if CategorizeCommunity(nil) == nil {
		t.Fatal("empty community response must be an array")
	}
}
