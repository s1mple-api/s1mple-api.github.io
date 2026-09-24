package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/example/cs-pulse/backend/internal/config"
	"github.com/example/cs-pulse/backend/internal/repository"
	"github.com/example/cs-pulse/backend/internal/service"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var readerIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

func NewRouter(repo *repository.Repository, syncer *service.SyncService, cfg config.Config) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	// The local development server does not sit behind a trusted reverse proxy.
	if err := r.SetTrustedProxies(nil); err != nil {
		panic(err)
	}
	r.Use(cors.New(cors.Config{AllowOrigins: []string{cfg.CORSOrigin}, AllowMethods: []string{"GET", "POST"}, AllowHeaders: []string{"Content-Type"}}))
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	api := r.Group("/api/v1")
	api.GET("/dashboard", func(c *gin.Context) {
		articles, err := repo.LatestTranslatedArticles(48)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		worldArticles, err := repo.LatestWorldArticles(48)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		baiduTrends, err := repo.LatestTrends("baidu", 30)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		weiboTrends, err := repo.LatestTrends("weibo", 30)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		matches, err := repo.RecentMatches([]string{"scheduled", "live"}, 8)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		events, err := repo.UpcomingEvents(0, "", "", 500)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		rankings, err := repo.Rankings("hltv", "", 50)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		valveRankings, err := repo.Rankings("valve", "global", 50)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		players, err := repo.Players("90d", 30)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		allNews := append(articles[:len(articles):len(articles)], worldArticles...)
		ids := make([]uint, 0, len(allNews))
		for _, article := range allNews {
			ids = append(ids, article.ID)
		}
		reads, err := repo.RecentArticleReads(ids, time.Now().UTC().Add(-7*24*time.Hour))
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		newsFeed := service.RankNews(allNews, append(baiduTrends[:len(baiduTrends):len(baiduTrends)], weiboTrends...), reads, time.Now().UTC())
		c.JSON(http.StatusOK, gin.H{"articles": articles, "worldArticles": worldArticles, "newsFeed": newsFeed, "baiduTrends": baiduTrends, "weiboTrends": weiboTrends, "matches": matches, "events": events, "rankings": rankings, "valveRankings": valveRankings, "players": players, "sourceStatus": syncer.SourceStatus()})
	})
	api.GET("/world-news", func(c *gin.Context) {
		rows, err := repo.LatestWorldArticles(intQuery(c, "limit", 30))
		respond(c, rows, err)
	})
	api.GET("/hot-search", func(c *gin.Context) {
		source := c.DefaultQuery("source", "baidu")
		if source != "baidu" && source != "weibo" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "source must be baidu or weibo"})
			return
		}
		rows, err := repo.LatestTrends(source, intQuery(c, "limit", 30))
		respond(c, rows, err)
	})
	api.GET("/articles", func(c *gin.Context) {
		limit := intQuery(c, "limit", 20)
		rows, err := repo.LatestArticles(limit)
		respond(c, rows, err)
	})
	api.GET("/articles/:id", func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid article id"})
			return
		}
		article, err := syncer.ArticleDetail(uint(id))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "article not found"})
			return
		}
		c.JSON(http.StatusOK, article)
	})
	api.POST("/articles/:id/read", func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		var input struct {
			ReaderID string `json:"readerId"`
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 512)
		if err != nil || id == 0 || c.ShouldBindJSON(&input) != nil || !readerIDPattern.MatchString(input.ReaderID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid article or reader"})
			return
		}
		digest := sha256.Sum256([]byte(strings.ToLower(input.ReaderID)))
		counted, err := repo.RecordArticleRead(uint(id), hex.EncodeToString(digest[:]))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "article not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not record read"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"counted": counted})
	})
	api.GET("/matches", func(c *gin.Context) {
		rows, err := repo.RecentMatches(nil, intQuery(c, "limit", 20))
		respond(c, rows, err)
	})
	api.GET("/events", func(c *gin.Context) {
		year := 0
		if rawYear := c.Query("year"); rawYear != "" {
			parsedYear, err := strconv.Atoi(rawYear)
			if err != nil || parsedYear < 2000 || parsedYear > 2200 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "year must be between 2000 and 2200"})
				return
			}
			year = parsedYear
		}
		eventType := c.Query("rating")
		if eventType == "" {
			eventType = c.Query("type")
		}
		rows, err := repo.UpcomingEvents(year, strings.TrimSpace(c.Query("series")), strings.TrimSpace(eventType), eventLimitQuery(c))
		respond(c, rows, err)
	})
	api.GET("/players", func(c *gin.Context) {
		period := c.DefaultQuery("period", "90d")
		if period != "30d" && period != "90d" && period != "180d" && period != "365d" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "period must be one of 30d, 90d, 180d, 365d"})
			return
		}
		rows, err := syncer.PlayersForPeriod(period, intQuery(c, "limit", 50))
		respond(c, rows, err)
	})
	api.GET("/rankings", func(c *gin.Context) {
		source := c.DefaultQuery("source", "hltv")
		region := c.Query("region")
		if source != "hltv" && source != "valve" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "source must be hltv or valve"})
			return
		}
		if source == "valve" && region == "" {
			region = "global"
		}
		if source == "hltv" {
			region = ""
		} else if region != "global" && region != "europe" && region != "americas" && region != "asia" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Valve ranking region"})
			return
		}
		rows, err := repo.Rankings(source, region, intQuery(c, "limit", 100))
		respond(c, rows, err)
	})
	api.GET("/players/:id", func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid player id"})
			return
		}
		player, err := repo.PlayerByID(uint(id))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "player not found"})
			return
		}
		c.JSON(http.StatusOK, player)
	})
	api.GET("/search", func(c *gin.Context) {
		query := strings.TrimSpace(c.Query("q"))
		if len([]rune(query)) < 2 {
			c.JSON(http.StatusOK, gin.H{"articles": []any{}, "players": []any{}})
			return
		}
		results, err := repo.Search(query, 8)
		respond(c, results, err)
	})
	api.POST("/sync", func(c *gin.Context) {
		go syncer.SyncOnce()
		c.JSON(http.StatusAccepted, gin.H{"status": "sync_started"})
	})
	return r
}

func intQuery(c *gin.Context, name string, fallback int) int {
	value, err := strconv.Atoi(c.DefaultQuery(name, strconv.Itoa(fallback)))
	if err != nil || value < 1 || value > 100 {
		return fallback
	}
	return value
}
func eventLimitQuery(c *gin.Context) int {
	value, err := strconv.Atoi(c.DefaultQuery("limit", "250"))
	if err != nil || value < 1 || value > 500 {
		return 250
	}
	return value
}
func respond(c *gin.Context, value any, err error) {
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, value)
}
