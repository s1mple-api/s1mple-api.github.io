package collector

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/example/cs-pulse/backend/internal/model"
)

// HLTVCollector 只做普通页面请求，不包含验证码、代理或反反爬绕过逻辑。
// 选择器会随 HLTV 页面结构调整而变化，因此被设计为可替换的采集适配器。
type HLTVCollector struct {
	client      *http.Client
	userAgent   string
	requestMu   sync.Mutex
	lastRequest time.Time
}

var (
	newsPath       = regexp.MustCompile(`^/news/(\d+)(?:/|$)`)
	newsDateInText = regexp.MustCompile(`20\d{2}-\d{2}-\d{2}`)
	teamPath       = regexp.MustCompile(`^/team/(\d+)(?:/|$)`)
	playerPath     = regexp.MustCompile(`^/(?:stats/)?players?/(\d+)(?:/|$)`)
	matchPath      = regexp.MustCompile(`^/matches/(\d+)(?:/|$)`)
	eventPath      = regexp.MustCompile(`^/events/(\d+)(?:/|$)`)
	eventYear      = regexp.MustCompile(`20\d{2}`)
	eventDateRange = regexp.MustCompile(`(?i)\b(Jan(?:uary)?|Feb(?:ruary)?|Mar(?:ch)?|Apr(?:il)?|May|June?|July?|Aug(?:ust)?|Sep(?:tember)?|Oct(?:ober)?|Nov(?:ember)?|Dec(?:ember)?)\s+(\d{1,2})(?:st|nd|rd|th)?(?:\s*[-–]\s*(?:(Jan(?:uary)?|Feb(?:ruary)?|Mar(?:ch)?|Apr(?:il)?|May|June?|July?|Aug(?:ust)?|Sep(?:tember)?|Oct(?:ober)?|Nov(?:ember)?|Dec(?:ember)?)\s*)?(\d{1,2})(?:st|nd|rd|th)?)?(?:,?\s*(20\d{2}))?`)
	eventPrize     = regexp.MustCompile(`\$\s?[\d,]+(?:\.\d+)?(?:\s?[KMB])?`)
	eventTeams     = regexp.MustCompile(`(?i)\b(\d+)\s*teams\b`)
	firstNumber    = regexp.MustCompile(`[+-]?\d+(?:\.\d+)?`)
)

func NewHLTVCollector(userAgent string) *HLTVCollector {
	return &HLTVCollector{client: &http.Client{Timeout: 15 * time.Second}, userAgent: userAgent}
}

// FetchLatestNews 从 HLTV 的新闻归档页取最新条目。它只请求一个列表页，
// 并以新闻 URL 去重；不下载新闻正文，也不尝试规避访问限制。
func (c *HLTVCollector) FetchLatestNews() ([]model.Article, error) {
	doc, err := c.getDocument("https://www.hltv.org/news/archive")
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	seen := make(map[string]struct{})
	articles := make([]model.Article, 0, 20)
	doc.Find(`a[href*="/news/"]`).Each(func(index int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		matched := newsPath.FindStringSubmatch(href)
		if !exists || len(matched) != 2 {
			return
		}
		if _, exists := seen[href]; exists {
			return
		}
		seen[href] = struct{}{}

		rawText := cleanText(s.Text())
		title := cleanText(s.Find(".newstext-title, .article-title").First().Text())
		if title == "" {
			title = rawText
			if position := newsDateInText.FindStringIndex(title); position != nil {
				title = strings.TrimSpace(title[:position[0]])
			}
		}
		if len([]rune(title)) < 5 || len([]rune(title)) > 350 {
			return
		}
		summary := cleanText(s.Find(".newstext-desc, .article-desc").First().Text())
		publishedAt := parseNewsTime(s, rawText, now)
		if publishedAt.IsZero() {
			// 列表页未提供机器可读时间时，保留页面顺序而非把历史条目都标成“刚刚”。
			publishedAt = now.Add(-time.Duration(index) * time.Minute)
		}
		articles = append(articles, model.Article{
			Source: "hltv", ExternalID: "hltv-news-" + matched[1], SourceURL: absoluteHLTVURL(href),
			Title: title, Summary: summary, PublishedAt: publishedAt, FetchedAt: now,
		})
	})
	return articles, nil
}

