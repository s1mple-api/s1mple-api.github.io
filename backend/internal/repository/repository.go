package repository

import (
	"errors"
	"fmt"
	"time"

	"github.com/example/cs-pulse/backend/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct{ db *gorm.DB }

func Open(dsn string) (*gorm.DB, error) {
	return gorm.Open(mysql.Open(dsn), &gorm.Config{})
}

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) MigrateAndSeed() error {
	if err := r.db.AutoMigrate(&model.Article{}, &model.ArticleRead{}, &model.Trend{}, &model.Team{}, &model.Player{}, &model.Event{}, &model.Match{}); err != nil {
		return err
	}
	return nil
}

func (r *Repository) UpsertArticle(article model.Article) error {
	columns := []string{"source_url", "title", "summary", "published_at", "fetched_at", "updated_at"}
	if article.ImageURL != "" {
		columns = append(columns, "image_url")
	}
	if article.Body != "" {
		columns = append(columns, "body")
	}
	return r.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "external_id"}}, DoUpdates: clause.AssignmentColumns(columns)}).Create(&article).Error
}

func (r *Repository) UpsertMatch(match model.Match) error {
	if match.Team1 != nil {
		id, err := r.ensureTeam(*match.Team1)
		if err != nil {
			return err
		}
		match.Team1ID = &id
	}
	if match.Team2 != nil {
		id, err := r.ensureTeam(*match.Team2)
		if err != nil {
			return err
		}
		match.Team2ID = &id
	}
	if match.Event != nil {
		if err := r.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "external_id"}}, DoUpdates: clause.AssignmentColumns([]string{"name", "updated_at"})}).Create(match.Event).Error; err != nil {
			return err
		}
		var event model.Event
		if err := r.db.Where("external_id = ?", match.Event.ExternalID).First(&event).Error; err != nil {
			return err
		}
		match.EventID = &event.ID
	}
	match.Team1, match.Team2, match.Event = nil, nil, nil
	return r.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "external_id"}}, DoUpdates: clause.AssignmentColumns([]string{"event_id", "team1_id", "team2_id", "status", "scheduled_at", "team1_score", "team2_score", "source_url", "updated_at"})}).Create(&match).Error
}

func (r *Repository) UpsertEvent(event model.Event) error {
	columns := []string{
		"name", "series", "event_type", "rating", "source", "location", "prize_pool", "attending_teams",
		"valve_ranked", "source_url", "start_at", "end_at", "updated_at",
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "external_id"}},
		DoUpdates: clause.AssignmentColumns(columns),
	}).Create(&event).Error
}

// UpcomingEvents returns active and scheduled events from a calendar source.
// Events without a source URL are legacy match placeholders and are excluded.
func (r *Repository) UpcomingEvents(year int, series, eventType string, limit int) ([]model.Event, error) {
	var events []model.Event
	if limit < 1 || limit > 500 {
		limit = 250
	}
	now := time.Now().UTC()
	query := r.db.Where("source = ? AND source_url <> '' AND (end_at IS NULL OR end_at >= ?)", "hltv", now)
	if year > 0 {
		query = query.Where("YEAR(start_at) = ?", year)
	}
	if series != "" {
		query = query.Where("series = ?", series)
	}
	if eventType != "" {
		query = query.Where("rating = ? OR event_type = ?", eventType, eventType)
	}
	err := query.Order("CASE WHEN start_at IS NULL THEN 1 ELSE 0 END").
		Order("start_at ASC").Order("name ASC").Limit(limit).Find(&events).Error
	return events, err
}

func (r *Repository) ensureTeam(team model.Team) (uint, error) {
	var current model.Team
	err := r.db.Where("external_id = ?", team.ExternalID).First(&current).Error
	if err == nil {
		return current.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	if err := r.UpsertTeam(team); err != nil {
		return 0, err
	}
	return team.ID, nil
}

func (r *Repository) UpsertTeam(team model.Team) error {
	columns := []string{"name", "updated_at"}
	if team.LogoURL != "" {
		columns = append(columns, "logo_url")
	}
	if team.Country != "" {
		columns = append(columns, "country")
	}
	if team.Ranking != nil {
		columns = append(columns, "ranking", "ranking_source", "ranking_region", "points", "source_url")
		if len(team.Roster) > 0 {
			columns = append(columns, "roster")
		}
	}
	return r.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "external_id"}}, DoUpdates: clause.AssignmentColumns(columns)}).Create(&team).Error
}

