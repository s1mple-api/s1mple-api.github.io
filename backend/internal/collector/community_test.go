package collector

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/example/cs-pulse/backend/internal/model"
)

func TestCommunityContentIsTextAndAllowedImages(t *testing.T) {
	blocks := contentBlocks(`<p>正文<script>bad()</script><br>下一行</p><img src="https://i1.hoopchina.com.cn/news/a.jpg"><img src="javascript:alert(1)"><img src="https://unknown.example/a.jpg"><iframe src="https://unknown.example/"></iframe><p>结尾</p>`)
	if len(blocks) != 3 || blocks[0].Text != "正文\n下一行" || blocks[1].URL != "https://i1.hoopchina.com.cn/news/a.jpg" || blocks[2].Text != "结尾" {
		t.Fatalf("unexpected normalized body: %+v", blocks)
	}
	for _, raw := range []string{"https://bbs.hupu.com.evil.example/1.html", "https://user:pass@bbs.hupu.com/1.html", "https://bbs.hupu.com:1234/1.html", "http://bbs.hupu.com/1.html", "javascript:alert(1)"} {
		if validCommunityURL("hupu", raw) {
			t.Errorf("unsafe source accepted: %s", raw)
		}
	}
	if publicMediaURL("https://i1.hoopchina.com.cn.evil.example/a.jpg") != "" {
		t.Fatal("unsafe image host accepted")
	}
}

func TestPublicForumPostListsExcludeUnrelatedLinks(t *testing.T) {
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(`<div class="content-outline"><div class="content-wrap"><a class="content-wrap-span" href="https://bbs.hupu.com/642643470.html">库里新闻</a><a class="content-wrap-span" href="https://bbs.hupu.com/502">篮球专区</a><span>2026-09-28</span><span class="content-wrap-span1">1672</span></div></div><div class="content-outline"><a class="content-wrap-span" href="https://evil.example/642643470.html">外站</a></div>`))
	posts := parseForumPostList(doc, "hupu")
	if len(posts) != 1 || posts[0].CommentCount != 1672 || posts[0].PublishedAt == nil || posts[0].Title != "库里新闻" {
		t.Fatalf("unexpected list: %+v", posts)
	}
	if posts[0].Key == model.CommunityKey("hupu", posts[0].SourceURL) {
		t.Fatal("topic and post must have different persistent keys")
	}
}

type communityTransport func(*http.Request) (*http.Response, error)

func (f communityTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestBilibiliCommentPaginationAndReplyPreview(t *testing.T) {
	c := NewCommunityCollector("LIFE TV test")
	c.client.Transport = communityTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "uapis.cn" || req.URL.Path != "/api/v1/social/bilibili/replies" || req.URL.Query().Get("oid") != "117343120398899" || req.URL.Query().Get("pn") != "2" || req.URL.Query().Get("sort") != "time" {
			t.Fatalf("unexpected request: %s", req.URL)
		}
		body := `{"page":{"num":2,"size":20,"count":45},"replies":[{"rpid":12,"ctime":1790568341,"like":50,"count":149,"member":{"uname":"用户"},"content":{"message":"评论正文"},"replies":[{"rpid":13,"member":{"uname":"回复用户"},"content":{"message":"子回复"}}]}]}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	rows, err := c.Comments(model.CommunityPost{Source: "bilibili", VideoAID: 117343120398899}, 2, "time")
	if err != nil || !rows.HasMore || rows.Total != 45 || len(rows.Comments) != 1 || len(rows.Comments[0].Replies) != 1 || rows.Comments[0].ReplyCount != 149 {
		t.Fatalf("invalid page/reply mapping: %+v, %v", rows, err)
	}
}

func TestNewPlatformHotboardsAndBilibiliIdentifiers(t *testing.T) {
	now := time.Date(2026, 9, 28, 6, 0, 0, 0, time.UTC)
	for _, source := range []string{"bilibili", "xiaohongshu", "douyin"} {
		body := `{"type":"` + source + `","update_time":"2026-09-28T05:55:00Z","list":[{"title":"公开话题","url":"https://www.` + source + `.com/search/test","hot_value":"947.5w"}]}`
		rows, err := decodeHotboard(strings.NewReader(body), source, now)
		if err != nil || len(rows) != 1 || rows[0].Heat != 9475000 {
			t.Fatalf("%s not decoded: %+v %v", source, rows, err)
		}
	}
	if id, err := biliID("https://www.bilibili.com/video/BV14Baa6JENd"); err != nil || id != "BV14Baa6JENd" {
		t.Fatal("valid BV identifier rejected")
	}
	if _, err := biliID("https://www.bilibili.com/video/BVbad"); err == nil {
		t.Fatal("invalid BV accepted")
	}
	comment := hupuComment(hupuReply{ID: "12", Content: "<p>正文</p>", Quote: &hupuReply{ID: "13", Content: "<p>隐藏引用</p>", Hidden: true}}, true)
	if comment.Quote != nil {
		t.Fatal("hidden quote must not be exposed")
	}
}
