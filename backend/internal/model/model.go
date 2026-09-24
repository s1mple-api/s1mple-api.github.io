package model

import "time"

type Article struct {
	ID                      uint       `json:"id" gorm:"primaryKey"`
	Source                  string     `json:"source" gorm:"size:40;index"`
	ExternalID              string     `json:"externalId" gorm:"size:100;uniqueIndex"`
	SourceURL               string     `json:"sourceUrl" gorm:"type:text"`
	ImageURL                string     `json:"imageUrl" gorm:"type:text"`
	Title                   string     `json:"title" gorm:"size:500"`
	Summary                 string     `json:"summary" gorm:"type:text"`
	TitleZH                 string     `json:"titleZh" gorm:"size:500"`
	SummaryZH               string     `json:"summaryZh" gorm:"type:text"`
	Body                    string     `json:"body" gorm:"type:longtext"`
	BodyZH                  string     `json:"bodyZh" gorm:"type:longtext"`
	BodyTranslationVerified bool       `json:"bodyTranslationVerified"`
	BodyFetchedAt           *time.Time `json:"bodyFetchedAt,omitempty"`
	BodyFetchError          string     `json:"bodyFetchError,omitempty" gorm:"size:500"`
	BodyTranslationError    string     `json:"bodyTranslationError,omitempty" gorm:"size:500"`
	TranslatedAt            *time.Time `json:"translatedAt"`
	PublishedAt             time.Time  `json:"publishedAt" gorm:"index"`
	FetchedAt               time.Time  `json:"fetchedAt"`
	CreatedAt               time.Time  `json:"createdAt"`
	UpdatedAt               time.Time  `json:"updatedAt"`
}

type Trend struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Source    string    `json:"source" gorm:"size:20;index"`
	Provider  string    `json:"provider" gorm:"size:60"`
	Rank      int       `json:"rank" gorm:"index"`
	Keyword   string    `json:"keyword" gorm:"size:255"`
	Heat      int64     `json:"heat"`
	Tag       string    `json:"tag" gorm:"size:30"`
	SourceURL string    `json:"sourceUrl" gorm:"type:text"`
	FetchedAt time.Time `json:"fetchedAt"`
}

// ArticleRead records one anonymous browser session per article. Repeated
// detail requests and translation polling do not create additional reads.
type ArticleRead struct {
	ArticleID  uint      `gorm:"primaryKey;autoIncrement:false"`
	ReaderHash string    `gorm:"primaryKey;size:64"`
	ReadAt     time.Time `gorm:"index"`
}

type Team struct {
	ID               uint         `json:"id" gorm:"primaryKey"`
	ExternalID       string       `json:"externalId" gorm:"size:100;uniqueIndex"`
	Name             string       `json:"name" gorm:"size:150;index"`
	LogoURL          string       `json:"logoUrl" gorm:"type:text"`
	Country          string       `json:"country" gorm:"size:100"`
	Ranking          *int         `json:"ranking"`
	RankingSource    string       `json:"rankingSource" gorm:"size:20;index"`
	RankingRegion    string       `json:"rankingRegion" gorm:"size:30;index"`
	Points           float64      `json:"points"`
	Roster           []TeamPlayer `json:"roster" gorm:"serializer:json;type:longtext"`
	RosterCheckedAt  *time.Time   `json:"rosterCheckedAt,omitempty"`
	RosterFetchError string       `json:"rosterFetchError,omitempty" gorm:"size:500"`
	SourceURL        string       `json:"sourceUrl" gorm:"type:text"`
	CreatedAt        time.Time    `json:"createdAt"`
	UpdatedAt        time.Time    `json:"updatedAt"`
}

type TeamPlayer struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AvatarURL  string `json:"avatarUrl"`
	ProfileURL string `json:"profileUrl"`
}

type Player struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	ExternalID  string    `json:"externalId" gorm:"size:100;uniqueIndex"`
	Name        string    `json:"name" gorm:"size:150;index"`
	TeamName    string    `json:"teamName" gorm:"size:150;index"`
	Country     string    `json:"country" gorm:"size:100"`
	MapsPlayed  int       `json:"mapsPlayed"`
	Rating      float64   `json:"rating" gorm:"index"`
	RatingLabel string    `json:"ratingLabel" gorm:"size:40"`
	Period      string    `json:"period" gorm:"size:20;index"`
	PeriodLabel string    `json:"periodLabel" gorm:"size:40"`
	SourceURL   string    `json:"sourceUrl" gorm:"type:text"`
	UpdatedAt   time.Time `json:"updatedAt"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Event struct {
	ID             uint       `json:"id" gorm:"primaryKey"`
	ExternalID     string     `json:"externalId" gorm:"size:100;uniqueIndex"`
	Name           string     `json:"name" gorm:"size:255;index"`
	Series         string     `json:"series" gorm:"size:120;index"`
	EventType      string     `json:"eventType" gorm:"size:60;index"`
	Rating         string     `json:"rating" gorm:"size:30;index"`
	Source         string     `json:"source" gorm:"size:30;index"`
	Location       string     `json:"location" gorm:"size:180"`
	PrizePool      string     `json:"prizePool" gorm:"size:60"`
	AttendingTeams int        `json:"attendingTeams"`
	ValveRanked    bool       `json:"valveRanked"`
	SourceURL      string     `json:"sourceUrl" gorm:"type:text"`
	StartAt        *time.Time `json:"startAt" gorm:"index"`
	EndAt          *time.Time `json:"endAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type Match struct {
	ID          uint       `json:"id" gorm:"primaryKey"`
	ExternalID  string     `json:"externalId" gorm:"size:100;uniqueIndex"`
	EventID     *uint      `json:"eventId"`
	Event       *Event     `json:"event,omitempty"`
	EventName   string     `json:"eventName" gorm:"size:255"`
	Team1ID     *uint      `json:"team1Id"`
	Team1       *Team      `json:"team1,omitempty"`
	Team2ID     *uint      `json:"team2Id"`
	Team2       *Team      `json:"team2,omitempty"`
	Status      string     `json:"status" gorm:"size:30;index"`
	ScheduledAt *time.Time `json:"scheduledAt" gorm:"index"`
	Team1Score  *int       `json:"team1Score"`
	Team2Score  *int       `json:"team2Score"`
	BestOf      int        `json:"bestOf"`
	SourceURL   string     `json:"sourceUrl" gorm:"type:text"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}
