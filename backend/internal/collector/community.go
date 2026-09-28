package collector

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/example/cs-pulse/backend/internal/model"
	"golang.org/x/net/html"
)

var (
	bvPattern     = regexp.MustCompile(`^BV[0-9A-Za-z]{10}$`)
	hupuPostPath  = regexp.MustCompile(`^/([0-9]{5,20})\.html$`)
	tiebaPostPath = regexp.MustCompile(`^/p/[0-9]{5,20}$`)
)

type CommunityCollector struct {
	client    *http.Client
	userAgent string
}

func NewCommunityCollector(userAgent string) *CommunityCollector {
	return &CommunityCollector{userAgent: userAgent, client: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || req.URL.Scheme != "https" || req.URL.User != nil || req.URL.Host != via[0].URL.Host {
			return fmt.Errorf("public page redirected outside the expected site")
		}
		return nil
	}}}
}

func (c *CommunityCollector) get(rawURL string) (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	response, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("public data request failed: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		closeResponseBody(response.Body)
		return nil, fmt.Errorf("public data returned HTTP %d", response.StatusCode)
	}
	return response.Body, nil
}

func (c *CommunityCollector) page(source, rawURL string) (*goquery.Document, error) {
	if !validCommunityURL(source, rawURL) {
		return nil, fmt.Errorf("unexpected source URL")
	}
	body, err := c.get(rawURL)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(body)
	doc, err := goquery.NewDocumentFromReader(io.LimitReader(body, 8<<20))
	if err != nil {
		return nil, err
	}
	title := doc.Find("title").Text()
	if strings.Contains(title, "安全验证") || strings.Contains(title, "访问验证") {
		return nil, fmt.Errorf("public page requires verification")
	}
	return doc, nil
}

func validCommunityURL(source, rawURL string) bool {
	u, err := url.Parse(rawURL)
	hosts := map[string]string{"tieba": "tieba.baidu.com", "hupu": "bbs.hupu.com", "bilibili": "www.bilibili.com", "xiaohongshu": "www.xiaohongshu.com", "douyin": "www.douyin.com"}
	return err == nil && u.Scheme == "https" && u.User == nil && hosts[source] != "" && u.Host == hosts[source]
}

func publicMediaURL(raw string) string {
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Port() != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	for _, host := range []string{"hdslb.com", "hoopchina.com.cn", "baidu.com", "xhscdn.com", "xiaohongshu.com", "douyinpic.com", "douyincdn.com"} {
		if u.Hostname() == host || strings.HasSuffix(u.Hostname(), "."+host) {
			u.Scheme = "https"
			return u.String()
		}
	}
	return ""
}

func newCommunityPost(source, rawURL, title string) model.CommunityPost {
	return model.CommunityPost{Key: model.CommunityKey(source, "post:"+rawURL), Source: source, SourceURL: rawURL, Title: truncateText(cleanText(title), 500), Body: []model.ContentBlock{}}
}

func (c *CommunityCollector) TopicPosts(topic model.CommunityResource, page int) (model.CommunityPostList, error) {
	result := model.CommunityPostList{Topic: topic, Page: page, Posts: []model.CommunityPost{}}
	if topic.Source == "bilibili" {
		if page > 1 {
			return result, nil
		}
		post, err := c.bilibiliPost(topic.SourceURL)
		if err != nil {
			return result, err
		}
		result.Posts = append(result.Posts, post)
		return result, nil
	}
	if topic.Source == "xiaohongshu" || topic.Source == "douyin" {
		return result, fmt.Errorf("this platform does not expose a usable public topic post list")
	}
	u, err := url.Parse(topic.SourceURL)
	if err != nil || !validCommunityURL(topic.Source, topic.SourceURL) {
		return result, fmt.Errorf("unexpected topic URL")
	}
	query := u.Query()
	if topic.Source == "hupu" {
		query.Set("page", strconv.Itoa(page))
	} else {
		query.Set("pn", strconv.Itoa((page-1)*20))
	}
	u.RawQuery = query.Encode()
	doc, err := c.page(topic.Source, u.String())
	if err != nil {
		return result, err
	}
	result.Posts = parseForumPostList(doc, topic.Source)
	if len(result.Posts) == 0 {
		if topic.Source == "hupu" && doc.Find(".bbs-search-web-content").Length() > 0 {
			return result, nil
		}
		return result, fmt.Errorf("public page provided no readable posts")
	}
	if topic.Source == "tieba" {
		result.HasMore = len(result.Posts) >= 20
	} else {
		result.HasMore = len(result.Posts) >= 20
		doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
			link, _ := url.Parse(a.AttrOr("href", ""))
			if link != nil && link.Query().Get("page") == strconv.Itoa(page+1) {
				result.HasMore = true
			}
		})
	}
	return result, nil
}

