package collector

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/example/cs-pulse/backend/internal/model"
)

const valveStandingsRepo = "ValveSoftware/counter-strike_regional_standings"

var valveStandingFile = regexp.MustCompile(`^standings_(global|europe|americas|asia)_(\d{4}_\d{2}_\d{2})\.md$`)

type ValveRankingsCollector struct {
	client    *http.Client
	userAgent string
}

func NewValveRankingsCollector(userAgent string) *ValveRankingsCollector {
	if userAgent == "" {
		userAgent = "LIFE-TV personal CS2 news reader"
	}
	return &ValveRankingsCollector{client: &http.Client{Timeout: 20 * time.Second}, userAgent: userAgent}
}

type githubFile struct {
	Name        string `json:"name"`
	DownloadURL string `json:"download_url"`
}

// Fetch reads Valve's published VRS snapshots from the official public GitHub
// repository. The repo is updated periodically, so this uses its latest dated
// snapshot rather than inventing a live ranking endpoint.
func (c *ValveRankingsCollector) Fetch() ([]model.Team, error) {
	year := time.Now().UTC().Year()
	files, err := c.listSnapshotFiles(year)
	if err != nil {
		return nil, err
	}
	regions := []string{"global", "europe", "americas", "asia"}
	var all []model.Team
	for _, region := range regions {
		var candidates []githubFile
		for _, file := range files {
			match := valveStandingFile.FindStringSubmatch(file.Name)
			if len(match) == 3 && match[1] == region {
				candidates = append(candidates, file)
			}
		}
		if len(candidates) == 0 {
			continue
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].Name > candidates[j].Name })
		content, err := c.getText(candidates[0].DownloadURL)
		if err != nil {
			return nil, fmt.Errorf("valve %s standings: %w", region, err)
		}
		snapshot, err := parseValveStandings(content, region, year, candidates[0].Name)
		if err != nil {
			return nil, err
		}
		all = append(all, snapshot...)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("valve standings repository has no current-year ranking snapshots")
	}
	return all, nil
}

func (c *ValveRankingsCollector) listSnapshotFiles(year int) ([]githubFile, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/invitation/%d", valveStandingsRepo, year)
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", c.userAgent)
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(response.Body)
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("official GitHub standings returned %s", response.Status)
	}
	var files []githubFile
	if err := json.NewDecoder(response.Body).Decode(&files); err != nil {
		return nil, err
	}
	return files, nil
}

func (c *ValveRankingsCollector) getText(url string) (string, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", c.userAgent)
	response, err := c.client.Do(request)
	if err != nil {
		return "", err
	}
	defer closeResponseBody(response.Body)
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("snapshot returned %s", response.Status)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func parseValveStandings(markdown, region string, year int, filename string) ([]model.Team, error) {
	match := valveStandingFile.FindStringSubmatch(filename)
	if len(match) != 3 || match[1] != region {
		return nil, fmt.Errorf("unexpected Valve standings filename %q", filename)
	}
	rows := make([]model.Team, 0, 250)
	for _, line := range strings.Split(markdown, "\n") {
		if !strings.Contains(line, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(line, " |\r\t"), "|")
		if len(cells) < 4 {
			continue
		}
		rank, err := strconv.Atoi(strings.TrimSpace(cells[0]))
		if err != nil || rank < 1 {
			continue
		}
		points, err := strconv.ParseFloat(strings.TrimSpace(cells[1]), 64)
		if err != nil {
			continue
		}
		name := stripMarkdownLink(strings.TrimSpace(cells[2]))
		if name == "" {
			continue
		}
		roster := make([]model.TeamPlayer, 0, 5)
		for _, player := range strings.Split(stripMarkdownLink(strings.TrimSpace(cells[3])), ",") {
			player = strings.TrimSpace(player)
			if player != "" && player != "-" {
				roster = append(roster, model.TeamPlayer{Name: player})
			}
		}
		ranking := rank
		normalized := strings.ToLower(strings.TrimSpace(name))
		normalized = strings.Join(strings.Fields(normalized), "-")
		rows = append(rows, model.Team{
			ExternalID: fmt.Sprintf("valve-%s-%d-%s", region, rank, normalized),
			Name:       name, Ranking: &ranking, RankingSource: "valve", RankingRegion: region,
			Points: points, Roster: roster,
			SourceURL: fmt.Sprintf("https://github.com/%s/blob/main/invitation/%d/%s", valveStandingsRepo, year, filename),
			UpdatedAt: time.Now().UTC(),
		})
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("valve %s snapshot parsed no ranking rows", region)
	}
	return rows, nil
}

func stripMarkdownLink(value string) string {
	value = regexp.MustCompile(`\[([^]]+)]\([^)]*\)`).ReplaceAllString(value, "$1")
	return strings.Trim(value, " `*_\r\t")
}
