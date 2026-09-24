package collector

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/example/cs-pulse/backend/internal/model"
)

const bbcWorldFeedURL = "https://feeds.bbci.co.uk/news/world/rss.xml"

type BBCCollector struct{ client *http.Client }

type bbcFeed struct {
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Description string `xml:"description"`
			Link        string `xml:"link"`
			Published   string `xml:"pubDate"`
			Thumbnail   struct {
				URL string `xml:"url,attr"`
			} `xml:"http://search.yahoo.com/mrss/ thumbnail"`
		} `xml:"item"`
	} `xml:"channel"`
}

func NewBBCCollector() *BBCCollector {
	return &BBCCollector{client: &http.Client{Timeout: 25 * time.Second}}
}

func (c *BBCCollector) FetchWorldNews() ([]model.Article, error) {
	response, err := c.client.Get(bbcWorldFeedURL)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(response.Body)
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("BBC RSS returned %s", response.Status)
	}
	var feed bbcFeed
	if err := xml.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&feed); err != nil {
		return nil, fmt.Errorf("decode BBC RSS: %w", err)
	}
	now := time.Now().UTC()
	articles := make([]model.Article, 0, len(feed.Channel.Items))
	seen := make(map[string]bool)
	for _, item := range feed.Channel.Items {
		pageURL, err := url.Parse(strings.TrimSpace(item.Link))
		if err != nil || pageURL.Scheme != "https" || !bbcNewsHost(pageURL.Hostname()) || !strings.HasPrefix(pageURL.Path, "/news/") {
			continue
		}
		pageURL.RawQuery = ""
		pageURL.Fragment = ""
		canonical := pageURL.String()
		if seen[canonical] || strings.TrimSpace(item.Title) == "" {
			continue
		}
		seen[canonical] = true
		publishedAt, err := time.Parse(time.RFC1123Z, strings.TrimSpace(item.Published))
		if err != nil {
			publishedAt, err = time.Parse(time.RFC1123, strings.TrimSpace(item.Published))
			if err != nil {
				publishedAt = now
			}
		}
		digest := sha1.Sum([]byte(canonical))
		articles = append(articles, model.Article{
			Source: "bbc", ExternalID: "bbc-world-" + hex.EncodeToString(digest[:10]),
			SourceURL: canonical, ImageURL: safeBBCImageURL(item.Thumbnail.URL),
			Title: strings.TrimSpace(item.Title), Summary: strings.TrimSpace(item.Description),
			PublishedAt: publishedAt.UTC(), FetchedAt: now,
		})
	}
	if len(articles) == 0 {
		return nil, fmt.Errorf("BBC World RSS contained no usable articles")
	}
	return articles, nil
}

func (c *BBCCollector) FetchArticleBody(pageURL string) (string, string, error) {
	parsed, err := url.Parse(pageURL)
	if err != nil || parsed.Scheme != "https" || !bbcNewsHost(parsed.Hostname()) || !strings.HasPrefix(parsed.Path, "/news/") {
		return "", "", fmt.Errorf("unsupported BBC article URL")
	}
	response, err := c.client.Get(pageURL)
	if err != nil {
		return "", "", err
	}
	defer closeResponseBody(response.Body)
	if response.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("BBC article returned %s", response.Status)
	}
	doc, err := goquery.NewDocumentFromReader(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return "", "", err
	}
	imageURL := safeBBCImageURL(doc.Find(`meta[property="og:image"]`).First().AttrOr("content", ""))
	for _, selector := range []string{`article [data-testid="rich-text"] p`, `article [data-component="text-block"] p`, `article p`} {
		paragraphs := make([]string, 0, 20)
		doc.Find(selector).Each(func(_ int, node *goquery.Selection) {
			line := cleanText(node.Text())
			if len([]rune(line)) < 25 || (len(paragraphs) > 0 && paragraphs[len(paragraphs)-1] == line) {
				return
			}
			paragraphs = append(paragraphs, line)
		})
		if len(paragraphs) >= 2 {
			return strings.Join(paragraphs, "\n\n"), imageURL, nil
		}
	}
	return "", imageURL, fmt.Errorf("BBC article contained no readable report text")
}

func bbcNewsHost(host string) bool {
	return host == "bbc.co.uk" || host == "www.bbc.co.uk" || host == "bbc.com" || host == "www.bbc.com"
}

func safeBBCImageURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" {
		return ""
	}
	host := parsed.Hostname()
	if host == "bbci.co.uk" || strings.HasSuffix(host, ".bbci.co.uk") || host == "bbcimg.co.uk" || strings.HasSuffix(host, ".bbcimg.co.uk") {
		return parsed.String()
	}
	return ""
}