func parseForumPostList(doc *goquery.Document, source string) []model.CommunityPost {
	posts := make([]model.CommunityPost, 0, 20)
	selector := ".thread-item"
	if source == "hupu" {
		selector = ".content-outline"
	}
	seen := map[string]bool{}
	doc.Find(selector).EachWithBreak(func(_ int, card *goquery.Selection) bool {
		link := card.Find("a.track-thread-title").First()
		if source == "hupu" {
			link = card.Find("a.content-wrap-span").First()
		}
		href := link.AttrOr("href", "")
		base, _ := url.Parse("https://tieba.baidu.com")
		if source == "hupu" {
			base, _ = url.Parse("https://bbs.hupu.com")
		}
		u, err := url.Parse(href)
		if err != nil {
			return true
		}
		u = base.ResolveReference(u)
		if !validCommunityURL(source, u.String()) || (source == "tieba" && !tiebaPostPath.MatchString(u.Path)) || (source == "hupu" && !hupuPostPath.MatchString(u.Path)) {
			return true
		}
		if seen[u.String()] || cleanText(link.Text()) == "" {
			return true
		}
		seen[u.String()] = true
		post := newCommunityPost(source, u.String(), link.Text())
		if source == "tieba" {
			post.Summary = truncateText(cleanText(card.Find(".content").First().Text()), 4000)
			post.Author = cleanText(card.Find(".author-info a").First().Text())
			post.CommentCount = int(parseHeat(card.Find(".reply-num").Text()))
			post.CoverURL = publicMediaURL(card.Find(".photo-wrapper img").First().AttrOr("src", ""))
		} else {
			spans := card.Find(".content-wrap > span")
			if date, err := time.ParseInLocation("2006-01-02", cleanText(spans.First().Text()), time.FixedZone("CST", 8*3600)); err == nil {
				post.PublishedAt = &date
			}
			post.CommentCount = int(parseHeat(card.Find(".content-wrap-span1").First().Text()))
		}
		posts = append(posts, post)
		return len(posts) < 20
	})
	return posts
}

func (c *CommunityCollector) Post(resource model.CommunityResource) (model.CommunityPost, error) {
	post := newCommunityPost(resource.Source, resource.SourceURL, resource.Title)
	post.Summary = resource.Summary
	if resource.Source == "bilibili" {
		return c.bilibiliPost(resource.SourceURL)
	}
	if resource.Source == "hupu" {
		detail, err := c.hupuPage(resource.SourceURL, 1)
		if err != nil {
			return post, err
		}
		post.Title = detail.Thread.Title
		post.Author, post.AvatarURL = detail.Thread.Author.Name, publicMediaURL(detail.Thread.Author.Avatar)
		post.Body = contentBlocks(detail.Thread.Content)
		post.PublishedAt = unixTime(detail.Thread.CreatedAt, true)
		post.Views, post.Likes, post.CommentCount = detail.Thread.Read, detail.Thread.Recommend, detail.Thread.Replies
		for _, block := range post.Body {
			if block.Type == "image" {
				post.CoverURL = block.URL
				break
			}
		}
		if len(post.Body) == 0 {
			return post, fmt.Errorf("public post has no readable body")
		}
		return post, nil
	}
	if resource.Source == "tieba" {
		doc, err := c.page("tieba", resource.SourceURL)
		if err != nil {
			return post, err
		}
		content := doc.Find(".d_post_content").First()
		fragment, _ := content.Html()
		post.Body = contentBlocks(fragment)
		if len(post.Body) == 0 {
			return post, fmt.Errorf("public post has no readable body")
		}
		post.Title = cleanText(doc.Find("h3.core_title_txt").First().Text())
		if post.Title == "" {
			post.Title = resource.Title
		}
		post.Author = cleanText(doc.Find(".p_author_name").First().Text())
		post.CommentCount = int(parseHeat(doc.Find(".l_reply_num span").First().Text()))
		return post, nil
	}
	return post, fmt.Errorf("public post detail unavailable")
}