func (c *HLTVCollector) FetchUpcomingMatches() ([]model.Match, error) {
	doc, err := c.getDocument("https://www.hltv.org/matches")
	if err != nil {
		return nil, err
	}
	var matches []model.Match
	doc.Find(".liveMatch-container, .upcomingMatch, a.match").Each(func(_ int, s *goquery.Selection) {
		link := s
		if !s.Is("a") {
			link = s.Find("a.a-reset, a.match, a[href*='/matches/']").First()
		}
		href, ok := link.Attr("href")
		if !ok {
			return
		}
		matched := matchPath.FindStringSubmatch(href)
		if len(matched) != 2 {
			return
		}
		teamNames := s.Find(".matchTeamName")
		if teamNames.Length() < 2 {
			teamNames = s.Find(".team1 .team, .team2 .team")
		}
		first, second := "", ""
		if teamNames.Length() >= 2 {
			first, second = cleanText(teamNames.Eq(0).Text()), cleanText(teamNames.Eq(1).Text())
		}
		if first == "" && second == "" {
			title := cleanText(s.Find(".matchInfoEmpty").Text())
			if title == "" {
				return
			}
		}
		team1ID := dataID(s, "team1")
		team2ID := dataID(s, "team2")
		match := model.Match{ExternalID: "hltv-match-" + matched[1], Status: "scheduled", SourceURL: absoluteHLTVURL(href), BestOf: 3}
		if strings.EqualFold(cleanText(s.Find(".matchTime").Text()), "LIVE") {
			match.Status = "live"
		}
		if raw, ok := s.Find(".matchTime").First().Attr("data-unix"); ok {
			if unix, err := strconv.ParseInt(raw, 10, 64); err == nil && unix > 0 {
				at := time.UnixMilli(unix).UTC()
				match.ScheduledAt = &at
			}
		}
		if team1ID != "" && first != "" {
			match.Team1 = &model.Team{ExternalID: "hltv-team-" + team1ID, Name: first}
		}
		if team2ID != "" && second != "" {
			match.Team2 = &model.Team{ExternalID: "hltv-team-" + team2ID, Name: second}
		}
		if eventName := strings.TrimSpace(s.Find(".matchEventLogo").First().AttrOr("title", "")); eventName != "" {
			eventID := strings.ToLower(strings.Trim(strings.Join(strings.Fields(eventName), "-"), "-"))
			match.EventName = eventName
			match.Event = &model.Event{ExternalID: "hltv-event-" + eventID, Name: eventName}
		}
		matches = append(matches, match)
	})
	return matches, nil
}

// FetchUpcomingEvents reads HLTV's public event calendar using a normal page
// request. It intentionally does not try to evade access controls.
func (c *HLTVCollector) FetchUpcomingEvents() ([]model.Event, error) {
	doc, err := c.getDocument("https://www.hltv.org/events")
	if err != nil {
		return nil, err
	}
	var events []model.Event
	seen := make(map[string]struct{})
	currentYear := time.Now().UTC().Year()
	upcomingSection := false
	doc.Find("h1, h2, h3, h4, a[href*='/events/']").Each(func(_ int, node *goquery.Selection) {
		text := cleanText(node.Text())
		if node.Is("h1, h2, h3, h4") {
			lower := strings.ToLower(text)
			if strings.Contains(lower, "upcoming events") {
				upcomingSection = true
			} else if strings.Contains(lower, "played events") || strings.Contains(lower, "ongoing events") {
				upcomingSection = false
			}
			if year := eventYear.FindString(text); year != "" {
				if parsed, parseErr := strconv.Atoi(year); parseErr == nil {
					currentYear = parsed
				}
			}
			return
		}
		if !upcomingSection {
			return
		}
		href, exists := node.Attr("href")
		if !exists {
			return
		}
		matched := eventPath.FindStringSubmatch(href)
		if len(matched) != 2 {
			return
		}
		if _, exists := seen[matched[1]]; exists {
			return
		}
		seen[matched[1]] = struct{}{}

		card := eventCard(node)
		cardText := cleanText(card.Text())
		if cardText == "" {
			cardText = text
		}
		name := eventName(node, card, cardText)
		if name == "" {
			return
		}
		year := currentYear
		if match := eventYear.FindString(name); match != "" {
			if parsed, parseErr := strconv.Atoi(match); parseErr == nil {
				year = parsed
			}
		}
		startAt, endAt := parseEventDates(cardText, year, time.Now().UTC())
		series := eventSeries(name)
		eventType := eventTypeFromText(cardText)
		location := cleanText(card.Find(".event-location, .eventLocation, .location").First().Text())
		prizePool := eventPrize.FindString(cardText)
		teamCount := 0
		if teamMatch := eventTeams.FindStringSubmatch(cardText); len(teamMatch) == 2 {
			teamCount, _ = strconv.Atoi(teamMatch[1])
		}
		lowerText := strings.ToLower(cardText)
		valveRanked := strings.Contains(lowerText, "valve ranked") || strings.Contains(strings.ToLower(card.AttrOr("class", "")), "valve-ranked")
		events = append(events, model.Event{
			ExternalID: "hltv-event-" + matched[1], Name: name, Series: series, EventType: eventType, Source: "hltv",
			Location: location, PrizePool: prizePool, AttendingTeams: teamCount, ValveRanked: valveRanked,
			SourceURL: absoluteHLTVURL(href), StartAt: startAt, EndAt: endAt,
		})
	})
	return events, nil
}

