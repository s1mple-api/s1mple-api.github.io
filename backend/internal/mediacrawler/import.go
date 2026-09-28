package mediacrawler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/example/cs-pulse/backend/internal/collector"
	"github.com/example/cs-pulse/backend/internal/model"
	"github.com/example/cs-pulse/backend/internal/repository"
)

type Row map[string]any
type crawlRow = Row

func Import(repo *repository.Repository, source, keyword string, rows, commentRows []Row) error {
	topicURL := searchURL(source, keyword)
	topic := model.CommunityResource{Key: model.CommunityKey(source, topicURL), Kind: "topic", Source: source, SourceURL: topicURL, Title: fmt.Sprintf("%s · %s 社区讨论", keyword, sourceName(source)), Rank: 1}
	list, comments, err := Build(topic, rows, commentRows)
	if err != nil {
		return err
	}
	topic = list.Topic
	trend := model.Trend{Source: source, Provider: "MediaCrawler", Rank: 1, Keyword: topic.Title, HeatLabel: topic.HeatLabel, Summary: topic.Summary, Tag: "community", SourceURL: topicURL, FetchedAt: topic.FetchedAt}
	return repo.ImportCommunitySnapshot(trend, topic, list, comments, 30*24*time.Hour)
}

func Build(topic model.CommunityResource, rows, commentRows []Row) (model.CommunityPostList, map[string]model.CommunityComments, error) {
	source := topic.Source
	now := time.Now().UTC()
	communityPosts := make([]model.CommunityPost, 0, len(rows))
	commentsByPost := make(map[string]model.CommunityComments, len(rows))
	commentsByID := make(map[string][]crawlRow, len(rows))
	for _, row := range commentRows {
		commentsByID[text(row["note_id"])] = append(commentsByID[text(row["note_id"])], row)
	}
	for _, row := range rows {
		id := text(row["note_id"])
		title := firstText(text(row["title"]), text(row["desc"]), id)
		sourceURL := canonicalPostURL(source, text(row["note_url"]), id)
		if id == "" || sourceURL == "" {
			continue
		}
		description := text(row["desc"])
		summary := description
		if summary == title {
			summary = ""
		}
		published := parsePublished(firstValue(row, "time", "publish_time"))
		author := firstText(text(row["nickname"]), text(row["user_nickname"]))
		commentCount := int(number(firstValue(row, "comment_count", "total_replay_num")))
		likes := number(firstValue(row, "liked_count", "like_count"))
		body := importedBody(row, description)
		coverURL := ""
		for _, block := range body {
			if block.Type == "image" {
				coverURL = block.URL
				break
			}
		}
		post := model.CommunityPost{
			Key: model.CommunityKey(source, "post:"+sourceURL), Source: source, SourceURL: sourceURL,
			Title: title, Summary: summary, Author: author, CoverURL: coverURL, PublishedAt: published,
			Likes: likes, CommentCount: commentCount, Body: body,
		}
		communityPosts = append(communityPosts, post)
		hotComments, timeComments := convertComments(post, commentsByID[id], now)
		commentsByPost[post.Key+"|hot"] = hotComments
		commentsByPost[post.Key+"|time"] = timeComments
		if row["comments_error"] == true {
			for _, order := range []string{"hot", "time"} {
				key := post.Key + "|" + order
				result := commentsByPost[key]
				result.Status = model.CommunityState{State: "unavailable", Message: "帖子已读取，但源站暂时未返回评论，请稍后重新读取话题。", FetchedAt: now}
				commentsByPost[key] = result
			}
		}
	}
	if len(communityPosts) == 0 {
		return model.CommunityPostList{}, nil, fmt.Errorf("%s: no posts had a valid source URL", source)
	}
	topic.FetchedAt = now
	if topic.Summary == "" {
		topic.Summary = fmt.Sprintf("已读取 %d 篇相关帖子。", len(communityPosts))
	}
	if topic.HeatLabel == "" {
		topic.HeatLabel = fmt.Sprintf("%d 条帖子", len(communityPosts))
	}
	status := model.CommunityState{State: "ready", Message: "已读取相关帖子；评论展示当前获取到的内容。", FetchedAt: now, Cached: true}
	list := model.CommunityPostList{Topic: topic, Posts: communityPosts, Page: 1, HasMore: false, Status: status}
	for key := range commentsByPost {
		comments := commentsByPost[key]
		if comments.Status.State != "unavailable" {
			comments.Status = status
		}
		comments.Page = 1
		commentsByPost[key] = comments
	}
	return list, commentsByPost, nil
}

