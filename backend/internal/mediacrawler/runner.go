package mediacrawler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/example/cs-pulse/backend/internal/config"
	"github.com/example/cs-pulse/backend/internal/model"
)

type Runner struct {
	cfg config.Config
	mu  sync.Mutex
}

func NewRunner(cfg config.Config) *Runner { return &Runner{cfg: cfg} }
func (r *Runner) Enabled(source string) bool {
	return (source == "xiaohongshu" || source == "tieba") && r.cfg.MediaCrawlerHome != "" && r.cfg.MediaCrawlerPython != "" && r.cfg.MediaCrawlerBridge != ""
}

func (r *Runner) Fetch(topic model.CommunityResource, page int) (model.CommunityPostList, map[string]model.CommunityComments, error) {
	if !r.mu.TryLock() {
		return model.CommunityPostList{}, nil, fmt.Errorf("其他话题正在读取，请稍后重试。")
	}
	defer r.mu.Unlock()
	dir, err := os.MkdirTemp("", "lifetv-community-")
	if err != nil {
		return model.CommunityPostList{}, nil, err
	}
	defer os.RemoveAll(dir)
	keyword := topic.Title
	if parsed, err := url.Parse(topic.SourceURL); err == nil {
		for _, key := range []string{"keyword", "word", "kw"} {
			if value := parsed.Query().Get(key); value != "" {
				keyword = value
				break
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	output := filepath.Join(dir, "result.json")
	cmd := exec.CommandContext(ctx, r.cfg.MediaCrawlerPython, r.cfg.MediaCrawlerBridge, "--home", r.cfg.MediaCrawlerHome, "--output", output, "--source", topic.Source, "--keyword", keyword, "--page", fmt.Sprint(page), "--cdp", r.cfg.MediaCrawlerCDP)
	cmd.Dir = r.cfg.MediaCrawlerHome
	if cmd.Run() != nil {
		return model.CommunityPostList{}, nil, fmt.Errorf("话题读取服务暂时不可用，请检查浏览器连接。")
	}
	body, err := os.ReadFile(output)
	if err != nil {
		return model.CommunityPostList{}, nil, fmt.Errorf("话题读取未返回数据。")
	}
	var result struct {
		Posts    []Row  `json:"posts"`
		Comments []Row  `json:"comments"`
		HasMore  bool   `json:"hasMore"`
		Error    string `json:"error"`
	}
	if json.Unmarshal(body, &result) != nil {
		return model.CommunityPostList{}, nil, fmt.Errorf("话题读取返回异常。")
	}
	if result.Error == "session" {
		return model.CommunityPostList{}, nil, fmt.Errorf("浏览器中的平台登录状态已失效，重新登录后可重试。")
	}
	if result.Error != "" {
		return model.CommunityPostList{}, nil, fmt.Errorf("浏览器连接或来源暂时不可用，请稍后重试。")
	}
	if len(result.Posts) == 0 {
		return model.CommunityPostList{Topic: topic, Posts: []model.CommunityPost{}, Page: page, Status: model.CommunityState{State: "ready", FetchedAt: time.Now().UTC()}}, nil, nil
	}
	list, comments, err := Build(topic, result.Posts, result.Comments)
	list.Page = page
	list.HasMore = result.HasMore
	return list, comments, err
}