func eventCard(link *goquery.Selection) *goquery.Selection {
	best := link
	current := link
	for depth := 0; depth < 6; depth++ {
		parent := current.Parent()
		if parent.Length() == 0 {
			break
		}
		text := cleanText(parent.Text())
		if len([]rune(text)) > 700 {
			break
		}
		best = parent
		if eventDateRange.MatchString(text) || parent.Find("[data-unix], time").Length() > 0 {
			return parent
		}
		current = parent
	}
	return best
}

func eventName(link, card *goquery.Selection, rawText string) string {
	for _, selector := range []string{".event-name", ".eventName", ".event-name-container", ".event-title", ".name"} {
		value := cleanText(card.Find(selector).First().Text())
		if value != "" {
			return value
		}
	}
	for _, attr := range []string{"title", "aria-label"} {
		if value := cleanText(link.AttrOr(attr, "")); value != "" {
			return value
		}
	}
	value := cleanText(link.Text())
	if value == "" {
		value = rawText
	}
	if match := eventDateRange.FindStringIndex(value); match != nil {
		value = value[:match[0]]
	}
	if separator := strings.Index(value, "|"); separator >= 0 {
		value = value[:separator]
	}
	if prize := eventPrize.FindStringIndex(value); prize != nil {
		value = value[:prize[0]]
	}
	category := regexp.MustCompile(`(?i)\b(?:major|intl\.?\s*lan|international lan|reg\.?\s*lan|regional lan|local lan|online|other)\b.*$`)
	value = category.ReplaceAllString(value, "")
	value = strings.TrimSpace(strings.Trim(value, "|·–- "))
	value = regexp.MustCompile(`(?i)\b(?:tba|teams?|prize|date)\b.*$`).ReplaceAllString(value, "")
	return cleanText(value)
}

func eventTypeFromText(value string) string {
	lower := strings.ToLower(value)
	switch {
	case strings.Contains(lower, "major"):
		return "Major"
	case strings.Contains(lower, "international lan"), strings.Contains(lower, "intl. lan"), strings.Contains(lower, "intl lan"):
		return "International LAN"
	case strings.Contains(lower, "regional lan"), strings.Contains(lower, "reg. lan"), strings.Contains(lower, "reg lan"):
		return "Regional LAN"
	case strings.Contains(lower, "local lan"):
		return "Local LAN"
	case strings.Contains(lower, "online"):
		return "Online"
	case strings.Contains(lower, "other"):
		return "Other"
	default:
		return ""
	}
}

func eventSeries(name string) string {
	lower := strings.ToLower(name)
	series := []struct{ match, label string }{
		{"esl pro league", "ESL Pro League"}, {"esl challenger league", "ESL Challenger League"},
		{"iem ", "IEM"}, {"intel extreme masters", "IEM"}, {"blast", "BLAST"}, {"pgl", "PGL"},
		{"cct", "CCT"}, {"esea", "ESEA"}, {"thunderpick", "Thunderpick"}, {"fissure", "FISSURE"},
		{"stake", "Stake"}, {"esports world cup", "Esports World Cup"}, {"european pro league", "European Pro League"},
	}
	for _, item := range series {
		if strings.Contains(lower, item.match) {
			return item.label
		}
	}
	base := regexp.MustCompile(`(?i)\s+(?:20\d{2}|season\s+\d+|series\s+\d+|episode\s+\d+|cup\s+[ivx\d]+|#\s?\d+).*$`).ReplaceAllString(name, "")
	base = cleanText(strings.TrimSpace(base))
	if base != "" {
		return base
	}
	return "单站赛事"
}

