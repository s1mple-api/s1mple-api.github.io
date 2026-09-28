package repository

import (
	"encoding/json"
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
