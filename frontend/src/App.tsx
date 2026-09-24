import { useEffect, useState } from 'react'
import { getArticle, getDashboard, getEvents, getPlayer, getPlayers, getRankings, recordArticleRead, search, type Article, type Dashboard, type Event, type Player, type SearchResults, type SourceStatus, type Team, type Trend } from './lib/api'
import { NewsHome, NewsCollection } from './components/NewsHome'

type Tab = 'news' | 'world' | 'cs2' | 'trending' | 'events' | 'teams' | 'players'
const dateTime = new Intl.DateTimeFormat('zh-CN', { month: 'long', day: 'numeric', hour: '2-digit', minute: '2-digit' })
const fullDate = new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: 'long', day: 'numeric', weekday: 'long' })
const eventDate = new Intl.DateTimeFormat('zh-CN', { timeZone: 'UTC', year: 'numeric', month: 'short', day: 'numeric' })
const noEvents: Event[] = []
const tabs: { key: Tab; label: string }[] = [
  { key: 'news', label: '首页' },
  { key: 'world', label: '世界新闻' },
  { key: 'cs2', label: 'CS2 新闻' },
  { key: 'trending', label: '热搜榜' },
  { key: 'events', label: '赛事数据' },
  { key: 'teams', label: '战队排名' },
  { key: 'players', label: '选手数据' },
]

