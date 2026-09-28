package collector

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/example/cs-pulse/backend/internal/model"
)

type hupuAuthor struct {
	Name   string `json:"puname"`
	Avatar string `json:"header"`
}
type hupuReply struct {
	ID          string     `json:"pid"`
	Content     string     `json:"content"`
	Likes       int64      `json:"count"`
	ReplyNum    int        `json:"replyNum"`
	CreatedAt   int64      `json:"createdAt"`
	Author      hupuAuthor `json:"author"`
	Hidden      bool       `json:"isHidden"`
	Deleted     bool       `json:"isDelete"`
	SelfDeleted bool       `json:"isSelfDelete"`
	Quote       *hupuReply `json:"quote"`
}
type hupuDetail struct {
	Thread struct {
		Title     string     `json:"title"`
		Content   string     `json:"content"`
		CreatedAt int64      `json:"createdAt"`
		Author    hupuAuthor `json:"author"`
		Read      int64      `json:"read"`
		Recommend int64      `json:"recommend"`
		Replies   int        `json:"replies"`
	} `json:"thread"`
	Lights  []hupuReply `json:"lights"`
	Replies struct {
		Count   int         `json:"count"`
		Current int         `json:"current"`
		Total   int         `json:"total"`
		List    []hupuReply `json:"list"`
	} `json:"replies"`
}

func (c *CommunityCollector) hupuPage(rawURL string, page int) (hupuDetail, error) {
	var detail hupuDetail
	u, err := url.Parse(rawURL)
	if err != nil || !validCommunityURL("hupu", rawURL) || !hupuPostPath.MatchString(u.Path) {
		return detail, fmt.Errorf("invalid Hupu post URL")
	}
	if page > 1 {
		u.Path = strings.TrimSuffix(u.Path, ".html") + "-" + strconv.Itoa(page) + ".html"
	}
	doc, err := c.page("hupu", u.String())
	if err != nil {
		return detail, err
	}
	return decodeHupuDetail(doc)
}

func decodeHupuDetail(doc *goquery.Document) (hupuDetail, error) {
	var payload struct {
		Props struct {
			PageProps struct {
				Detail hupuDetail `json:"detail"`
			} `json:"pageProps"`
		} `json:"props"`
	}
	err := json.Unmarshal([]byte(doc.Find("script#__NEXT_DATA__").Text()), &payload)
	detail := payload.Props.PageProps.Detail
	if err != nil || detail.Thread.Title == "" {
		return detail, fmt.Errorf("public page did not provide post details")
	}
	return detail, nil
}

func hupuComment(reply hupuReply, includeQuote bool) model.CommunityComment {
	comment := model.CommunityComment{ID: reply.ID, Author: reply.Author.Name, AvatarURL: publicMediaURL(reply.Author.Avatar), PublishedAt: unixTime(reply.CreatedAt, true), Likes: reply.Likes, ReplyCount: reply.ReplyNum, Body: contentBlocks(reply.Content), Replies: []model.CommunityComment{}}
	if includeQuote && reply.Quote != nil && !reply.Quote.Hidden && !reply.Quote.Deleted && !reply.Quote.SelfDeleted {
		quote := hupuComment(*reply.Quote, false)
		comment.Quote = &quote
	}
	return comment
}

type biliReply struct {
	ID        int64 `json:"rpid"`
	CreatedAt int64 `json:"ctime"`
	Likes     int64 `json:"like"`
	Count     int   `json:"count"`
	Member    struct {
		Name   string `json:"uname"`
		Avatar string `json:"avatar"`
	} `json:"member"`
	Content struct {
		Message string `json:"message"`
	} `json:"content"`
	Replies []biliReply `json:"replies"`
}

func biliComment(reply biliReply, nested bool) model.CommunityComment {
	comment := model.CommunityComment{ID: strconv.FormatInt(reply.ID, 10), Author: reply.Member.Name, AvatarURL: publicMediaURL(reply.Member.Avatar), PublishedAt: unixTime(reply.CreatedAt, false), Likes: reply.Likes, ReplyCount: reply.Count, Body: []model.ContentBlock{{Type: "text", Text: reply.Content.Message}}, Replies: []model.CommunityComment{}}
	if nested {
		for _, child := range reply.Replies {
			if len(comment.Replies) >= 10 {
				break
			}
			comment.Replies = append(comment.Replies, biliComment(child, false))
		}
	}
	return comment
}

