package model

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

func CommunitySources() []string {
	return []string{"tieba", "hupu", "xiaohongshu"}
}

func IsCommunitySource(source string) bool {
	for _, candidate := range CommunitySources() {
		if candidate == source {
			return true
		}
	}
	return false
}

func CommunityKey(source, identity string) string {
	sum := sha256.Sum256([]byte(source + "\n" + identity))
	return hex.EncodeToString(sum[:12])
}

// Topic/post metadata outlives rotating hotboards, so bookmarked URLs remain valid.
type CommunityResource struct {
	Key       string     `json:"key" gorm:"primaryKey;size:24"`
	Kind      string     `json:"kind" gorm:"size:20;index"`
	Source    string     `json:"source" gorm:"size:20;index"`
	SourceURL string     `json:"sourceUrl" gorm:"type:text"`
	Title     string     `json:"title" gorm:"size:500"`
	Summary   string     `json:"summary" gorm:"type:text"`
	Rank      int        `json:"rank"`
	HeatLabel string     `json:"heatLabel" gorm:"size:100"`
	FetchedAt time.Time  `json:"fetchedAt"`
	DataJSON  string     `json:"-" gorm:"type:longtext"`
	ExpiresAt *time.Time `json:"-"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	URL  string `json:"url,omitempty"`
}

type CommunityPost struct {
	Key          string         `json:"key"`
	Source       string         `json:"source"`
	SourceURL    string         `json:"sourceUrl"`
	Title        string         `json:"title"`
	Summary      string         `json:"summary"`
	Author       string         `json:"author"`
	AvatarURL    string         `json:"avatarUrl"`
	CoverURL     string         `json:"coverUrl"`
	PublishedAt  *time.Time     `json:"publishedAt,omitempty"`
	Views        int64          `json:"views"`
	Likes        int64          `json:"likes"`
	CommentCount int            `json:"commentCount"`
	VideoID      string         `json:"videoId,omitempty"`
	VideoAID     int64          `json:"videoAid,omitempty"`
	Body         []ContentBlock `json:"body"`
}

type CommunityComment struct {
	ID          string             `json:"id"`
	Author      string             `json:"author"`
	AvatarURL   string             `json:"avatarUrl"`
	PublishedAt *time.Time         `json:"publishedAt,omitempty"`
	Likes       int64              `json:"likes"`
	ReplyCount  int                `json:"replyCount"`
	Body        []ContentBlock     `json:"body"`
	Replies     []CommunityComment `json:"replies"`
	Quote       *CommunityComment  `json:"quote,omitempty"`
}

type CommunityState struct {
	State     string    `json:"state"`
	Message   string    `json:"message,omitempty"`
	FetchedAt time.Time `json:"fetchedAt"`
	Cached    bool      `json:"cached"`
}

type CommunityPostList struct {
	Topic   CommunityResource `json:"topic"`
	Posts   []CommunityPost   `json:"posts"`
	Page    int               `json:"page"`
	HasMore bool              `json:"hasMore"`
	Status  CommunityState    `json:"status"`
}

type CommunityComments struct {
	Comments []CommunityComment `json:"comments"`
	Page     int                `json:"page"`
	Total    int                `json:"total"`
	HasMore  bool               `json:"hasMore"`
	Partial  bool               `json:"partial"`
	Sort     string             `json:"sort"`
	Status   CommunityState     `json:"status"`
}

type CommunityPostDetail struct {
	Post   CommunityPost  `json:"post"`
	Status CommunityState `json:"status"`
}