export default function App() {
  const [data, setData] = useState<Dashboard | null>(null)
  const [error, setError] = useState('')
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null)
  const [tab, setTab] = useState<Tab>('news')
  const [lastTab, setLastTab] = useState<Tab>('news')
  const [route, setRoute] = useState(window.location.pathname)
  const [detailArticle, setDetailArticle] = useState<Article | null>(null)
  const [detailPlayer, setDetailPlayer] = useState<Player | null>(null)
  const [loadedDetailRoute, setLoadedDetailRoute] = useState('')
  const [searchText, setSearchText] = useState('')
  const [searchResponse, setSearchResponse] = useState<{ query: string; results: SearchResults } | null>(null)
  const [searchFocused, setSearchFocused] = useState(false)

  useEffect(() => {
    const refresh = () => getDashboard()
      .then((next) => { setData(next); setUpdatedAt(new Date()); setError('') })
      .catch((reason: Error) => setError(reason.message))
    void refresh()
    const interval = window.setInterval(() => { void refresh() }, 60_000)
    return () => window.clearInterval(interval)
  }, [])

  useEffect(() => {
    const pop = () => {
      setDetailArticle(null)
      setDetailPlayer(null)
      setLoadedDetailRoute('')
      setRoute(window.location.pathname)
    }
    window.addEventListener('popstate', pop)
    return () => window.removeEventListener('popstate', pop)
  }, [])

  useEffect(() => {
    const shortcut = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        document.querySelector<HTMLInputElement>('.search-box input')?.focus()
      }
    }
    window.addEventListener('keydown', shortcut)
    return () => window.removeEventListener('keydown', shortcut)
  }, [])

  useEffect(() => {
    const articleID = route.match(/^\/article\/(\d+)$/)?.[1]
    const playerID = route.match(/^\/player\/(\d+)$/)?.[1]
    if (!articleID && !playerID) return
    let active = true
    const load = articleID ? getArticle(Number(articleID)) : getPlayer(Number(playerID))
    load.then((item) => {
      if (!active) return
      if (articleID) {
        setDetailArticle(item as Article)
        void recordArticleRead((item as Article).id).catch(() => undefined)
      }
      else setDetailPlayer(item as Player)
      setLoadedDetailRoute(route)
    }).catch(() => {
      if (active) {
        setError(articleID ? '这篇文章暂时无法打开。' : '这位选手暂时无法打开。')
        setLoadedDetailRoute(route)
      }
    })
    return () => { active = false }
  }, [route])

  useEffect(() => {
    if (loadedDetailRoute !== route || !detailArticle?.body || detailArticle.bodyZh || detailArticle.bodyTranslationError) return
    const articleID = detailArticle.id
    const timer = window.setInterval(() => {
      getArticle(articleID).then((article) => setDetailArticle(article)).catch(() => undefined)
    }, 5000)
    return () => window.clearInterval(timer)
  }, [route, loadedDetailRoute, detailArticle?.id, detailArticle?.body, detailArticle?.bodyZh, detailArticle?.bodyTranslationError])

  useEffect(() => {
    const query = searchText.trim()
    if (query.length < 2) return
    let active = true
    const timer = window.setTimeout(() => {
      search(query).then((results) => { if (active) setSearchResponse({ query, results }) })
        .catch(() => { if (active) setSearchResponse({ query, results: { articles: [], players: [] } }) })
    }, 250)
    return () => { active = false; window.clearTimeout(timer) }
  }, [searchText])

  const selectTab = (next: Tab) => {
    setTab(next)
    setLastTab(next)
    if (route !== '/') navigate('/')
  }
  const openArticle = (article: Article) => {
    setSearchFocused(false)
    setLastTab(tab)
    navigate(`/article/${article.id}`)
  }
  const openPlayer = (player: Player) => {
    setSearchFocused(false)
    setLastTab(tab)
    navigate(`/player/${player.id}`)
  }
  const backToContent = () => {
    setTab(lastTab)
    navigate('/')
  }
  const navigate = (path: string) => {
    window.history.pushState({}, '', path)
    setDetailArticle(null)
    setDetailPlayer(null)
    setLoadedDetailRoute('')
    setRoute(path)
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  const newsFeed = data?.newsFeed ?? []
  const baiduTrends = data?.baiduTrends ?? []
  const weiboTrends = data?.weiboTrends ?? []
  const events = data?.events ?? noEvents
  const rankings = data?.rankings ?? []
  const valveRankings = data?.valveRankings ?? []
  const players = data?.players ?? []
  const isArticle = route.startsWith('/article/')
  const isPlayer = route.startsWith('/player/')
  const detailLoading = (isArticle || isPlayer) && loadedDetailRoute !== route
  const activeArticle = isArticle && !detailLoading ? detailArticle : null
  const activePlayer = isPlayer && !detailLoading ? detailPlayer : null
  const showSearch = searchFocused && searchText.trim().length >= 2
  const currentSearch = searchResponse?.query === searchText.trim() ? searchResponse.results : null

  return <>
    <div className="utility-bar"><div className="page-width"><span>{fullDate.format(new Date())}</span><span>全球新闻 · 热点趋势 · Counter-Strike 2</span></div></div>
    <header className="site-header page-width">
      <button className="wordmark" onClick={() => { setTab('news'); setLastTab('news'); navigate('/') }} aria-label="LIFE TV 首页"><span>LIFE</span><span>TV</span></button>
      <div className="header-description">全球视野 · 即时资讯</div>
      <div className="header-search search-holder">
        <label className="search-box"><span aria-hidden="true">⌕</span><input value={searchText} onChange={(event) => setSearchText(event.target.value)} onFocus={() => setSearchFocused(true)} onBlur={() => window.setTimeout(() => setSearchFocused(false), 140)} onKeyDown={(event) => { if (event.key === 'Escape') setSearchFocused(false) }} placeholder="搜索全球新闻、CS2、选手" aria-label="搜索全球新闻、CS2、选手" /><kbd>⌘ K</kbd></label>
        {showSearch && <SearchPanel results={currentSearch} loading={!currentSearch} query={searchText} onArticle={openArticle} onPlayer={openPlayer} />}
      </div>
      <div className="header-update"><span className="status-dot" />{updatedAt ? `刷新于 ${new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit' }).format(updatedAt)}` : '正在连接'}</div>
    </header>

    <nav className="main-nav" aria-label="内容分类"><div className="page-width" role="tablist" aria-label="LIFE TV 栏目">
      {tabs.map((item) => <button key={item.key} role="tab" aria-selected={!isArticle && !isPlayer && tab === item.key} className={(!isArticle && !isPlayer && tab === item.key) ? 'active' : ''} onClick={() => selectTab(item.key)}>{item.label}</button>)}
    </div></nav>

    <main className="page-width content">
      {error && <div className="error-message" role="alert">{error}</div>}
      {isArticle ? <ArticlePage article={activeArticle} loading={detailLoading} backLabel={tabs.find((item) => item.key === lastTab)?.label || '内容'} onBack={backToContent} /> : isPlayer ? <PlayerPage player={activePlayer} loading={detailLoading} backLabel={tabs.find((item) => item.key === lastTab)?.label || '内容'} onBack={backToContent} /> : <>
        <div className="page-intro"><div><span className="red-line" /><h1>{tab === 'news' ? '今日要闻' : tabs.find((item) => item.key === tab)?.label}</h1></div><p>世界动态、热搜话题与 CS2 赛场，一站阅读。</p></div>
        {tab === 'news' && <NewsHome stories={newsFeed} loading={!data} onArticle={openArticle} sourceStatus={data?.sourceStatus} hotPanel={<HotPanel baiduTrends={baiduTrends} weiboTrends={weiboTrends} sourceStatus={data?.sourceStatus} loading={!data} compact />} />}
        {(tab === 'world' || tab === 'cs2') && <NewsCollection stories={newsFeed} category={tab} loading={!data} onArticle={openArticle} />}
        {tab === 'trending' && <TrendingPage baiduTrends={baiduTrends} weiboTrends={weiboTrends} sourceStatus={data?.sourceStatus} loading={!data} />}
        {tab === 'events' && <EventsPage events={events} status={data?.sourceStatus?.events} loading={!data} />}
        {tab === 'teams' && <TeamsPage teams={rankings} valveTeams={valveRankings} hltvStatus={data?.sourceStatus?.rankings} rosterStatus={data?.sourceStatus?.teamRoster} valveStatus={data?.sourceStatus?.valveRankings} loading={!data} />}
        {tab === 'players' && <PlayersPage players={players} status={data?.sourceStatus?.players} loading={!data} onPlayer={openPlayer} />}
      </>}
    </main>
    <footer className="footer"><div className="page-width"><strong>LIFE TV</strong><span>世界新闻 · 热搜 · CS2 中文资讯</span><button onClick={() => window.scrollTo({ top: 0, behavior: 'smooth' })}>返回顶部 ↑</button></div></footer>
  </>
}


