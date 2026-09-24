package collector

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/example/cs-pulse/backend/internal/model"
)

const steamNewsURL = "https://api.steampowered.com/ISteamNews/GetNewsForApp/v2/?appid=730&count=20&maxlength=0&feeds=steam_community_announcements"

type SteamNewsCollector struct{ client *http.Client }

func NewSteamNewsCollector() *SteamNewsCollector {
	return &SteamNewsCollector{client: &http.Client{Timeout: 15 * time.Second}}
}

func (c *SteamNewsCollector) Fetch() ([]model.Article, error) {
	resp, err := c.client.Get(steamNewsURL)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("steam news returned %s", resp.Status)
	}

	var payload struct {
		AppNews struct {
			NewsItems []struct {
				GID      string `json:"gid"`
				Title    string `json:"title"`
				URL      string `json:"url"`
				Contents string `json:"contents"`
				Date     int64  `json:"date"`
			} `json:"newsitems"`
		} `json:"appnews"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	articles := make([]model.Article, 0, len(payload.AppNews.NewsItems))
	for _, item := range payload.AppNews.NewsItems {
		articles = append(articles, model.Article{Source: "steam", ExternalID: "steam-" + item.GID, SourceURL: item.URL, Title: item.Title, Summary: item.Contents, Body: item.Contents, BodyFetchedAt: &now, PublishedAt: time.Unix(item.Date, 0).UTC(), FetchedAt: now})
	}
	return articles, nil
}