func (c *CommunityCollector) Comments(post model.CommunityPost, page int, sort string) (model.CommunityComments, error) {
	result := model.CommunityComments{Comments: []model.CommunityComment{}, Page: page, Sort: sort}
	switch post.Source {
	case "bilibili":
		if post.VideoAID < 1 {
			return result, fmt.Errorf("video object ID is unavailable")
		}
		var payload struct {
			Page struct {
				Count int `json:"count"`
				Size  int `json:"size"`
				Num   int `json:"num"`
			} `json:"page"`
			Replies []biliReply `json:"replies"`
		}
		err := c.api("replies", url.Values{"oid": {strconv.FormatInt(post.VideoAID, 10)}, "sort": {sort}, "ps": {"20"}, "pn": {strconv.Itoa(page)}}, &payload)
		if err != nil {
			return result, err
		}
		if payload.Page.Num < 1 || payload.Page.Size < 1 {
			return result, fmt.Errorf("public API did not provide comment pagination")
		}
		result.Total, result.HasMore = payload.Page.Count, payload.Page.Num*payload.Page.Size < payload.Page.Count
		seen := make(map[int64]bool)
		for _, reply := range payload.Replies {
			if reply.ID < 1 || seen[reply.ID] {
				continue
			}
			seen[reply.ID] = true
			result.Comments = append(result.Comments, biliComment(reply, true))
			if len(result.Comments) >= 20 {
				break
			}
		}
		result.Partial = result.HasMore && len(result.Comments) < payload.Page.Size
	case "hupu":
		detail, err := c.hupuPage(post.SourceURL, page)
		if err != nil {
			return result, err
		}
		rows := detail.Replies.List
		result.Total, result.HasMore = detail.Replies.Count, detail.Replies.Current < detail.Replies.Total
		if sort == "hot" {
			rows, result.HasMore = detail.Lights, false
			result.Total = len(rows)
		}
		for _, reply := range rows {
			if reply.Hidden || reply.Deleted || reply.SelfDeleted {
				continue
			}
			result.Comments = append(result.Comments, hupuComment(reply, true))
		}
	case "tieba":
		u, err := url.Parse(post.SourceURL)
		if err != nil {
			return result, err
		}
		query := u.Query()
		query.Set("pn", strconv.Itoa(page))
		u.RawQuery = query.Encode()
		doc, err := c.page("tieba", u.String())
		if err != nil {
			return result, err
		}
		doc.Find(".l_post").Each(func(index int, row *goquery.Selection) {
			if page == 1 && index == 0 {
				return
			}
			var field struct {
				Content struct {
					ID   int64  `json:"post_id"`
					Date string `json:"date"`
				} `json:"content"`
			}
			_ = json.Unmarshal([]byte(row.AttrOr("data-field", "{}")), &field)
			fragment, _ := row.Find(".d_post_content").Html()
			blocks := contentBlocks(fragment)
			if field.Content.ID <= 0 || len(blocks) == 0 {
				return
			}
			result.Comments = append(result.Comments, model.CommunityComment{ID: strconv.FormatInt(field.Content.ID, 10), Author: cleanText(row.Find(".p_author_name").First().Text()), Body: blocks, Replies: []model.CommunityComment{}})
		})
		if len(result.Comments) == 0 && doc.Find(".l_post").Length() == 0 {
			return result, fmt.Errorf("public page did not provide readable comments")
		}
		result.Total = post.CommentCount
		doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
			href, _ := url.Parse(a.AttrOr("href", ""))
			if href != nil && href.Query().Get("pn") == strconv.Itoa(page+1) {
				result.HasMore = true
			}
		})
	default:
		return result, fmt.Errorf("public comments unavailable")
	}
	return result, nil
}