function HotPanel({ baiduTrends, weiboTrends, sourceStatus, loading, compact = false }: { baiduTrends: Trend[]; weiboTrends: Trend[]; sourceStatus?: Record<string, SourceStatus>; loading: boolean; compact?: boolean }) {
  const [source, setSource] = useState<'baidu' | 'weibo'>('baidu')
  const rows = source === 'baidu' ? baiduTrends : weiboTrends
  const status = sourceStatus?.[source === 'baidu' ? 'baiduTrends' : 'weiboTrends']
  const visible = compact ? rows.slice(0, 8) : rows
  return <section className={`hot-panel ${compact ? 'hot-panel-compact' : ''}`} aria-label="实时热搜">
    <div className="section-label"><h2>实时热搜</h2><span>HOT SEARCH</span></div>
    <div className="hot-source-tabs" role="tablist" aria-label="热搜来源"><button role="tab" aria-selected={source === 'baidu'} className={source === 'baidu' ? 'active' : ''} onClick={() => setSource('baidu')}>百度热搜</button><button role="tab" aria-selected={source === 'weibo'} className={source === 'weibo' ? 'active' : ''} onClick={() => setSource('weibo')}>微博热搜</button></div>
    {loading ? <div className="hot-empty">正在读取实时榜单…</div> : visible.length ? <ol className="hot-list">{visible.map((item) => <li key={`${item.source}-${item.rank}-${item.keyword}`}><a href={item.sourceUrl} target="_blank" rel="noreferrer"><span className={`hot-rank ${item.rank <= 3 ? 'top' : ''}`}>{item.rank.toString().padStart(2, '0')}</span><span className="hot-keyword">{item.keyword}{item.tag && <em>{item.tag}</em>}</span><span className="hot-value">{item.heat ? new Intl.NumberFormat('zh-CN', { notation: 'compact', maximumFractionDigits: 1 }).format(item.heat) : '—'}</span></a></li>)}</ol> : <div className="hot-empty">{status?.state === 'unavailable' ? '该来源暂时无法更新，稍后自动重试。' : '热搜榜单正在同步。'}</div>}
    <div className="hot-panel-foot"><span>{source === 'weibo' ? '微博榜单 · UAPI 聚合' : '百度官方热搜榜'}{visible[0]?.fetchedAt ? ` · ${dateTime.format(new Date(visible[0].fetchedAt))}` : ''}</span><a href={source === 'weibo' ? 'https://uapis.cn/docs/api-reference/get-misc-hotboard' : 'https://top.baidu.com/board?tab=realtime'} target="_blank" rel="noreferrer">数据来源 ↗</a></div>
  </section>
}

