package service

import (
	"context"
	"html"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/example/cs-pulse/backend/internal/collector"
	"github.com/example/cs-pulse/backend/internal/config"
	"github.com/example/cs-pulse/backend/internal/model"
	"github.com/example/cs-pulse/backend/internal/repository"
)

var (
	articleHTMLTag = regexp.MustCompile(`<[^>]+>`)
	articleBBTag   = regexp.MustCompile(`\[[^]]+]`)
)

type SyncService struct {
	repo          *repository.Repository
	cfg           config.Config
	steam         *collector.SteamNewsCollector
	bbc           *collector.BBCCollector
	trends        *collector.TrendCollector
	hltv          *collector.HLTVCollector
	valve         *collector.ValveRankingsCollector
	translation   *translator
	translationMu sync.Mutex
	articleMu     sync.Mutex
	articleWork   map[uint]bool
	articleRetry  map[uint]time.Time
	periodFetchMu sync.Mutex
	statusMu      sync.RWMutex
	sourceStatus  map[string]SourceStatus
}

type SourceStatus struct {
	State       string     `json:"state"`
	Error       string     `json:"error,omitempty"`
	Count       int        `json:"count"`
	LastAttempt time.Time  `json:"lastAttempt"`
	LastSuccess *time.Time `json:"lastSuccess,omitempty"`
	RetryAfter  *time.Time `json:"retryAfter,omitempty"`
}

func NewSyncService(repo *repository.Repository, cfg config.Config) *SyncService {
	return &SyncService{repo: repo, cfg: cfg, steam: collector.NewSteamNewsCollector(), bbc: collector.NewBBCCollector(), trends: collector.NewTrendCollector(), hltv: collector.NewHLTVCollector(cfg.CollectorUserAgent), valve: collector.NewValveRankingsCollector(cfg.CollectorUserAgent), translation: newTranslator(cfg.TranslationURL), sourceStatus: make(map[string]SourceStatus), articleWork: make(map[uint]bool), articleRetry: make(map[uint]time.Time)}
}

func (s *SyncService) SourceStatus() map[string]SourceStatus {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	snapshot := make(map[string]SourceStatus, len(s.sourceStatus))
	for key, value := range s.sourceStatus {
		snapshot[key] = value
	}
	return snapshot
}

func (s *SyncService) recordSourceStatus(name string, count int, err error) {
	now := time.Now().UTC()
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	previous := s.sourceStatus[name]
	status := SourceStatus{State: "ready", Count: count, LastAttempt: now, LastSuccess: &now}
	if name == "valveRankings" {
		retryAt := now.Add(time.Hour)
		status.RetryAfter = &retryAt
	} else if name == "events" {
		retryAt := now.Add(30 * time.Minute)
		status.RetryAfter = &retryAt
	}
	if err != nil {
		status.State = "unavailable"
		status.Error = err.Error()
		status.LastSuccess = previous.LastSuccess
		status.Count = previous.Count
		cooldown := 15 * time.Minute
		if strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "authorized access") {
			cooldown = time.Hour
		}
		retryAt := now.Add(cooldown)
		status.RetryAfter = &retryAt
	}
	s.sourceStatus[name] = status
}

func (s *SyncService) sourceCanBeRetried(name string) bool {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	status, ok := s.sourceStatus[name]
	return !ok || status.RetryAfter == nil || !time.Now().UTC().Before(*status.RetryAfter)
}

func (s *SyncService) SyncOnce() {
	s.syncSteam()
	s.syncBBC()
	s.syncBaiduTrends()
	s.syncWeiboTrends()
	s.syncValveRankings()
	if s.cfg.EnableHLTVCollector {
		s.syncHLTV()
		s.syncHLTVEvents()
		s.syncRankingsAndPlayers()
	}
	s.translateRecent(10)
}