func (r *Repository) ReplaceValveRankings(teams []model.Team) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		regions := make(map[string]struct{})
		for _, team := range teams {
			regions[team.RankingRegion] = struct{}{}
		}
		for region := range regions {
			if err := tx.Where("ranking_source = ? AND ranking_region = ?", "valve", region).Delete(&model.Team{}).Error; err != nil {
				return err
			}
		}
		for _, team := range teams {
			if err := tx.Create(&team).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) TeamsNeedingRosterRefresh(limit int) ([]model.Team, error) {
	var teams []model.Team
	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
	err := r.db.Where("ranking IS NOT NULL AND (ranking_source = ? OR ranking_source = '')", "hltv").
		Where("roster_checked_at IS NULL OR roster_checked_at < ?", cutoff).
		Order("ranking ASC").Limit(limit).Find(&teams).Error
	return teams, err
}

func (r *Repository) SaveTeamRoster(externalID string, roster []model.TeamPlayer, fetchError string) error {
	if fetchError != "" {
		return r.db.Model(&model.Team{}).Where("external_id = ?", externalID).Update("roster_fetch_error", fetchError).Error
	}
	now := time.Now().UTC()
	return r.db.Model(&model.Team{}).Where("external_id = ?", externalID).Updates(map[string]any{
		"roster": roster, "roster_checked_at": now, "roster_fetch_error": "",
	}).Error
}

func (r *Repository) UpsertPlayer(player model.Player) error {
	return r.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "external_id"}}, DoUpdates: clause.AssignmentColumns([]string{"name", "team_name", "country", "maps_played", "rating", "rating_label", "period", "period_label", "source_url", "updated_at"})}).Create(&player).Error
}

func (r *Repository) LatestArticles(limit int) ([]model.Article, error) {
	var articles []model.Article
	err := r.db.Order("published_at DESC").Limit(limit).Find(&articles).Error
	return articles, err
}

func (r *Repository) LatestTranslatedArticles(limit int) ([]model.Article, error) {
	var articles []model.Article
	err := r.db.Where("source IN ? AND title_zh <> ''", []string{"hltv", "steam"}).Order("published_at DESC").Limit(limit).Find(&articles).Error
	return articles, err
}

func (r *Repository) LatestWorldArticles(limit int) ([]model.Article, error) {
	var articles []model.Article
	err := r.db.Where("source = ?", "bbc").Order("published_at DESC").Limit(limit).Find(&articles).Error
	return articles, err
}

func (r *Repository) ReplaceTrends(source string, rows []model.Trend) error {
	if len(rows) == 0 {
		return fmt.Errorf("%s trend list is empty", source)
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("source = ?", source).Delete(&model.Trend{}).Error; err != nil {
			return err
		}
		return tx.Create(&rows).Error
	})
}

func (r *Repository) LatestTrends(source string, limit int) ([]model.Trend, error) {
	var rows []model.Trend
	err := r.db.Where("source = ?", source).Order("`rank` ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (r *Repository) ArticleByID(id uint) (model.Article, error) {
	var article model.Article
	err := r.db.First(&article, id).Error
	return article, err
}

func (r *Repository) RecordArticleRead(articleID uint, readerHash string) (bool, error) {
	var article model.Article
	if err := r.db.Select("id").First(&article, articleID).Error; err != nil {
		return false, err
	}
	read := model.ArticleRead{ArticleID: articleID, ReaderHash: readerHash, ReadAt: time.Now().UTC()}
	result := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&read)
	return result.RowsAffected > 0, result.Error
}

func (r *Repository) RecentArticleReads(ids []uint, since time.Time) (map[uint]int, error) {
	counts := make(map[uint]int)
	if len(ids) == 0 {
		return counts, nil
	}
	var rows []struct {
		ArticleID uint
		ReadCount int
	}
	err := r.db.Model(&model.ArticleRead{}).Select("article_id, COUNT(*) AS read_count").
		Where("article_id IN ? AND read_at >= ?", ids, since).Group("article_id").Scan(&rows).Error
	for _, row := range rows {
		counts[row.ArticleID] = row.ReadCount
	}
	return counts, err
}