func ReadRows(contentsPath, commentsPath string) ([]Row, []Row, error) {
	if strings.HasSuffix(strings.ToLower(contentsPath), ".json") {
		body, err := os.ReadFile(contentsPath)
		if err != nil {
			return nil, nil, err
		}
		var snapshot struct {
			Posts    []Row  `json:"posts"`
			Comments []Row  `json:"comments"`
			Error    string `json:"error"`
		}
		if err := json.Unmarshal(body, &snapshot); err != nil {
			return nil, nil, err
		}
		if snapshot.Error != "" {
			return nil, nil, fmt.Errorf("crawler snapshot unavailable")
		}
		return snapshot.Posts, snapshot.Comments, nil
	}
	contents, err := readJSONL(contentsPath)
	if err != nil {
		return nil, nil, err
	}
	var comments []crawlRow
	if commentsPath != "" {
		comments, err = readJSONL(commentsPath)
		if err != nil {
			return nil, nil, err
		}
	}
	return contents, comments, nil
}

func readJSONL(path string) ([]crawlRow, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	rows := make([]crawlRow, 0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		var row crawlRow
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return nil, fmt.Errorf("decode crawler JSONL: %w", err)
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func convertComments(post model.CommunityPost, rows []crawlRow, now time.Time) (model.CommunityComments, model.CommunityComments) {
	byID := make(map[string]*model.CommunityComment, len(rows))
	parentByID := make(map[string]string, len(rows))
	ids := make([]string, 0, len(rows))
	for i, row := range rows {
		id := firstText(text(row["comment_id"]), fmt.Sprintf("%s-%d", post.Key, i))
		author := firstText(text(row["nickname"]), text(row["user_nickname"]), "社区用户")
		body := text(row["content"])
		comment := &model.CommunityComment{ID: id, Author: author, PublishedAt: parsePublished(firstValue(row, "create_time", "publish_time")), Likes: number(firstValue(row, "like_count")), ReplyCount: int(number(row["sub_comment_count"])), Body: importedBody(row, body), Replies: []model.CommunityComment{}}
		if byID[id] == nil {
			ids = append(ids, id)
		}
		byID[id] = comment
		parentByID[id] = text(row["parent_comment_id"])
	}
	roots := make([]model.CommunityComment, 0, len(rows))
	children := make(map[string][]string)
	for _, id := range ids {
		parent := parentByID[id]
		if parent != "" && parent != id && byID[parent] != nil {
			children[parent] = append(children[parent], id)
		}
	}
	var buildReply func(string, map[string]bool) model.CommunityComment
	buildReply = func(id string, ancestors map[string]bool) model.CommunityComment {
		comment := *byID[id]
		comment.Replies = []model.CommunityComment{}
		if len(ancestors) >= 10 {
			return comment
		}
		ancestors[id] = true
		for _, child := range children[id] {
			if !ancestors[child] {
				comment.Replies = append(comment.Replies, buildReply(child, ancestors))
			}
		}
		delete(ancestors, id)
		return comment
	}
	for _, id := range ids {
		if parentByID[id] == "" || parentByID[id] == id || byID[parentByID[id]] == nil {
			roots = append(roots, buildReply(id, make(map[string]bool)))
		}
	}
	hot := append([]model.CommunityComment{}, roots...)
	timeSorted := append([]model.CommunityComment{}, roots...)
	sort.SliceStable(hot, func(i, j int) bool { return hot[i].Likes > hot[j].Likes })
	sort.SliceStable(timeSorted, func(i, j int) bool {
		if timeSorted[i].PublishedAt == nil {
			return false
		}
		if timeSorted[j].PublishedAt == nil {
			return true
		}
		return timeSorted[i].PublishedAt.After(*timeSorted[j].PublishedAt)
	})
	makeResult := func(comments []model.CommunityComment, sort string) model.CommunityComments {
		return model.CommunityComments{Comments: comments, Page: 1, Total: post.CommentCount, HasMore: false, Partial: len(rows) < post.CommentCount, Sort: sort, Status: model.CommunityState{State: "ready", FetchedAt: now, Cached: true}}
	}
	return makeResult(hot, "hot"), makeResult(timeSorted, "time")
}

func textBody(value string) []model.ContentBlock {
	if strings.TrimSpace(value) == "" {
		return []model.ContentBlock{}
	}
	return []model.ContentBlock{{Type: "text", Text: strings.TrimSpace(value)}}
}

func crawlerImageURL(raw string) string { return collector.CommunityImageURL(raw) }

func importedBody(row Row, description string) []model.ContentBlock {
	blocks := textBody(description)
	if raw, ok := row["body"]; ok {
		encoded, _ := json.Marshal(raw)
		var supplied []model.ContentBlock
		if json.Unmarshal(encoded, &supplied) == nil {
			blocks = []model.ContentBlock{}
			for _, block := range supplied {
				if block.Type == "text" && block.Text != "" {
					blocks = append(blocks, model.ContentBlock{Type: "text", Text: block.Text})
				}
				if block.Type == "image" {
					if image := crawlerImageURL(block.URL); image != "" {
						blocks = append(blocks, model.ContentBlock{Type: "image", URL: image})
					}
				}
			}
		}
	}
	seen := make(map[string]bool)
	for _, block := range blocks {
		if block.Type == "image" {
			seen[block.URL] = true
		}
	}
	for _, field := range []string{"image_list", "pictures"} {
		for _, image := range imageURLs(row[field]) {
			if !seen[image] {
				blocks = append(blocks, model.ContentBlock{Type: "image", URL: image})
				seen[image] = true
			}
		}
	}
	return blocks
}

func imageURLs(value any) []string {
	images := []string{}
	var walk func(any)
	walk = func(value any) {
		switch value := value.(type) {
		case string:
			var decoded any
			if strings.HasPrefix(strings.TrimSpace(value), "[") && json.Unmarshal([]byte(value), &decoded) == nil {
				walk(decoded)
				return
			}
			for _, part := range strings.Split(value, ",") {
				if image := crawlerImageURL(part); image != "" {
					images = append(images, image)
				}
			}
		case []any:
			for _, item := range value {
				walk(item)
			}
		case []string:
			for _, item := range value {
				walk(item)
			}
		case map[string]any:
			walk(firstValue(value, "url_default", "url", "src", "original_src"))
		}
	}
	walk(value)
	return images
}

func searchURL(source, keyword string) string {
	switch source {
	case "tieba":
		return "https://tieba.baidu.com/f/search/res?ie=utf-8&kw=" + url.QueryEscape(keyword)
	default:
		return "https://www.xiaohongshu.com/search_result?keyword=" + url.QueryEscape(keyword)
	}
}

func canonicalPostURL(source, candidate, id string) string {
	parsed, err := url.Parse(strings.TrimSpace(candidate))
	if err == nil && parsed.IsAbs() && parsed.Scheme == "https" {
		host := strings.ToLower(parsed.Hostname())
		if source == "tieba" && (host == "tieba.baidu.com" || strings.HasSuffix(host, ".tieba.baidu.com")) {
			parsed.RawQuery = ""
			parsed.Fragment = ""
			return parsed.String()
		}
		if source == "xiaohongshu" && (host == "xiaohongshu.com" || strings.HasSuffix(host, ".xiaohongshu.com")) {
			query := parsed.Query()
			query.Del("xsec_token")
			query.Del("sec_token")
			parsed.RawQuery = query.Encode()
			parsed.Fragment = ""
			return parsed.String()
		}
	}
	switch source {
	case "tieba":
		if id != "" {
			return "https://tieba.baidu.com/p/" + url.PathEscape(id)
		}
	case "xiaohongshu":
		if id != "" {
			return "https://www.xiaohongshu.com/explore/" + url.PathEscape(id)
		}
	}
	return ""
}

func parsePublished(value any) *time.Time {
	if value == nil {
		return nil
	}
	if n := number(value); n > 0 {
		timestamp := n
		if timestamp > 100000000000 {
			timestamp /= 1000
		}
		parsed := time.Unix(timestamp, 0).UTC()
		if parsed.Year() >= 2000 && parsed.Year() <= time.Now().Year()+1 {
			return &parsed
		}
	}
	raw := text(value)
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, raw, time.FixedZone("CST", 8*3600)); err == nil {
			parsed = parsed.UTC()
			return &parsed
		}
	}
	return nil
}

func firstValue(row crawlRow, keys ...string) any {
	for _, key := range keys {
		if value, exists := row[key]; exists && value != nil && text(value) != "" {
			return value
		}
	}
	return nil
}

func firstText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func text(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case json.Number:
		return typed.String()
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return ""
	}
}

func number(value any) int64 {
	raw := text(value)
	if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return parsed
	}
	return 0
}

func sourceName(source string) string {
	if source == "tieba" {
		return "贴吧"
	}
	return "小红书"
}
