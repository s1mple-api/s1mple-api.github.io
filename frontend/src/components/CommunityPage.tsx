import { useEffect, useState } from 'react'
import type { CommunitySource, CommunityTopic, CommunityTopics, ContentSection, SourceStatus } from '../lib/api'
import { communityPlatforms } from '../lib/community'
const timestamp = new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' })
const heat = new Intl.NumberFormat('zh-CN', { notation: 'compact', maximumFractionDigits: 1 })
const formatTime = (value?: string) => value && Number.isFinite(Date.parse(value)) ? timestamp.format(new Date(value)) : '尚无更新时间'

type CommunityProps = {
  topics: CommunityTopics
  sourceStatus?: Record<string, SourceStatus>
  loading: boolean
}

export function CommunityPage({ topics, sourceStatus, sections, loading, onTopic }: CommunityProps & { sections: ContentSection[]; onTopic: (topic: CommunityTopic) => void }) {
  const [source, setSource] = useState<CommunitySource | 'all'>('all')
  const [section, setSection] = useState('all')
  const [query, setQuery] = useState('')
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 60_000)
    return () => window.clearInterval(timer)
  }, [])
  const selectedPlatforms = communityPlatforms.filter((platform) => source === 'all' || platform.source === source)
  const sourceTopics = selectedPlatforms.flatMap((platform) => topics[platform.source] ?? [])
  const needle = query.trim().toLocaleLowerCase()
  const matching = (topic: CommunityTopic) => (section === 'all' || topic.section === section) && (!needle || `${topic.keyword} ${topic.summary}`.toLocaleLowerCase().includes(needle))
  const resultCount = sourceTopics.filter(matching).length

  return <section className="community-page" aria-label="社区热门话题">
    <div className="community-intro"><div><span className="article-kicker">LIFE TV / COMMUNITY</span><h2>看看社区正在聊什么</h2><p>体育、数码、电竞与日常生活。社区讨论不等同于已核实的新闻报道。</p></div><span className="community-cadence">每 10 分钟尝试同步<br /><small>榜单由 UAPI 聚合提供</small></span></div>
    <div className="community-toolbar">
      <div className="news-filter-tabs" role="group" aria-label="社区来源">
        <button className={source === 'all' ? 'active' : ''} aria-pressed={source === 'all'} onClick={() => setSource('all')}>全部社区</button>
        {communityPlatforms.map((platform) => <button key={platform.source} className={source === platform.source ? 'active' : ''} aria-pressed={source === platform.source} onClick={() => setSource(platform.source)}>{platform.name}<span>{topics[platform.source]?.length ?? 0}</span></button>)}
      </div>
      <label className="community-search"><span aria-hidden="true">⌕</span><input type="search" aria-label="搜索社区热门话题" placeholder="搜索话题或摘要" value={query} onChange={(event) => setQuery(event.target.value)} /></label>
    </div>
    <div className="community-section-filters" role="group" aria-label="社区话题分类">
      <button className={`section-chip ${section === 'all' ? 'active' : ''}`} aria-pressed={section === 'all'} onClick={() => setSection('all')}>全部话题<span>{sourceTopics.length}</span></button>
      {sections.map((item) => <button className={`section-chip ${section === item.id ? 'active' : ''}`} key={item.id} aria-pressed={section === item.id} onClick={() => setSection(item.id)}>{item.label}<span>{sourceTopics.filter((topic) => topic.section === item.id).length}</span></button>)}
    </div>
    <div className="community-results"><p className="section-help">话题按关键词自动归类；热度口径不同，排名仅在各平台内比较。</p><span aria-live="polite">{loading ? '读取中…' : `${resultCount} 条话题`}</span></div>
    <div className={`community-boards ${source !== 'all' ? 'community-boards-single' : ''}`}>
      {selectedPlatforms.map((platform) => {
        const allRows = topics[platform.source] ?? []
        const rows = allRows.filter(matching)
        const status = sourceStatus?.[`${platform.source}Topics`]
        const updated = Date.parse(allRows[0]?.fetchedAt ?? '')
        const stale = allRows.length > 0 && (!Number.isFinite(updated) || now - updated > 7_200_000 || updated > now + 300_000)
        const unavailable = status?.state === 'unavailable'
        return <section className={`community-board community-${platform.source}`} key={platform.source} aria-label={`${platform.name}话题榜`}>
          <div className="community-board-heading"><div><h3>{platform.name}</h3><span>{platform.description}</span></div><span className={`community-status ${stale || unavailable ? 'is-stale' : ''}`}>{unavailable ? '更新受限' : stale ? '历史缓存' : allRows.length ? '已同步' : '待同步'}</span></div>
          <p className="community-updated">榜单更新：{formatTime(allRows[0]?.fetchedAt)}</p>
          {(stale || unavailable) && <p className="community-warning" role="status">{allRows.length ? '当前为上次成功同步的缓存，不代表最新热榜。' : '该来源暂时无法获取，稍后会自动重试。'}{status?.retryAfter && ` 重试不早于 ${formatTime(status.retryAfter)}。`}</p>}
          {loading ? <p className="quiet-state">正在读取话题…</p> : rows.length ? <ol className="community-topic-list">{rows.map((topic) => <li key={topic.sourceUrl}>
            <button className="community-topic-open" onClick={() => onTopic(topic)}><span className={`community-rank ${topic.rank <= 3 ? 'top' : ''}`}>{String(topic.rank).padStart(2, '0')}</span><span className="community-topic-copy"><strong>{topic.keyword}</strong><span className="community-topic-meta"><span>{topic.sectionLabel}</span><span>{topic.heatLabel || (topic.heat ? `${heat.format(topic.heat)} 热度` : '未提供热度')}</span></span><small>查看相关帖子与详情</small></span><span className="community-expand" aria-hidden="true">→</span></button>
          </li>)}</ol> : <div className="community-empty"><strong>{allRows.length ? '没有匹配的话题' : unavailable ? '暂时无法更新榜单' : '尚未同步到话题'}</strong><p>{allRows.length ? '试试其他分类或关键词。' : '不会使用示例话题代替真实数据，成功同步后自动显示。'}</p>{(section !== 'all' || needle) && <button onClick={() => { setSection('all'); setQuery('') }}>清除筛选</button>}</div>}
          <div className="community-board-footer"><span>来源：{platform.name} · UAPI 聚合</span><a href="https://uapis.cn/docs/api-reference/get-misc-hotboard" target="_blank" rel="noreferrer">接口说明 ↗</a></div>
        </section>
      })}
    </div>
  </section>
}

export function CommunityPreview({ topics, sourceStatus, loading, onOpen }: CommunityProps & { onOpen: () => void }) {
  return <section className="community-preview" aria-label="社区热议速览">
    <div className="news-section-heading"><h2>社区热议</h2><span>COMMUNITY</span></div>
    {communityPlatforms.map((platform) => <div className="community-preview-source" key={platform.source}><h3>{platform.name}<span>{sourceStatus?.[`${platform.source}Topics`]?.state === 'unavailable' ? '更新受限' : formatTime(topics[platform.source]?.[0]?.fetchedAt)}</span></h3>{topics[platform.source]?.length ? <ol>{topics[platform.source]?.slice(0, 1).map((topic) => <li key={topic.sourceUrl}><span>{String(topic.rank).padStart(2, '0')}</span>{topic.keyword}</li>)}</ol> : <p>{loading ? '正在读取…' : '暂无已同步话题'}</p>}</div>)}
    <button className="news-load-more" onClick={onOpen}>浏览全部社区话题 <span>→</span></button>
  </section>
}