func (s *SyncService) syncValveRankings() {
	if !s.sourceCanBeRetried("valveRankings") {
		return
	}
	teams, err := s.valve.Fetch()
	if err != nil {
		log.Printf("Valve VRS sync: %v", err)
		s.recordSourceStatus("valveRankings", 0, err)
		return
	}
	saveErr := s.repo.ReplaceValveRankings(teams)
	log.Printf("Valve VRS updated: %d team rows", len(teams))
	s.recordSourceStatus("valveRankings", len(teams), saveErr)
}

// ArticleDetail fetches a public article body only when that internal detail
// page is opened; previously cached body text is served from MySQL.
func (s *SyncService) ArticleDetail(id uint) (model.Article, error) {
	article, err := s.repo.ArticleByID(id)
	if err != nil {
		return article, err
	}
	if article.Body == "" && (article.Source == "hltv" || article.Source == "bbc") && (article.BodyFetchedAt == nil || time.Since(*article.BodyFetchedAt) > 24*time.Hour) {
		var body, imageURL string
		var fetchErr error
		if article.Source == "bbc" {
			body, imageURL, fetchErr = s.bbc.FetchArticleBody(article.SourceURL)
		} else {
			body, fetchErr = s.hltv.FetchArticleBody(article.SourceURL)
		}
		errorText := ""
		if fetchErr != nil {
			errorText = fetchErr.Error()
			log.Printf("%s article detail %s: %v", article.Source, article.ExternalID, fetchErr)
		} else {
			body = plainArticleBody(body)
		}
		if saveErr := s.repo.SaveArticleBody(article.ID, body, imageURL, errorText); saveErr != nil {
			return article, saveErr
		}
		article, err = s.repo.ArticleByID(id)
		if err != nil {
			return article, err
		}
	}
	if article.Source == "steam" {
		if article.Body == "" {
			article.Body = article.Summary
		}
		article.Body = plainArticleBody(article.Body)
	}
	needsVerifiedBBCTranslation := article.Source == "bbc" && !article.BodyTranslationVerified
	if article.Body != "" && (article.BodyZH == "" || needsVerifiedBBCTranslation) {
		started := s.queueBodyTranslation(article)
		if needsVerifiedBBCTranslation {
			article.BodyZH = ""
		}
		if started {
			article.BodyTranslationError = ""
		}
	}
	return article, nil
}

func (s *SyncService) queueBodyTranslation(article model.Article) bool {
	s.articleMu.Lock()
	if s.articleWork[article.ID] || time.Now().Before(s.articleRetry[article.ID]) {
		s.articleMu.Unlock()
		return false
	}
	s.articleWork[article.ID] = true
	s.articleMu.Unlock()
	if err := s.repo.SaveBodyTranslationError(article.ID, ""); err != nil {
		log.Printf("clear article translation error %s: %v", article.ExternalID, err)
	}
	go func() {
		defer func() {
			s.articleMu.Lock()
			delete(s.articleWork, article.ID)
			s.articleMu.Unlock()
		}()
		translated, err := s.translation.translateLong(context.Background(), article.Body)
		if err != nil {
			log.Printf("translate article body %s: %v", article.ExternalID, err)
			message := err.Error()
			if len(message) > 500 {
				message = message[:500]
			}
			if saveErr := s.repo.SaveBodyTranslationError(article.ID, message); saveErr != nil {
				log.Printf("save article translation error %s: %v", article.ExternalID, saveErr)
			}
			s.articleMu.Lock()
			s.articleRetry[article.ID] = time.Now().Add(time.Hour)
			s.articleMu.Unlock()
			return
		}
		if err := s.repo.SaveBodyTranslation(article.ID, translated); err != nil {
			log.Printf("save article body translation %s: %v", article.ExternalID, err)
		}
	}()
	return true
}