function TrendingPage({ baiduTrends, weiboTrends, sourceStatus, loading }: { baiduTrends: Trend[]; weiboTrends: Trend[]; sourceStatus?: Record<string, SourceStatus>; loading: boolean }) {
  return <div className="trending-page"><HotPanel baiduTrends={baiduTrends} weiboTrends={weiboTrends} sourceStatus={sourceStatus} loading={loading} /><aside className="trending-aside"><span className="article-kicker">LIFE TV / TRENDING</span><h2>此刻大家在关注什么</h2><p>百度榜单直接读取官方公开页面的热搜指数；微博榜单由 UAPI 聚合微博热搜数据。两种热度口径不同，排名只在各自榜单内比较。</p></aside></div>
}

function EventsPage({ events, status, loading }: { events: Event[]; status?: SourceStatus; loading: boolean }) {
  const [year, setYear] = useState('')
  const [series, setSeries] = useState('')
  const [rating, setRating] = useState('')
  const [response, setResponse] = useState<{ key: string; rows: Event[] } | null>(null)
  const hasFilters = Boolean(year || series || rating)
  const filterKey = JSON.stringify([year, series, rating])
  const years = [...new Set(events.flatMap((event) => event.startAt ? [String(new Date(event.startAt).getUTCFullYear())] : []))].sort((a, b) => Number(b) - Number(a))
  const seriesList = [...new Set(events.map((event) => event.series).filter(Boolean))].sort((a, b) => a.localeCompare(b))
  const ratings = [...new Set(events.map((event) => event.rating || event.eventType).filter(Boolean))].sort((a, b) => a.localeCompare(b))

  useEffect(() => {
    if (!year && !series && !rating) return
    let active = true
    getEvents({ year, series, rating }).then((rows) => { if (active) setResponse({ key: filterKey, rows }) })
      .catch(() => { if (active) setResponse({ key: filterKey, rows: [] }) })
    return () => { active = false }
  }, [year, series, rating, filterKey])

  const rows = hasFilters ? response?.key === filterKey ? response.rows : [] : events
  const busy = loading || (hasFilters && response?.key !== filterKey)
  return <section className="data-page events-page">
    <div className="section-label"><h2>后续赛事</h2><span>HLTV 赛事日历 · 按当前时间筛选</span></div>
    <div className="event-filters" aria-label="赛事筛选条件">
      <label>年份<select value={year} onChange={(event) => setYear(event.target.value)}><option value="">全部年份</option>{years.map((item) => <option value={item} key={item}>{item} 年</option>)}</select></label>
      <label>赛事系列<select value={series} onChange={(event) => setSeries(event.target.value)}><option value="">全部系列</option>{seriesList.map((item) => <option value={item} key={item}>{item}</option>)}</select></label>
      <label>赛事类型<select value={rating} onChange={(event) => setRating(event.target.value)}><option value="">全部类型</option>{ratings.map((item) => <option value={item} key={item}>{item}</option>)}</select></label>
      {(year || series || rating) && <button className="clear-filters" onClick={() => { setYear(''); setSeries(''); setRating('') }}>清除筛选</button>}
    </div>
    <p className="event-rating-note">赛事类型与日期来自 HLTV 赛事日历；HLTV 未提供结构化评级时不推测 S/A/B/C 等级。数据每 30 分钟同步一次。</p>
    {busy ? <div className="loading-panel">正在读取后续赛事…</div> : rows.length ? <div className="event-grid">{rows.map((event) => <EventCard event={event} key={event.id} />)}</div> : <EmptyState title="暂时没有可展示的后续赛事" text={sourceMessage(status, '赛事列表仅显示当前时间之后仍在规划或进行中的赛事；HLTV 赛事日历同步后会自动更新。')} />}
    {!busy && rows.length > 0 && status?.state === 'unavailable' && <StatusWarning message={sourceMessage(status, '赛事数据暂时无法更新，当前展示数据库中已缓存的计划。')} />}
  </section>
}

