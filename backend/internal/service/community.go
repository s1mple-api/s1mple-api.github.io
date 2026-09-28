package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/example/cs-pulse/backend/internal/model"
	"gorm.io/gorm"
)

func (s *SyncService) lockCommunity(key string) func() {
	value, _ := s.communityLocks.LoadOrStore(key, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func (s *SyncService) communityCache(key string, target any) (bool, bool, error) {
	cache, err := s.repo.CommunityResource(key, "cache")
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if json.Unmarshal([]byte(cache.DataJSON), target) != nil {
		return false, false, nil
	}
	return true, cache.ExpiresAt != nil && time.Now().UTC().Before(*cache.ExpiresAt), nil
}

func publicCommunityFailure(source string, err error) string {
	if source == "xiaohongshu" || source == "douyin" {
		return "该平台的公开热榜可读取，话题下的帖子和评论目前需要登录或可用的数据接口。"
	}
	if strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "verification") {
		return "源站暂时要求访问验证，无法读取这部分公开内容，请稍后再试。"
	}
	return "这部分内容暂时未能从公开来源读取，请稍后再试。"
}

func communityStatus(source string, err error) model.CommunityState {
	status := model.CommunityState{State: "ready", FetchedAt: time.Now().UTC()}
	if err != nil {
		status.State, status.Message = "unavailable", publicCommunityFailure(source, err)
	}
	return status
}

func (s *SyncService) CommunityTopicPosts(key string, page int, refresh bool) (model.CommunityPostList, error) {
	topic, err := s.repo.CommunityResource(key, "topic")
	if err != nil {
		return model.CommunityPostList{}, err
	}
	cacheKey := model.CommunityKey("topic-page", fmt.Sprintf("%s:%d", key, page))
	unlock := s.lockCommunity(cacheKey)
	defer unlock()
	var previous model.CommunityPostList
	found, fresh, err := s.communityCache(cacheKey, &previous)
	if err != nil {
		return previous, err
	}
	if fresh && !refresh && !(s.crawler.Enabled(topic.Source) && previous.Status.State == "unavailable") {
		previous.Topic = topic
		previous.Status.Cached = true
		return previous, nil
	}
	var result model.CommunityPostList
	var fetchErr error
	usedBrowser := false
	if topic.Source == "xiaohongshu" && s.crawler.Enabled(topic.Source) {
		usedBrowser = true
	} else {
		result, fetchErr = s.community.TopicPosts(topic, page)
		usedBrowser = fetchErr != nil && s.crawler.Enabled(topic.Source)
	}
	if usedBrowser {
		var comments map[string]model.CommunityComments
		result, comments, fetchErr = s.crawler.Fetch(topic, page)
		if fetchErr == nil {
			if len(result.Posts) > 0 {
				if err := s.repo.SaveCommunitySnapshot(result, comments, time.Hour); err != nil {
					return result, err
				}
			} else {
				_ = s.repo.SaveCommunityCache(cacheKey, topic.Source, result, 5*time.Minute)
			}
			return result, nil
		}
	}
	result.Topic = topic
	result.Page = page
	if result.Posts == nil {
		result.Posts = []model.CommunityPost{}
	}
	result.Status = communityStatus(topic.Source, fetchErr)
	if usedBrowser && fetchErr != nil {
		result.Status.Message = fetchErr.Error()
	}
	if fetchErr != nil && found && len(previous.Posts) > 0 {
		result = previous
		result.Topic = topic
		result.Status.State = "stale"
		result.Status.Message = "来源暂时无法更新，当前展示上次成功获取的帖子列表。"
		result.Status.Cached = true
	}
	if err := s.repo.SaveCommunityPosts(result.Posts); err != nil {
		return result, err
	}
	for _, post := range result.Posts {
		if post.Source != "bilibili" || fetchErr != nil {
			continue
		}
		// A Bilibili hotboard item is already a concrete video; reuse its fetched
		// details instead of spending another upstream call when it is opened.
		detail := model.CommunityPostDetail{Post: post, Status: result.Status}
		if err := s.repo.SaveCommunityCache(model.CommunityKey("post-detail", post.Key), post.Source, detail, 5*time.Minute); err != nil {
			return result, err
		}
	}
	if err := s.repo.SaveCommunityCache(cacheKey, topic.Source, result, 5*time.Minute); err != nil {
		return result, err
	}
	return result, nil
}

func (s *SyncService) CommunityPostDetails(key string) (model.CommunityPostDetail, error) {
	resource, err := s.repo.CommunityResource(key, "post")
	if err != nil {
		return model.CommunityPostDetail{}, err
	}
	cacheKey := model.CommunityKey("post-detail", key)
	unlock := s.lockCommunity(cacheKey)
	defer unlock()
	var previous model.CommunityPostDetail
	found, fresh, err := s.communityCache(cacheKey, &previous)
	if err != nil {
		return previous, err
	}
	if fresh {
		previous.Status.Cached = true
		return previous, nil
	}
	post, fetchErr := s.community.Post(resource)
	if fetchErr != nil {
		if resource.DataJSON != "" {
			_ = json.Unmarshal([]byte(resource.DataJSON), &post)
		}
		post.Title, post.Summary, post.Source, post.SourceURL, post.Key = resource.Title, resource.Summary, resource.Source, resource.SourceURL, resource.Key
	}
	result := model.CommunityPostDetail{Post: post, Status: communityStatus(resource.Source, fetchErr)}
	if fetchErr != nil && len(post.Body) > 0 {
		result.Status.State = "stale"
		result.Status.Message = "来源暂时无法更新，当前展示已获取的正文。"
		result.Status.Cached = true
	}
	if fetchErr != nil && found && (len(previous.Post.Body) > 0 || previous.Post.VideoID != "") {
		result = previous
		result.Status.State = "stale"
		result.Status.Message = "来源暂时无法更新，当前展示上次成功获取的正文。"
		result.Status.Cached = true
	}
	if err := s.repo.SaveCommunityCache(cacheKey, resource.Source, result, 5*time.Minute); err != nil {
		return result, err
	}
	return result, nil
}

func (s *SyncService) CommunityPostComments(key string, page int, sort string) (model.CommunityComments, error) {
	resource, err := s.repo.CommunityResource(key, "post")
	if err != nil {
		return model.CommunityComments{}, err
	}
	cacheKey := model.CommunityKey("comments", fmt.Sprintf("%s:%s:%d", key, sort, page))
	unlock := s.lockCommunity(cacheKey)
	defer unlock()
	var previous model.CommunityComments
	found, fresh, err := s.communityCache(cacheKey, &previous)
	if err != nil {
		return previous, err
	}
	if fresh {
		previous.Status.Cached = true
		return previous, nil
	}
	detail, err := s.CommunityPostDetails(key)
	if err != nil {
		return previous, err
	}
	var result model.CommunityComments
	var fetchErr error
	if detail.Status.State == "unavailable" {
		result = model.CommunityComments{Comments: []model.CommunityComment{}, Page: page, Sort: sort, Status: detail.Status}
		fetchErr = fmt.Errorf("%s", detail.Status.Message)
	} else {
		result, fetchErr = s.community.Comments(detail.Post, page, sort)
		result.Status = communityStatus(resource.Source, fetchErr)
		if fetchErr == nil && result.Partial {
			result.Status.Message = "公开接口当前只返回部分评论，后续分页暂时可能受限。"
		}
	}
	if fetchErr != nil && found && len(previous.Comments) > 0 {
		result = previous
		result.Status.State = "stale"
		result.Status.Message = "评论暂时无法更新，当前展示上次成功获取的评论。"
		result.Status.Cached = true
	}
	if err := s.repo.SaveCommunityCache(cacheKey, resource.Source, result, 5*time.Minute); err != nil {
		return result, err
	}
	return result, nil
}
