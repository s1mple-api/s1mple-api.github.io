export type Article = {
  id: number
  source: string
  sourceUrl: string
  imageUrl: string
  title: string
  summary: string
  titleZh: string
  summaryZh: string
  body: string
  bodyZh: string
  bodyFetchedAt?: string
  bodyFetchError?: string
  bodyTranslationError?: string
  publishedAt: string
}

export type NewsCategory = 'world' | 'cs2'
export type ContentSection = { id: string; label: string; category: NewsCategory | 'community' }
export type RankedArticle = Article & {
  category: NewsCategory
  section: string
  sectionLabel: string
  heatScore: number
  heat: { freshness: number; topics: number; reading: number; reads: number; matches: string[] }
}

export type TeamPlayer = { id: string; name: string; avatarUrl: string; profileUrl: string }
export type Team = {
  id: number
  name: string
  country: string
  ranking: number | null
  rankingSource: string
  rankingRegion: string
  points: number
  roster: TeamPlayer[]
  sourceUrl: string
  logoUrl: string
  updatedAt?: string
}
export type Player = {
  id: number
  name: string
  teamName: string
  country: string
  mapsPlayed: number
  rating: number
  ratingLabel: string
  period: string
  periodLabel: string
  sourceUrl: string
  updatedAt: string
}
export type Match = {
  id: number
  status: string
  scheduledAt: string | null
  team1?: Team
  team2?: Team
  team1Score: number | null
  team2Score: number | null
  eventName: string
  event?: { name: string }
  sourceUrl: string
}
export type Event = {
  id: number
  externalId: string
  name: string
  series: string
  eventType: string
  rating: string
  source: string
  location: string
  prizePool: string
  attendingTeams: number
  valveRanked: boolean
  startAt: string | null
  endAt: string | null
  sourceUrl: string
}
export type SourceStatus = { state: string; error?: string; count: number; lastAttempt: string; lastSuccess?: string; retryAfter?: string }
export type Trend = { id: number; source: 'baidu' | 'weibo'; provider: string; rank: number; keyword: string; heat: number; tag: string; sourceUrl: string; fetchedAt: string }
export type CommunitySource = 'tieba' | 'hupu' | 'xiaohongshu'
export type CommunityTopic = Omit<Trend, 'source'> & { key: string; source: CommunitySource; summary: string; heatLabel: string; section: string; sectionLabel: string }
export type CommunityTopics = Partial<Record<CommunitySource, CommunityTopic[]>>
export type ContentBlock = { type: 'text' | 'image'; text?: string; url?: string }
export type CommunityState = { state: 'ready' | 'unavailable' | 'stale'; message?: string; fetchedAt: string; cached: boolean }
export type CommunityPost = { key: string; source: CommunitySource; sourceUrl: string; title: string; summary: string; author: string; avatarUrl: string; coverUrl: string; publishedAt?: string; views: number; likes: number; commentCount: number; body: ContentBlock[] }
export type CommunityComment = { id: string; author: string; avatarUrl: string; publishedAt?: string; likes: number; replyCount: number; body: ContentBlock[]; replies: CommunityComment[]; quote?: CommunityComment }
export type CommunityPostList = { topic: { key: string; title: string; summary: string; source: CommunitySource; sourceUrl: string; heatLabel: string; fetchedAt: string }; posts: CommunityPost[]; page: number; hasMore: boolean; status: CommunityState }
export type CommunityPostDetail = { post: CommunityPost; status: CommunityState }
export type CommunityComments = { comments: CommunityComment[]; page: number; total: number; hasMore: boolean; partial: boolean; sort: string; status: CommunityState }
export type Dashboard = { articles: Article[]; worldArticles: Article[]; newsFeed: RankedArticle[]; newsSections: ContentSection[]; baiduTrends: Trend[]; weiboTrends: Trend[]; tiebaTopics: CommunityTopic[]; hupuTopics: CommunityTopic[]; communityTopics: CommunityTopics; communitySections: ContentSection[]; matches: Match[]; events: Event[]; rankings: Team[]; valveRankings: Team[]; players: Player[]; sourceStatus?: Record<string, SourceStatus> }
export type SearchResults = { articles: Article[]; players: Player[] }

const API_BASE_URL = (import.meta.env.VITE_API_BASE_URL || '').replace(/\/$/, '')
export const communityImageURL = (url: string) => `${API_BASE_URL}/api/v1/community/images?url=${encodeURIComponent(url)}`

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${url}`, init)
  if (!response.ok) throw new Error('暂时无法获取数据')
  return await response.json() as T
}

export const getDashboard = () => request<Dashboard>('/api/v1/dashboard')
export const getArticle = (id: number) => request<Article>(`/api/v1/articles/${id}`)
export const getCommunityTopic = (key: string, page = 1, refresh = false) => request<CommunityPostList>(`/api/v1/community/topics/${key}?page=${page}${refresh ? '&refresh=1' : ''}`)
export const getCommunityPost = (key: string) => request<CommunityPostDetail>(`/api/v1/community/posts/${key}`)
export const getCommunityComments = (key: string, page = 1, sort = 'hot') => request<CommunityComments>(`/api/v1/community/posts/${key}/comments?page=${page}&sort=${sort}`)

let anonymousReader = ''
export function recordArticleRead(id: number) {
  if (!anonymousReader) {
    try {
      anonymousReader = sessionStorage.getItem('life-reader') || crypto.randomUUID()
      sessionStorage.setItem('life-reader', anonymousReader)
    } catch { anonymousReader = crypto.randomUUID() }
  }
  return request<{ counted: boolean }>(`/api/v1/articles/${id}/read`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ readerId: anonymousReader }),
  })
}
export const getPlayer = (id: number) => request<Player>(`/api/v1/players/${id}`)
export const getPlayers = (period: string) => request<Player[]>(`/api/v1/players?period=${encodeURIComponent(period)}`)
export const getRankings = (source: string, region = '') => request<Team[]>(`/api/v1/rankings?source=${encodeURIComponent(source)}&region=${encodeURIComponent(region)}`)
export const getEvents = (filters: { year: string; series: string; rating: string }) => {
  const params = new URLSearchParams()
  if (filters.year) params.set('year', filters.year)
  if (filters.series) params.set('series', filters.series)
  if (filters.rating) params.set('rating', filters.rating)
  return request<Event[]>(`/api/v1/events${params.size ? `?${params.toString()}` : ''}`)
}
export const search = (query: string) => request<SearchResults>(`/api/v1/search?q=${encodeURIComponent(query)}`)