function EventCard({ event }: { event: Event }) {
  const startKey = event.startAt ? new Date(event.startAt).toISOString().slice(0, 10) : ''
  const endKey = event.endAt ? new Date(event.endAt).toISOString().slice(0, 10) : ''
  const dateText = event.startAt
    ? `${eventDate.format(new Date(event.startAt))}${event.endAt && endKey !== startKey ? ` — ${eventDate.format(new Date(event.endAt))}` : ''}`
    : '日期待定'
  return <article className="event-card">
    <div className="event-card-top"><span className="event-date-label">{dateText}</span>{(event.rating || event.eventType) && <span className="event-type-pill">{event.rating || event.eventType}</span>}</div>
    <h3>{event.name}</h3>
    <div className="event-series">{event.series || '其他赛事'}</div>
    <dl className="event-facts">
      <div><dt>地点</dt><dd>{event.location || '待公布'}</dd></div>
      {event.prizePool && <div><dt>奖金</dt><dd>{event.prizePool}</dd></div>}
      {event.attendingTeams > 0 && <div><dt>参赛队伍</dt><dd>{event.attendingTeams} 支</dd></div>}
    </dl>
    <div className="event-card-bottom"><span className={event.valveRanked ? 'valve-ranked' : 'event-source-label'}>{event.valveRanked ? 'Valve Ranked' : 'HLTV 赛事数据'}</span><a href={event.sourceUrl} target="_blank" rel="noreferrer">赛事详情 ↗</a></div>
  </article>
}

function TeamsPage({ teams, valveTeams, hltvStatus, rosterStatus, valveStatus, loading }: { teams: Team[]; valveTeams: Team[]; hltvStatus?: SourceStatus; rosterStatus?: SourceStatus; valveStatus?: SourceStatus; loading: boolean }) {
  const [source, setSource] = useState<'hltv' | 'valve'>('hltv')
  const [region, setRegion] = useState('global')
  const [response, setResponse] = useState<{ region: string; rows: Team[] } | null>(null)
  const needsRequest = source === 'valve' && (region !== 'global' || valveTeams.length === 0)

  useEffect(() => {
    if (source === 'hltv' || (region === 'global' && valveTeams.length)) return
    let active = true
    getRankings('valve', region).then((rows) => { if (active) setResponse({ region, rows }) })
      .catch(() => { if (active) setResponse({ region, rows: [] }) })
    return () => { active = false }
  }, [source, region, valveTeams.length])

  const rows = source === 'hltv' ? teams : !needsRequest ? valveTeams : response?.region === region ? response.rows : []
  const requesting = needsRequest && response?.region !== region
  const status = source === 'hltv' ? hltvStatus : valveStatus
  const sourceLabel = source === 'hltv' ? 'HLTV World Ranking' : 'Valve Regional Standings (VRS)'
  const valveUnavailable = valveStatus?.state === 'unavailable'
    ? `Valve 官方仓库当前返回 ${valveStatus.error || '数据暂不可用'}；采集器会冷却后重试。`
    : 'Valve 官方 VRS 快照同步后会显示在这里。'
  return <section className="data-page team-rankings-page"><div className="section-label"><h2>战队排名</h2><span>{sourceLabel}</span></div>
    <div className="ranking-controls">
      <div className="ranking-source-tabs" role="tablist" aria-label="排名来源">
        <button role="tab" aria-selected={source === 'hltv'} className={source === 'hltv' ? 'active' : ''} onClick={() => setSource('hltv')}>HLTV 排名</button>
        <button role="tab" aria-selected={source === 'valve'} className={source === 'valve' ? 'active' : ''} onClick={() => setSource('valve')}>Valve VRS</button>
      </div>
      {source === 'valve' && <label className="region-select">地区
        <select value={region} onChange={(event) => setRegion(event.target.value)}>
          <option value="global">全球</option><option value="europe">欧洲</option><option value="americas">美洲</option><option value="asia">亚洲</option>
        </select>
      </label>}
    </div>
    {loading || requesting ? <div className="loading-panel">正在读取{source === 'hltv' ? ' HLTV' : ' Valve'}战队榜…</div> : rows.length ? <>
      {status?.state === 'unavailable' && <StatusWarning message={source === 'hltv' ? sourceMessage(status, '') : `Valve 官方排名暂时无法更新，展示已缓存快照。${status.retryAfter ? ` 下次检查时间：${dateTime.format(new Date(status.retryAfter))}。` : ''}`} />}
      {source === 'hltv' && rosterStatus?.state === 'unavailable' && <StatusWarning message={`战队名单或头像暂时无法从 HLTV 资料页读取；已缓存的名单仍会保留，采集器冷却后再试。`} />}
      <div className="table-wrap"><table className="data-table team-table"><thead><tr><th>排名</th><th>战队 / 当前阵容</th>{source === 'valve' ? <th>VRS 分数</th> : <th>国家 / 地区</th>}<th>数据来源</th></tr></thead><tbody>{rows.map((team) => <tr key={team.id}>
        <td className="rank-number">{team.ranking ?? '—'}</td>
        <td><div className="team-row-info">{team.logoUrl && <img className="team-logo" src={team.logoUrl} alt="" loading="lazy" />}<div><strong className="team-row-name">{team.name}</strong><TeamRosterStrip players={team.roster || []} /></div></div></td>
        <td>{source === 'valve' ? Math.round(team.points).toLocaleString() : team.country || '—'}</td>
        <td><a href={team.sourceUrl || 'https://www.hltv.org/ranking/teams'} target="_blank" rel="noreferrer">{source === 'valve' ? 'Valve 官方 VRS ↗' : 'HLTV ↗'}</a></td>
      </tr>)}</tbody></table></div>
      <p className="ranking-footnote">Valve VRS 为官方周期快照，HLTV 排名按 HLTV 榜单展示；两者算法和更新时间不同，不作混合排序。</p>
    </> : <EmptyState title={source === 'hltv' ? 'HLTV 战队排名暂不可用' : 'Valve 排名快照暂不可用'} text={source === 'hltv' ? sourceMessage(status, 'HLTV 榜单采集已启用，首次同步可能需要一点时间。') : valveUnavailable} />}
  </section>
}

