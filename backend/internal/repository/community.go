package repository

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/example/cs-pulse/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func saveCommunityTopics(db *gorm.DB, rows []model.Trend) error {
	for _, row := range rows {
		resource := model.CommunityResource{Key: model.CommunityKey(row.Source, row.SourceURL), Kind: "topic", Source: row.Source, SourceURL: row.SourceURL, Title: row.Keyword, Summary: row.Summary, Rank: row.Rank, HeatLabel: row.HeatLabel, FetchedAt: row.FetchedAt}
		if err := saveCommunityMetadata(db, resource); err != nil {
			return err
		}
	}
	return nil
}

func saveCommunityMetadata(db *gorm.DB, resource model.CommunityResource) error {
	columns := []string{"source_url", "title", "summary", "rank", "heat_label", "fetched_at"}
	if resource.Kind == "post" {
		columns = append(columns, "data_json")
	}
	return db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns(columns)}).Create(&resource).Error
}

func (r *Repository) CommunityResource(key, kind string) (model.CommunityResource, error) {
	var resource model.CommunityResource
	result := r.db.Where("`key` = ? AND kind = ?", key, kind).Limit(1).Find(&resource)
	if result.Error != nil {
		return resource, result.Error
	}
	if result.RowsAffected == 0 {
		return resource, gorm.ErrRecordNotFound
	}
	return resource, nil
}

func (r *Repository) SaveCommunityPosts(posts []model.CommunityPost) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, post := range posts {
			// List-only refreshes must not erase a previously collected full body.
			if len(post.Body) == 0 {
				var previous model.CommunityResource
				if err := tx.Select("data_json").Where("`key` = ? AND kind = ?", post.Key, "post").Limit(1).Find(&previous).Error; err != nil {
					return err
				}
				var cached model.CommunityPost
				if json.Unmarshal([]byte(previous.DataJSON), &cached) == nil {
					post.Body = cached.Body
					if post.CoverURL == "" {
						post.CoverURL = cached.CoverURL
					}
				}
			}
			resource := model.CommunityResource{Key: post.Key, Kind: "post", Source: post.Source, SourceURL: post.SourceURL, Title: post.Title, Summary: post.Summary, FetchedAt: time.Now().UTC()}
			payload, err := json.Marshal(post)
			if err != nil {
				return err
			}
			resource.DataJSON = string(payload)
			if err := saveCommunityMetadata(tx, resource); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) SaveCommunityCache(key, source string, data any, duration time.Duration) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	expires := time.Now().UTC().Add(duration)
	resource := model.CommunityResource{Key: key, Kind: "cache", Source: source, DataJSON: string(encoded), FetchedAt: time.Now().UTC(), ExpiresAt: &expires}
	return r.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"data_json", "expires_at"})}).Create(&resource).Error
}

func (r *Repository) SaveCommunitySnapshot(list model.CommunityPostList, comments map[string]model.CommunityComments, duration time.Duration) error {
	if err := r.SaveCommunityPosts(list.Posts); err != nil {
		return err
	}
	for _, post := range list.Posts {
		if err := r.SaveCommunityCache(model.CommunityKey("post-detail", post.Key), post.Source, model.CommunityPostDetail{Post: post, Status: list.Status}, duration); err != nil {
			return err
		}
		for _, order := range []string{"hot", "time"} {
			result := comments[post.Key+"|"+order]
			if result.Status.State == "unavailable" {
				// Leave any prior successful comments available for stale fallback.
				continue
			}
			if err := r.SaveCommunityCache(model.CommunityKey("comments", fmt.Sprintf("%s:%s:1", post.Key, order)), post.Source, result, duration); err != nil {
				return err
			}
		}
	}
	return r.SaveCommunityCache(model.CommunityKey("topic-page", fmt.Sprintf("%s:%d", list.Topic.Key, list.Page)), list.Topic.Source, list, duration)
}

// ImportCommunitySnapshot stores a small, already-collected MediaCrawler
// snapshot in the same resources and cache rows used by the community pages.
// The snapshot remains available as stale content if a later live refresh fails.
func (r *Repository) ImportCommunitySnapshot(topic model.Trend, topicResource model.CommunityResource, list model.CommunityPostList, comments map[string]model.CommunityComments, duration time.Duration) error {
	if !model.IsCommunitySource(topic.Source) || topic.Source != topicResource.Source || len(list.Posts) == 0 {
		return fmt.Errorf("invalid community snapshot")
	}
	if duration <= 0 {
		duration = 24 * time.Hour
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("source = ? AND provider = ? AND source_url = ?", topic.Source, topic.Provider, topic.SourceURL).Delete(&model.Trend{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&topic).Error; err != nil {
			return err
		}
		if err := saveCommunityMetadata(tx, topicResource); err != nil {
			return err
		}
		for _, post := range list.Posts {
			resource := model.CommunityResource{Key: post.Key, Kind: "post", Source: post.Source, SourceURL: post.SourceURL, Title: post.Title, Summary: post.Summary, FetchedAt: time.Now().UTC()}
			payload, err := json.Marshal(post)
			if err != nil {
				return err
			}
			resource.DataJSON = string(payload)
			if err := saveCommunityMetadata(tx, resource); err != nil {
				return err
			}
			detail := model.CommunityPostDetail{Post: post, Status: list.Status}
			if err := saveCommunityCacheTx(tx, model.CommunityKey("post-detail", post.Key), post.Source, detail, duration); err != nil {
				return err
			}
			for _, sort := range []string{"hot", "time"} {
				result := comments[post.Key+"|"+sort]
				result.Sort = sort
				if err := saveCommunityCacheTx(tx, model.CommunityKey("comments", fmt.Sprintf("%s:%s:%d", post.Key, sort, 1)), post.Source, result, duration); err != nil {
					return err
				}
			}
		}
		list.Topic = topicResource
		if err := saveCommunityCacheTx(tx, model.CommunityKey("topic-page", fmt.Sprintf("%s:%d", topicResource.Key, 1)), topic.Source, list, duration); err != nil {
			return err
		}
		return nil
	})
}

func saveCommunityCacheTx(tx *gorm.DB, key, source string, data any, duration time.Duration) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	expires := time.Now().UTC().Add(duration)
	resource := model.CommunityResource{Key: key, Kind: "cache", Source: source, DataJSON: string(encoded), FetchedAt: time.Now().UTC(), ExpiresAt: &expires}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"data_json", "expires_at", "fetched_at"})}).Create(&resource).Error
}