func (s *SyncService) PlayersForPeriod(period string, limit int) ([]model.Player, error) {
	if period == "" {
		period = "90d"
	}
	players, err := s.repo.Players(period, limit)
	if err != nil || len(players) > 0 || !s.cfg.EnableHLTVCollector {
		return players, err
	}
	s.periodFetchMu.Lock()
	defer s.periodFetchMu.Unlock()
	players, err = s.repo.Players(period, limit)
	if err != nil || len(players) > 0 || !s.sourceCanBeRetried("players") {
		return players, err
	}
	rows, fetchErr := s.hltv.FetchPlayerRankingForPeriod(period)
	if fetchErr != nil {
		s.recordSourceStatus("players", 0, fetchErr)
		return players, nil
	}
	var saveErr error
	for _, player := range rows {
		if err := s.repo.UpsertPlayer(player); err != nil {
			saveErr = err
		}
	}
	s.recordSourceStatus("players", len(rows), saveErr)
	return s.repo.Players(period, limit)
}

func plainArticleBody(body string) string {
	body = html.UnescapeString(body)
	body = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/li|/h[1-6])\s*/?>`).ReplaceAllString(body, "\n\n")
	body = articleHTMLTag.ReplaceAllString(body, " ")
	body = regexp.MustCompile(`(?i)\[/?(?:p|b|i|u|h[1-6]|quote|list|\*|hr)[^]]*]`).ReplaceAllString(body, "\n\n")
	body = articleBBTag.ReplaceAllString(body, " ")
	lines := strings.Split(body, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.TrimSpace(regexp.MustCompile(`\n{3,}`).ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

func (s *SyncService) translateRecent(limit int) {
	if !s.translationMu.TryLock() {
		return
	}
	defer s.translationMu.Unlock()
	articles, err := s.repo.ArticlesNeedingTitleTranslation(limit)
	if err != nil {
		log.Printf("translation queue: %v", err)
		return
	}
	for _, article := range articles {
		titleZH, translateErr := s.translation.translate(context.Background(), article.Title)
		if translateErr != nil {
			log.Printf("translate title %s: %v", article.ExternalID, translateErr)
			return
		}
		if saveErr := s.repo.SaveTranslation(article.ID, titleZH, ""); saveErr != nil {
			log.Printf("save translation %s: %v", article.ExternalID, saveErr)
		}
		time.Sleep(time.Second)
	}
	summaries, err := s.repo.ArticlesNeedingSummaryTranslation(2)
	if err != nil {
		log.Printf("summary translation queue: %v", err)
		return
	}
	for _, article := range summaries {
		summaryZH, translateErr := s.translation.translate(context.Background(), article.Summary)
		if translateErr != nil {
			log.Printf("translate summary %s: %v", article.ExternalID, translateErr)
			return
		}
		if saveErr := s.repo.SaveSummaryTranslation(article.ID, summaryZH); saveErr != nil {
			log.Printf("save summary translation %s: %v", article.ExternalID, saveErr)
		}
		time.Sleep(time.Second)
	}
}

func (s *SyncService) syncSteam() {
	if articles, err := s.steam.Fetch(); err != nil {
		log.Printf("steam sync: %v", err)
		s.recordSourceStatus("steamNews", 0, err)
	} else {
		var saveErr error
		for _, article := range articles {
			if err := s.repo.UpsertArticle(article); err != nil {
				log.Printf("steam article save: %v", err)
				saveErr = err
			}
		}
		s.recordSourceStatus("steamNews", len(articles), saveErr)
	}
}

func (s *SyncService) syncBBC() {
	if !s.sourceCanBeRetried("bbcNews") {
		return
	}
	articles, err := s.bbc.FetchWorldNews()
	if err != nil {
		log.Printf("BBC world news sync: %v", err)
		s.recordSourceStatus("bbcNews", 0, err)
		return
	}
	var saveErr error
	for _, article := range articles {
		if err := s.repo.UpsertArticle(article); err != nil {
			log.Printf("BBC article save: %v", err)
			saveErr = err
		}
	}
	log.Printf("BBC world news updated: %d articles", len(articles))
	s.recordSourceStatus("bbcNews", len(articles), saveErr)
}

func (s *SyncService) syncBaiduTrends() {
	if !s.sourceCanBeRetried("baiduTrends") {
		return
	}
	rows, err := s.trends.FetchBaidu()
	if err == nil {
		err = s.repo.ReplaceTrends("baidu", rows)
	}
	if err != nil {
		log.Printf("Baidu trends sync: %v", err)
	} else {
		log.Printf("Baidu trends updated: %d rows", len(rows))
	}
	s.recordSourceStatus("baiduTrends", len(rows), err)
}

func (s *SyncService) syncWeiboTrends() {
	if !s.sourceCanBeRetried("weiboTrends") {
		return
	}
	rows, err := s.trends.FetchWeibo()
	if err == nil {
		err = s.repo.ReplaceTrends("weibo", rows)
	}
	if err != nil {
		log.Printf("Weibo trends sync: %v", err)
	} else {
		log.Printf("Weibo trends updated: %d rows", len(rows))
	}
	s.recordSourceStatus("weiboTrends", len(rows), err)
}

func (s *SyncService) syncHLTV() {
	if s.sourceCanBeRetried("news") {
		if articles, err := s.hltv.FetchLatestNews(); err != nil {
			log.Printf("hltv news sync: %v", err)
			s.recordSourceStatus("news", 0, err)
		} else {
			var saveErr error
			for _, article := range articles {
				if err := s.repo.UpsertArticle(article); err != nil {
					log.Printf("hltv article save: %v", err)
					saveErr = err
				}
			}
			s.recordSourceStatus("news", len(articles), saveErr)
		}
	}
	if s.sourceCanBeRetried("matches") {
		if matches, err := s.hltv.FetchUpcomingMatches(); err != nil {
			log.Printf("hltv match sync: %v", err)
			s.recordSourceStatus("matches", 0, err)
		} else {
			var saveErr error
			for _, match := range matches {
				if err := s.repo.UpsertMatch(match); err != nil {
					log.Printf("hltv match save: %v", err)
					saveErr = err
				}
			}
			s.recordSourceStatus("matches", len(matches), saveErr)
		}
	}
}

func (s *SyncService) syncHLTVEvents() {
	if !s.sourceCanBeRetried("events") {
		return
	}
	events, err := s.hltv.FetchUpcomingEvents()
	if err != nil {
		log.Printf("hltv event sync: %v", err)
		s.recordSourceStatus("events", 0, err)
		return
	}
	var saveErr error
	for _, event := range events {
		if err := s.repo.UpsertEvent(event); err != nil {
			log.Printf("hltv event save: %v", err)
			saveErr = err
		}
	}
	log.Printf("hltv upcoming events updated: %d events", len(events))
	s.recordSourceStatus("events", len(events), saveErr)
}

func (s *SyncService) syncRankingsAndPlayers() {
	if s.sourceCanBeRetried("rankings") {
		teams, err := s.hltv.FetchTeamRanking()
		if err != nil {
			log.Printf("hltv team ranking sync: %v", err)
			s.recordSourceStatus("rankings", 0, err)
		} else {
			var saveErr error
			for _, team := range teams {
				if err := s.repo.UpsertTeam(team); err != nil {
					log.Printf("hltv team ranking save: %v", err)
					saveErr = err
				}
			}
			log.Printf("hltv team rankings updated: %d teams", len(teams))
			s.recordSourceStatus("rankings", len(teams), saveErr)
			if saveErr == nil {
				s.syncTeamRosters()
			}
		}
	}
	if s.sourceCanBeRetried("players") {
		players, err := s.hltv.FetchPlayerRanking()
		if err != nil {
			log.Printf("hltv player ranking sync: %v", err)
			s.recordSourceStatus("players", 0, err)
		} else {
			var saveErr error
			for _, player := range players {
				if err := s.repo.UpsertPlayer(player); err != nil {
					log.Printf("hltv player ranking save: %v", err)
					saveErr = err
				}
			}
			log.Printf("hltv player ratings updated: %d players", len(players))
			s.recordSourceStatus("players", len(players), saveErr)
		}
	}
}

func (s *SyncService) syncTeamRosters() {
	if !s.sourceCanBeRetried("teamRoster") {
		return
	}
	teams, err := s.repo.TeamsNeedingRosterRefresh(10)
	if err != nil {
		s.recordSourceStatus("teamRoster", 0, err)
		return
	}
	if len(teams) == 0 {
		return
	}
	updated := 0
	for _, team := range teams {
		players, fetchErr := s.hltv.FetchTeamRoster(team.SourceURL)
		if fetchErr != nil {
			_ = s.repo.SaveTeamRoster(team.ExternalID, nil, fetchErr.Error())
			log.Printf("hltv team roster %s: %v", team.Name, fetchErr)
			s.recordSourceStatus("teamRoster", updated, fetchErr)
			return
		}
		players = mergeTeamRoster(team.Roster, players)
		if saveErr := s.repo.SaveTeamRoster(team.ExternalID, players, ""); saveErr != nil {
			log.Printf("hltv team roster save %s: %v", team.Name, saveErr)
			s.recordSourceStatus("teamRoster", updated, saveErr)
			return
		}
		updated++
	}
	s.recordSourceStatus("teamRoster", updated, nil)
}

func mergeTeamRoster(current, profile []model.TeamPlayer) []model.TeamPlayer {
	byID := make(map[string]model.TeamPlayer, len(profile))
	byName := make(map[string]model.TeamPlayer, len(profile))
	for _, player := range profile {
		if player.ID != "" {
			byID[player.ID] = player
		}
		byName[strings.ToLower(player.Name)] = player
	}
	merged := make([]model.TeamPlayer, 0, len(current)+len(profile))
	seen := make(map[string]struct{})
	for _, player := range current {
		match, ok := byID[player.ID]
		if !ok {
			match, ok = byName[strings.ToLower(player.Name)]
		}
		if ok {
			if match.AvatarURL != "" {
				player.AvatarURL = match.AvatarURL
			}
			if player.ProfileURL == "" {
				player.ProfileURL = match.ProfileURL
			}
		}
		key := strings.ToLower(player.Name)
		if key == "" {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, player)
	}
	for _, player := range profile {
		key := strings.ToLower(player.Name)
		if _, exists := seen[key]; exists || key == "" {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, player)
	}
	return merged
}

func (s *SyncService) Run() {
	steamTicker := time.NewTicker(15 * time.Minute)
	defer steamTicker.Stop()
	bbcTicker := time.NewTicker(15 * time.Minute)
	defer bbcTicker.Stop()
	trendsTicker := time.NewTicker(10 * time.Minute)
	defer trendsTicker.Stop()
	hltvTicker := time.NewTicker(5 * time.Minute)
	defer hltvTicker.Stop()
	rankingsTicker := time.NewTicker(time.Hour)
	defer rankingsTicker.Stop()
	eventsTicker := time.NewTicker(30 * time.Minute)
	defer eventsTicker.Stop()
	translationTicker := time.NewTicker(time.Minute)
	defer translationTicker.Stop()
	for {
		select {
		case <-steamTicker.C:
			s.syncSteam()
		case <-bbcTicker.C:
			s.syncBBC()
		case <-trendsTicker.C:
			s.syncBaiduTrends()
			s.syncWeiboTrends()
		case <-hltvTicker.C:
			if s.cfg.EnableHLTVCollector {
				s.syncHLTV()
			}
		case <-rankingsTicker.C:
			s.syncValveRankings()
			if s.cfg.EnableHLTVCollector {
				s.syncRankingsAndPlayers()
			}
		case <-eventsTicker.C:
			if s.cfg.EnableHLTVCollector {
				s.syncHLTVEvents()
			}
		case <-translationTicker.C:
			s.translateRecent(8)
		}
	}
}