function TeamRosterStrip({ players }: { players: Team['roster'] }) {
  if (!players.length) return <span className="roster-unavailable">阵容暂未读取</span>
  return <div className="team-roster" aria-label={`阵容：${players.map((player) => player.name).join('、')}`}>
    {players.slice(0, 5).map((player, index) => <span className="roster-player" key={`${player.id || player.name}-${index}`} title={player.name}>
      {player.avatarUrl ? <img src={player.avatarUrl} alt="" loading="lazy" /> : <span className="roster-avatar-fallback">{player.name.slice(0, 1).toUpperCase()}</span>}
      <small>{player.name}</small>
    </span>)}
  </div>
}

function PlayersPage({ players, status, loading, onPlayer }: { players: Player[]; status?: SourceStatus; loading: boolean; onPlayer: (player: Player) => void }) {
  const [period, setPeriod] = useState('90d')
  const [response, setResponse] = useState<{ period: string; rows: Player[] } | null>(null)
  useEffect(() => {
    if (period === '90d') return
    let active = true
    getPlayers(period).then((rows) => { if (active) setResponse({ period, rows }) })
      .catch(() => { if (active) setResponse({ period, rows: [] }) })
    return () => { active = false }
  }, [period])
  const rows = period === '90d' ? players : response?.period === period ? response.rows : []
  const periodLoading = period !== '90d' && response?.period !== period
  const leaders = rows.slice(0, 3)
  const periodLabel = ({ '30d': '近 30 天', '90d': '近 90 天', '180d': '近 180 天', '365d': '近 1 年' } as Record<string, string>)[period]
  return <section className="data-page"><div className="section-label"><h2>选手 Rating 排行</h2><span>{periodLabel} · 至少 20 张地图 · HLTV</span></div>
    <div className="player-period-control"><label htmlFor="player-period">统计周期</label><select id="player-period" value={period} onChange={(event) => setPeriod(event.target.value)}><option value="30d">近 30 天</option><option value="90d">近 90 天</option><option value="180d">近 180 天</option><option value="365d">近 1 年</option></select><span>按该周期的 Rating 由高到低排列</span></div>
    {loading || periodLoading ? <div className="loading-panel">正在读取{periodLabel}选手统计…</div> : rows.length ? <>
      <div className="player-leaders">{leaders.map((player, index) => <button className={`player-leader player-leader-${index + 1}`} key={player.id} onClick={() => onPlayer(player)}><span className="leader-place">0{index + 1}</span><span className="leader-name">{player.name}</span><span className="leader-team">{player.teamName || '自由选手'}</span><strong>{player.rating.toFixed(2)}</strong><small>{player.ratingLabel || 'Rating'} · {player.mapsPlayed} 张地图</small></button>)}</div>
      {status?.state === 'unavailable' && <StatusWarning message={sourceMessage(status, '')} />}<div className="table-wrap"><table className="data-table"><thead><tr><th>排名</th><th>选手</th><th>战队</th><th>Rating</th><th>地图数</th></tr></thead><tbody>{rows.map((player, index) => <tr key={player.id} className="clickable-row" onClick={() => onPlayer(player)}><td className="rank-number">{index + 1}</td><td className="player-name">{player.name}</td><td>{player.teamName || '—'}</td><td className="rating-value">{player.rating.toFixed(2)}</td><td>{player.mapsPlayed}</td></tr>)}</tbody></table></div>
    </> : <EmptyState title="暂时无法读取该周期的选手 Rating" text={sourceMessage(status, '切换周期后会从 HLTV 公开统计页面读取并缓存数据；该时间范围尚无缓存记录。')} />}
  </section>
}