func parseEventDates(value string, year int, now time.Time) (*time.Time, *time.Time) {
	match := eventDateRange.FindStringSubmatch(value)
	if len(match) == 0 {
		return nil, nil
	}
	startMonth, ok := eventMonth(match[1])
	if !ok {
		return nil, nil
	}
	startDay, _ := strconv.Atoi(match[2])
	endMonth, endDay := startMonth, startDay
	if match[3] != "" {
		if parsed, valid := eventMonth(match[3]); valid {
			endMonth = parsed
		}
	}
	if match[4] != "" {
		endDay, _ = strconv.Atoi(match[4])
	}
	if match[5] != "" {
		year, _ = strconv.Atoi(match[5])
	} else if year == now.Year() && startMonth < int(now.Month()) {
		year++
	}
	if startDay < 1 || startDay > 31 || endDay < 1 || endDay > 31 {
		return nil, nil
	}
	start := time.Date(year, time.Month(startMonth), startDay, 0, 0, 0, 0, time.UTC)
	endYear := year
	if endMonth < startMonth {
		endYear++
	}
	end := time.Date(endYear, time.Month(endMonth), endDay, 23, 59, 59, 0, time.UTC)
	return &start, &end
}

func eventMonth(value string) (int, bool) {
	if len(value) < 3 {
		return 0, false
	}
	month, ok := map[string]time.Month{
		"jan": time.January, "feb": time.February, "mar": time.March, "apr": time.April,
		"may": time.May, "jun": time.June, "jul": time.July, "aug": time.August,
		"sep": time.September, "oct": time.October, "nov": time.November, "dec": time.December,
	}[strings.ToLower(value[:3])]
	return int(month), ok
}

// FetchTeamRanking parses the current public HLTV team ranking page.
func (c *HLTVCollector) FetchTeamRanking() ([]model.Team, error) {
	doc, err := c.getDocument("https://www.hltv.org/ranking/teams")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	teams := make([]model.Team, 0, 50)
	doc.Find(".ranked-team").Each(func(_ int, row *goquery.Selection) {
		name := cleanText(row.Find(".name").Text())
		path, _ := row.Find(".moreLink").First().Attr("href")
		if path == "" {
			path, _ = row.Find("a[href*='/team/']").First().Attr("href")
		}
		matched := teamPath.FindStringSubmatch(path)
		positionText := strings.TrimPrefix(cleanText(row.Find(".position").Text()), "#")
		position, rankErr := strconv.Atoi(positionText)
		if name == "" || len(matched) != 2 || rankErr != nil || position < 1 {
			return
		}
		teams = append(teams, model.Team{
			ExternalID: "hltv-team-" + matched[1], Name: name, Ranking: &position,
			LogoURL: row.Find("img").First().AttrOr("src", ""), RankingSource: "hltv",
			SourceURL: absoluteHLTVURL(path), Roster: parseTeamRoster(row), UpdatedAt: now,
		})
	})
	if len(teams) == 0 {
		return nil, fmt.Errorf("hltv team ranking page contained no ranked teams")
	}
	return teams, nil
}

// FetchTeamRoster loads one public team profile to enrich roster names with
// portraits. Calls share the collector's two-second pacing and use no bypass.
func (c *HLTVCollector) FetchTeamRoster(pageURL string) ([]model.TeamPlayer, error) {
	parsed, err := url.Parse(pageURL)
	if err != nil || parsed.Scheme != "https" || (parsed.Hostname() != "hltv.org" && parsed.Hostname() != "www.hltv.org") || !teamPath.MatchString(parsed.Path) {
		return nil, fmt.Errorf("unsupported HLTV team URL")
	}
	doc, err := c.getDocument(pageURL)
	if err != nil {
		return nil, err
	}
	var roster []model.TeamPlayer
	for _, selector := range []string{".teamProfile .players", ".teamProfile .playerLineup", ".team-profile .players", ".teamRoster", ".team-roster", ".lineup"} {
		container := doc.Find(selector).First()
		if container.Length() == 0 {
			continue
		}
		roster = parseTeamRoster(container)
		if len(roster) > 0 {
			break
		}
	}
	if len(roster) == 0 {
		roster = parseTeamRoster(doc.Selection)
	}
	if len(roster) == 0 {
		return nil, fmt.Errorf("HLTV team page exposed no roster profiles")
	}
	if len(roster) > 8 {
		roster = roster[:8]
	}
	return roster, nil
}

