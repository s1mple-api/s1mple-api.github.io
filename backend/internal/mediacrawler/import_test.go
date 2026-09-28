package mediacrawler

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/example/cs-pulse/backend/internal/model"
)

func TestCanonicalPostURLRemovesXHSAccessToken(t *testing.T) {
	got := canonicalPostURL("xiaohongshu", "https://www.xiaohongshu.com/explore/note?xsec_token=private-value&xsec_source=pc_search", "note")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Has("xsec_token") || strings.Contains(got, "private-value") {
		t.Fatalf("access token must not be persisted in a visible source URL: %s", got)
	}
}

func TestBuildPreservesTopicFullTextAndEveryImage(t *testing.T) {
	topic := model.CommunityResource{Key: "actual-hot-topic", Source: "xiaohongshu", Title: "旅行摄影"}
	rows := []Row{{"note_id": "note", "title": "完整正文", "desc": "完整正文", "image_list": "http://sns-webpic.xhscdn.com/a.jpg,https://sns-webpic.xhscdn.com/b.jpg,https://evil.example/c.jpg", "comment_count": 5}}
	comments := []Row{
		{"note_id": "note", "comment_id": "grandchild", "parent_comment_id": "child", "content": "二级回复"},
		{"note_id": "note", "comment_id": "child", "parent_comment_id": "root", "content": "回复", "pictures": []any{map[string]any{"url": "https://sns-webpic.xhscdn.com/reply.jpg"}}},
		{"note_id": "note", "comment_id": "root", "content": "评论", "pictures": "https://sns-webpic.xhscdn.com/comment.jpg"},
	}
	list, results, err := Build(topic, rows, comments)
	if err != nil || list.Topic.Key != topic.Key || len(list.Posts) != 1 {
		t.Fatalf("topic identity lost: %+v %v", list, err)
	}
	post := list.Posts[0]
	if len(post.Body) != 3 || post.Body[0].Text != "完整正文" || post.CoverURL != "https://sns-webpic.xhscdn.com/a.jpg" {
		t.Fatalf("full text/multiple images not preserved: %+v", post)
	}
	result := results[post.Key+"|hot"]
	if len(result.Comments) != 1 || len(result.Comments[0].Replies) != 1 || len(result.Comments[0].Body) != 2 || len(result.Comments[0].Replies[0].Body) != 2 || !result.Partial {
		t.Fatalf("comment pictures/reply ordering lost: %+v", result)
	}
	if len(result.Comments[0].Replies[0].Replies) != 1 {
		t.Fatal("nested replies must not depend on JSONL row order")
	}
}

func TestBuildDoesNotReportCommentFetchFailureAsEmptySuccess(t *testing.T) {
	topic := model.CommunityResource{Key: "topic", Source: "xiaohongshu"}
	list, results, err := Build(topic, []Row{{"note_id": "note", "comments_error": true}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if results[list.Posts[0].Key+"|hot"].Status.State != "unavailable" {
		t.Fatal("comment retrieval failure must be explicit")
	}
}

func TestImportedRichBodyPreservesOrderAndDeduplicatesImages(t *testing.T) {
	row := Row{"body": []model.ContentBlock{{Type: "text", Text: "第一段"}, {Type: "image", URL: "https://tiebapic.baidu.com/a.jpg"}, {Type: "text", Text: "第二段"}}, "image_list": []string{"https://tiebapic.baidu.com/a.jpg", "https://tiebapic.baidu.com/b.jpg"}}
	body := importedBody(row, "fallback")
	if len(body) != 4 || body[0].Text != "第一段" || body[2].Text != "第二段" || body[3].URL != "https://tiebapic.baidu.com/b.jpg" {
		t.Fatalf("unexpected rich body: %+v", body)
	}
	post := model.CommunityPost{Key: "note"}
	hot, recent := convertComments(post, nil, time.Now())
	if hot.Comments == nil || recent.Comments == nil {
		t.Fatal("empty comment list must be [] instead of null")
	}
}

func TestCrawlerImageURLAllowsOnlyXHSCDN(t *testing.T) {
	if got := crawlerImageURL("http://sns-webpic.xhscdn.com/path/image.jpg"); !strings.HasPrefix(got, "https://") {
		t.Fatalf("expected secure XHS CDN URL, got %q", got)
	}
	if got := crawlerImageURL("https://evil.example/image.jpg"); got != "" {
		t.Fatalf("unexpected host was accepted: %q", got)
	}
}