function ArticlePage({ article, loading, backLabel, onBack }: { article: Article | null; loading: boolean; backLabel: string; onBack: () => void }) {
  if (loading) return <div className="loading-panel detail-loading">正在打开文章…</div>
  if (!article) return <EmptyState title="文章暂时无法显示" text="文章可能已被移除，请返回资讯列表。" action="返回资讯" onAction={onBack} />
  const title = article.titleZh || article.title
  const summary = article.summaryZh || article.summary
  const fullText = article.bodyZh || article.body
  const paragraphs = fullText.split(/\n{2,}/).map((paragraph) => paragraph.trim()).filter(Boolean)
  return <article className="article-page">
    <button className="back-button" onClick={onBack}>← 返回{backLabel}</button>
    <div className="article-page-grid"><div className="article-body">
      <div className="article-kicker">{sourceName(article.source)} · LIFE TV 全文阅读</div>
      <h1>{title}</h1>
      <div className="article-byline"><span>来源：{sourceName(article.source)}</span><time dateTime={article.publishedAt}>{dateTime.format(new Date(article.publishedAt))}</time></div>
      <div className="article-rule" />
      {article.imageUrl && <figure className="article-hero"><img src={article.imageUrl} alt="" /><figcaption>图片来源：{sourceName(article.source)}</figcaption></figure>}
      {fullText ? <>
        {!article.bodyZh && <div className="translation-notice">{article.bodyTranslationError ? '完整原文已读取，但自动翻译暂时不可用；当前先显示来源原文。' : '中文全文翻译生成中，先显示来源原文；完成后此页会自动更新。'}</div>}
        {article.bodyZh && <p className="article-translation-note">以下为自动翻译全文</p>}
        <div className="article-fulltext">{paragraphs.map((paragraph, index) => <p key={index}>{paragraph}</p>)}</div>
      </> : <>
        {summary && <><div className="article-translation-note">目前可读取的内容</div><div className="article-fulltext"><p>{summary}</p></div></>}
        {article.bodyFetchError ? <div className="article-body-warning">信息源暂未允许本站读取正文，因此这里无法提供完整报道。不会尝试绕过验证或访问限制；你仍可通过来源链接查看原文。</div> : <p className="article-summary muted">正在尝试读取完整报道…如果来源不提供正文，页面会保留来源链接。</p>}
      </>}
      <div className="source-callout"><div><strong>查看原始报道</strong><p>完整上下文与最终版本请以信息源页面为准。</p></div><a className="source-button" href={article.sourceUrl} target="_blank" rel="noreferrer">前往{sourceName(article.source)} <span aria-hidden="true">↗</span></a></div>
    </div><aside className="article-sidebar"><span className="source-note-kicker">LIFE TV 阅读说明</span><p>正文仅在打开详情时按普通公开页面请求读取并缓存。若来源限制访问，页面会说明情况并保留原文入口。</p><div className="article-side-meta"><span>正文状态</span><strong>{fullText ? (article.bodyZh ? '全文已翻译' : '原文已读取') : article.bodyFetchError ? '来源暂不可读' : '读取中'}</strong></div><div className="article-side-meta"><span>信息来源</span><strong>{sourceName(article.source)}</strong></div></aside></div>
  </article>
}