func (r *Repository) SaveArticleBody(articleID uint, body, imageURL, fetchError string) error {
	now := time.Now().UTC()
	updates := map[string]any{
		"body": body, "body_fetch_error": fetchError, "body_fetched_at": now,
	}
	if imageURL != "" {
		updates["image_url"] = imageURL
	}
	return r.db.Model(&model.Article{}).Where("id = ?", articleID).Updates(updates).Error
}

func (r *Repository) SaveBodyTranslation(articleID uint, bodyZH string) error {
	return r.db.Model(&model.Article{}).Where("id = ?", articleID).Updates(map[string]any{"body_zh": bodyZH, "body_translation_error": "", "body_translation_verified": true}).Error
}

func (r *Repository) SaveBodyTranslationError(articleID uint, message string) error {
	return r.db.Model(&model.Article{}).Where("id = ?", articleID).Update("body_translation_error", message).Error
}

func (r *Repository) ArticlesNeedingTitleTranslation(limit int) ([]model.Article, error) {
	var articles []model.Article
	err := r.db.Where("COALESCE(title_zh, '') = '' AND title <> ''").Order("published_at DESC").Limit(limit).Find(&articles).Error
	return articles, err
}

func (r *Repository) ArticlesNeedingSummaryTranslation(limit int) ([]model.Article, error) {
	var articles []model.Article
	err := r.db.Where("COALESCE(title_zh, '') <> '' AND summary <> '' AND COALESCE(summary_zh, '') = ''").Order("published_at DESC").Limit(limit).Find(&articles).Error
	return articles, err
}

func (r *Repository) SaveTranslation(articleID uint, titleZH, summaryZH string) error {
	now := time.Now().UTC()
	return r.db.Model(&model.Article{}).Where("id = ?", articleID).Updates(map[string]any{
		"title_zh": titleZH, "summary_zh": summaryZH, "translated_at": now,
	}).Error
}

func (r *Repository) SaveSummaryTranslation(articleID uint, summaryZH string) error {
	return r.db.Model(&model.Article{}).Where("id = ?", articleID).Update("summary_zh", summaryZH).Error
}

func (r *Repository) RecentMatches(statuses []string, limit int) ([]model.Match, error) {
	var matches []model.Match
	query := r.db.Preload("Team1").Preload("Team2").Preload("Event").Where("matches.external_id NOT LIKE ?", "demo-%").Order("scheduled_at ASC")
	if len(statuses) > 0 {
		query = query.Where("status IN ?", statuses)
	}
	err := query.Limit(limit).Find(&matches).Error
	return matches, err
}

func (r *Repository) Rankings(source, region string, limit int) ([]model.Team, error) {
	var teams []model.Team
	query := r.db.Where("ranking IS NOT NULL AND external_id NOT LIKE ?", "demo-%")
	if source == "hltv" {
		query = query.Where("(ranking_source = ? OR ranking_source = '')", source)
	} else {
		query = query.Where("ranking_source = ?", source)
	}
	if region != "" {
		query = query.Where("ranking_region = ?", region)
	}
	err := query.Order("ranking ASC").Limit(limit).Find(&teams).Error
	return teams, err
}

func (r *Repository) Players(period string, limit int) ([]model.Player, error) {
	var players []model.Player
	query := r.db
	if period != "" {
		if period == "90d" {
			query = query.Where("period = ? OR period = ''", period)
		} else {
			query = query.Where("period = ?", period)
		}
	}
	err := query.Order("rating DESC").Order("maps_played DESC").Limit(limit).Find(&players).Error
	return players, err
}

func (r *Repository) PlayerByID(id uint) (model.Player, error) {
	var player model.Player
	err := r.db.First(&player, id).Error
	return player, err
}

type SearchResults struct {
	Articles []model.Article `json:"articles"`
	Players  []model.Player  `json:"players"`
}

func (r *Repository) Search(query string, limit int) (SearchResults, error) {
	var results SearchResults
	pattern := "%" + query + "%"
	err := r.db.Where("title LIKE ? OR title_zh LIKE ? OR summary LIKE ? OR summary_zh LIKE ?", pattern, pattern, pattern, pattern).
		Order("published_at DESC").Limit(limit).Find(&results.Articles).Error
	if err != nil {
		return results, err
	}
	err = r.db.Where("name LIKE ? OR team_name LIKE ? OR country LIKE ?", pattern, pattern, pattern).
		Order("rating DESC").Limit(limit).Find(&results.Players).Error
	return results, err
}

func (r *Repository) MarkFetchedNow() time.Time { return time.Now().UTC() }