// FetchPlayerRanking returns HLTV CS2 stats for the latest 90-day window.
// HLTV changes its rating version from time to time; RatingLabel is retained
// alongside the numeric value so the UI can show the source's current label.
func (c *HLTVCollector) FetchPlayerRanking() ([]model.Player, error) {
	return c.FetchPlayerRankingForPeriod("90d")
}

func (c *HLTVCollector) FetchPlayerRankingForPeriod(period string) ([]model.Player, error) {
	now := time.Now().UTC()
	days := map[string]int{"30d": 30, "90d": 90, "180d": 180, "365d": 365}[period]
	if days == 0 {
		return nil, fmt.Errorf("unsupported player period %q", period)
	}
	start := now.AddDate(0, 0, -days).Format("2006-01-02")
	end := now.Format("2006-01-02")
	pageURL := "https://www.hltv.org/stats/players?csVersion=CS2&startDate=" + start + "&endDate=" + end + "&minMapCount=20"
	doc, err := c.getDocument(pageURL)
	if err != nil {
		return nil, err
	}
	players := make([]model.Player, 0, 50)
	doc.Find(".player-ratings-table tbody tr").Each(func(_ int, row *goquery.Selection) {
		link := row.Find(".playerCol a").First()
		href, _ := link.Attr("href")
		matched := playerPath.FindStringSubmatch(href)
		name := cleanText(link.Text())
		ratingText := cleanText(row.Find("td.ratingCol").Text())
		rating, ratingErr := strconv.ParseFloat(firstNumber.FindString(ratingText), 64)
		if len(matched) != 2 || name == "" || ratingErr != nil {
			return
		}
		mapsText := cleanText(row.Find("td.statsDetail").First().Text())
		maps, _ := strconv.Atoi(firstNumber.FindString(mapsText))
		team := cleanText(row.Find(".teamCol").First().Text())
		players = append(players, model.Player{
			ExternalID: playerExternalID(matched[1], period), Name: name, TeamName: team, MapsPlayed: maps,
			Rating: rating, RatingLabel: cleanText(row.Find(".ratingCol .ratingDesc").Text()), Period: period,
			PeriodLabel: fmt.Sprintf("近 %d 天", days), SourceURL: absoluteHLTVURL(href), UpdatedAt: now,
		})
	})
	if len(players) == 0 {
		return nil, fmt.Errorf("hltv player stats page contained no player ratings")
	}
	return players, nil
}

// FetchArticleBody fetches a publicly accessible HLTV report page using a regular
// HTTP request and extracts article paragraphs. It deliberately does not solve
// challenges, rotate identities, or bypass a denied response.
func (c *HLTVCollector) FetchArticleBody(pageURL string) (string, error) {
	parsed, err := url.Parse(pageURL)
	if err != nil || parsed.Scheme != "https" || (parsed.Hostname() != "hltv.org" && parsed.Hostname() != "www.hltv.org") || !newsPath.MatchString(parsed.Path) {
		return "", fmt.Errorf("unsupported HLTV article URL")
	}
	doc, err := c.getDocument(pageURL)
	if err != nil {
		return "", err
	}
	for _, selector := range []string{".article-content", ".newstext", ".news-content", "article", "main"} {
		container := doc.Find(selector).First()
		if container.Length() == 0 {
			continue
		}
		var blocks []string
		container.Find("h2, h3, p, blockquote, li").Each(func(_ int, node *goquery.Selection) {
			text := cleanText(node.Text())
			if len([]rune(text)) < 28 || looksLikeArticleChrome(text) {
				return
			}
			if len(blocks) == 0 || blocks[len(blocks)-1] != text {
				blocks = append(blocks, text)
			}
		})
		body := strings.Join(blocks, "\n\n")
		if len([]rune(body)) >= 250 {
			return body, nil
		}
	}
	return "", fmt.Errorf("HLTV article page did not expose readable body text")
}