function PlayerPage({ player, loading, backLabel, onBack }: { player: Player | null; loading: boolean; backLabel: string; onBack: () => void }) {
  if (loading) return <div className="loading-panel detail-loading">正在读取选手数据…</div>
  if (!player) return <EmptyState title="选手暂时无法显示" text="选手数据可能尚未完成同步。" action={`返回${backLabel}`} onAction={onBack} />
  return <section className="player-page"><button className="back-button" onClick={onBack}>← 返回{backLabel}</button><div className="player-profile"><span className="profile-monogram">{player.name.slice(0, 1).toUpperCase()}</span><div><span className="article-kicker">HLTV 选手统计</span><h1>{player.name}</h1><p>{player.teamName || '自由选手'}{player.country ? ` · ${player.country}` : ''}</p></div><div className="profile-rating"><strong>{player.rating.toFixed(2)}</strong><span>{player.ratingLabel || 'Rating'}</span></div></div><div className="player-facts"><div><strong>{player.mapsPlayed}</strong><span>{player.periodLabel || '当前周期'}地图</span></div><div><strong>{player.teamName || '—'}</strong><span>当前战队</span></div><div><strong>{new Intl.DateTimeFormat('zh-CN', { month: 'long', day: 'numeric' }).format(new Date(player.updatedAt))}</strong><span>数据更新时间</span></div></div><a className="source-button" href={player.sourceUrl} target="_blank" rel="noreferrer">查看 HLTV 选手统计 ↗</a></section>
}

function SearchPanel({ results, loading, query, onArticle, onPlayer }: { results: SearchResults | null; loading: boolean; query: string; onArticle: (article: Article) => void; onPlayer: (player: Player) => void }) {
  return <div className="search-panel">
    <div className="search-panel-title">搜索“{query.trim()}”</div>
    {loading && <div className="search-empty">正在搜索…</div>}
    {!loading && results && results.articles.length > 0 && <><div className="search-category">新闻资讯</div>{results.articles.map((article) => <button className="search-result" key={`a-${article.id}`} onMouseDown={(event) => event.preventDefault()} onClick={() => onArticle(article)}><span className="search-result-type">报</span><span><strong>{article.titleZh || article.title}</strong><small>{sourceName(article.source)} · {dateTime.format(new Date(article.publishedAt))}</small></span><b>→</b></button>)}</>}
    {!loading && results && results.players.length > 0 && <><div className="search-category">选手</div>{results.players.map((player) => <button className="search-result" key={`p-${player.id}`} onMouseDown={(event) => event.preventDefault()} onClick={() => onPlayer(player)}><span className="search-result-type player-type">选</span><span><strong>{player.name}</strong><small>{player.teamName || '自由选手'} · Rating {player.rating.toFixed(2)}</small></span><b>→</b></button>)}</>}
    {!loading && results && !results.articles.length && !results.players.length && <div className="search-empty">没有找到相关资讯或选手。</div>}
    {!results && !loading && <div className="search-empty">输入至少两个字符开始搜索。</div>}
  </div>
}


function EmptyState({ title, text, action, onAction }: { title: string; text: string; action?: string; onAction?: () => void }) {
  return <div className="empty-state"><span className="empty-mark" aria-hidden="true">L</span><h2>{title}</h2><p>{text}</p>{action && onAction && <button className="back-button" onClick={onAction}>{action} →</button>}</div>
}

function StatusWarning({ message }: { message: string }) { return <div className="error-message" role="status">{message}</div> }

function sourceName(source: string) { return source === 'hltv' ? 'HLTV' : source === 'steam' ? 'Steam 官方' : source === 'bbc' ? 'BBC News' : source.toUpperCase() }
function sourceMessage(status: SourceStatus | undefined, fallback: string, source = 'HLTV') {
  if (!status) return fallback
  if (status.state === 'unavailable') {
    const retry = status.retryAfter ? `预计 ${new Intl.DateTimeFormat('zh-CN', { month: 'long', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(status.retryAfter))} 后重试。` : '采集器会按计划重试。'
    const reason = status.error?.replace(/^hltv(?: api)? returned /i, '') || '数据暂不可用'
    return `${source} 当前返回 ${reason}。采集器不会尝试绕过访问限制，${retry}恢复后数据将自动出现。`
  }
  if (status.state === 'ready' && status.lastSuccess) return `最近一次同步于 ${new Intl.DateTimeFormat('zh-CN', { month: 'long', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(status.lastSuccess))}，当前没有可展示的数据。`
  return fallback
}