func contentBlocks(fragment string) []model.ContentBlock {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(fragment))
	if err != nil {
		return []model.ContentBlock{}
	}
	blocks := make([]model.ContentBlock, 0)
	var text strings.Builder
	flush := func() {
		value := strings.TrimSpace(text.String())
		if value != "" {
			blocks = append(blocks, model.ContentBlock{Type: "text", Text: truncateText(value, 20000)})
		}
		text.Reset()
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(blocks) >= 300 {
			return
		}
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
			return
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "iframe", "noscript":
				return
			case "br":
				text.WriteString("\n")
				return
			case "img":
				flush()
				for _, key := range []string{"data-original", "data-src", "data-url", "src"} {
					found := false
					for _, attr := range n.Attr {
						if attr.Key == key {
							if imageURL := CommunityImageURL(attr.Val); imageURL != "" {
								blocks = append(blocks, model.ContentBlock{Type: "image", URL: imageURL})
								found = true
							}
						}
					}
					if found {
						break
					}
				}
				return
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if n.Type == html.ElementNode && strings.Contains(" p div li blockquote h1 h2 h3 h4 ", " "+n.Data+" ") {
			flush()
		}
	}
	for _, n := range doc.Find("body").Nodes {
		walk(n)
	}
	flush()
	return blocks
}

func unixTime(timestamp int64, millis bool) *time.Time {
	if timestamp <= 0 {
		return nil
	}
	if millis {
		timestamp /= 1000
	}
	value := time.Unix(timestamp, 0).UTC()
	return &value
}

func (c *CommunityCollector) api(path string, query url.Values, output any) error {
	body, err := c.get("https://uapis.cn/api/v1/social/bilibili/" + path + "?" + query.Encode())
	if err != nil {
		return err
	}
	defer closeResponseBody(body)
	if err := json.NewDecoder(io.LimitReader(body, 8<<20)).Decode(output); err != nil {
		return fmt.Errorf("invalid public API response: %w", err)
	}
	return nil
}

type biliVideo struct {
	BVID        string `json:"bvid"`
	AID         int64  `json:"aid"`
	Title       string `json:"title"`
	Description string `json:"desc"`
	Cover       string `json:"pic"`
	Published   int64  `json:"pubdate"`
	Owner       struct {
		Name string `json:"name"`
		Face string `json:"face"`
	} `json:"owner"`
	Stat struct {
		View  int64 `json:"view"`
		Like  int64 `json:"like"`
		Reply int   `json:"reply"`
	} `json:"stat"`
}

func biliID(rawURL string) (string, error) {
	if !validCommunityURL("bilibili", rawURL) {
		return "", fmt.Errorf("unexpected Bilibili URL")
	}
	u, _ := url.Parse(rawURL)
	id := strings.TrimPrefix(strings.TrimSuffix(u.Path, "/"), "/video/")
	if !bvPattern.MatchString(id) {
		return "", fmt.Errorf("invalid Bilibili video ID")
	}
	return id, nil
}

func (c *CommunityCollector) video(rawURL string) (biliVideo, error) {
	var video biliVideo
	id, err := biliID(rawURL)
	if err != nil {
		return video, err
	}
	err = c.api("videoinfo", url.Values{"bvid": {id}}, &video)
	if err == nil && (video.BVID != id || video.AID <= 0 || video.Title == "") {
		err = fmt.Errorf("public API did not provide video details")
	}
	return video, err
}

func (c *CommunityCollector) bilibiliPost(rawURL string) (model.CommunityPost, error) {
	video, err := c.video(rawURL)
	post := newCommunityPost("bilibili", rawURL, video.Title)
	if err != nil {
		return post, err
	}
	post.Author, post.AvatarURL, post.CoverURL = video.Owner.Name, publicMediaURL(video.Owner.Face), publicMediaURL(video.Cover)
	post.VideoID, post.VideoAID, post.PublishedAt = video.BVID, video.AID, unixTime(video.Published, false)
	post.Views, post.Likes, post.CommentCount = video.Stat.View, video.Stat.Like, video.Stat.Reply
	post.Summary = truncateText(video.Description, 4000)
	if video.Description != "" {
		post.Body = []model.ContentBlock{{Type: "text", Text: video.Description}}
	}
	return post, nil
}