func parseTeamRoster(row *goquery.Selection) []model.TeamPlayer {
	roster := make([]model.TeamPlayer, 0, 5)
	seen := make(map[string]struct{})
	row.Find("a[href*='/player/'], a[href*='/players/'], a[href*='/stats/players/']").Each(func(_ int, link *goquery.Selection) {
		href, _ := link.Attr("href")
		matched := playerPath.FindStringSubmatch(href)
		name := cleanText(link.Text())
		if name == "" {
			name = cleanText(link.AttrOr("title", link.Find("img").First().AttrOr("alt", "")))
		}
		if name == "" {
			return
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		avatar := imageURL(link.Find("img").First())
		if avatar == "" {
			parent := link.Parent()
			for depth := 0; depth < 3 && parent.Length() > 0; depth++ {
				className := strings.ToLower(parent.AttrOr("class", ""))
				if strings.Contains(className, "player") {
					avatar = imageURL(parent.Find("img").First())
					if avatar != "" {
						break
					}
				}
				parent = parent.Parent()
			}
		}
		if avatar != "" {
			avatar = absoluteHLTVURL(avatar)
		}
		profile := ""
		if href != "" && len(matched) == 2 {
			profile = absoluteHLTVURL(href)
		}
		id := ""
		if len(matched) == 2 {
			id = matched[1]
		}
		roster = append(roster, model.TeamPlayer{ID: id, Name: name, AvatarURL: avatar, ProfileURL: profile})
	})
	return roster
}

func imageURL(image *goquery.Selection) string {
	if image.Length() == 0 {
		return ""
	}
	for _, attr := range []string{"data-src", "src", "data-original"} {
		if value := strings.TrimSpace(image.AttrOr(attr, "")); value != "" {
			return absoluteHLTVURL(value)
		}
	}
	if srcset := strings.TrimSpace(image.AttrOr("srcset", "")); srcset != "" {
		value := strings.Fields(strings.Split(srcset, ",")[0])
		if len(value) > 0 {
			return absoluteHLTVURL(value[0])
		}
	}
	return ""
}

func playerExternalID(id, period string) string {
	if period == "90d" || period == "" {
		return "hltv-player-" + id
	}
	return "hltv-player-" + id + "-" + period
}

func looksLikeArticleChrome(text string) bool {
	text = strings.ToLower(text)
	for _, word := range []string{"read more", "share this", "related news", "follow hltv", "cookie settings", "advertisement"} {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func dataID(s *goquery.Selection, key string) string {
	value, _ := s.Attr(key)
	return strings.TrimSpace(value)
}

func (c *HLTVCollector) getDocument(pageURL string) (*goquery.Document, error) {
	c.requestMu.Lock()
	if wait := time.Until(c.lastRequest.Add(2 * time.Second)); wait > 0 {
		time.Sleep(wait)
	}
	c.lastRequest = time.Now()
	c.requestMu.Unlock()
	req, err := http.NewRequest(http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hltv returned %s", resp.Status)
	}
	return goquery.NewDocumentFromReader(resp.Body)
}

func absoluteHLTVURL(path string) string {
	if parsed, err := url.Parse(path); err == nil && parsed.IsAbs() {
		return path
	}
	return "https://www.hltv.org" + path
}

func cleanText(value string) string { return strings.Join(strings.Fields(value), " ") }

func parseNewsTime(s *goquery.Selection, rawText string, now time.Time) time.Time {
	for _, selector := range []string{".newstext-date", ".article-date", ".time"} {
		candidate := s.Find(selector).First()
		for _, attr := range []string{"data-unix", "data-time", "data-timestamp"} {
			if raw, ok := candidate.Attr(attr); ok {
				if seconds, err := strconv.ParseInt(raw, 10, 64); err == nil && seconds > 0 {
					return time.Unix(seconds, 0).UTC()
				}
			}
		}
		text := cleanText(candidate.Text())
		for _, layout := range []string{"2006-01-02 15:04", "02/01/2006 - 15:04", "02-01-2006 15:04"} {
			if value, err := time.ParseInLocation(layout, text, time.UTC); err == nil {
				return value
			}
		}
		if strings.HasPrefix(text, "Today") {
			return now
		}
	}
	if date := newsDateInText.FindString(rawText); date != "" {
		if value, err := time.Parse("2006-01-02", date); err == nil {
			return value.UTC()
		}
	}
	return time.Time{}
}
