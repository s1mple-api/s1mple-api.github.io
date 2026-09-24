import { useState, type ReactNode } from 'react'
import type { Article, NewsCategory, RankedArticle, SourceStatus } from '../lib/api'

const publishedDate = new Intl.DateTimeFormat('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
const categoryName = (category: NewsCategory) => category === 'world' ? '世界新闻' : 'CS2'
const sourceName = (source: string) => source === 'bbc' ? 'BBC News' : source === 'hltv' ? 'HLTV' : 'Steam 官方'
const titleOf = (article: Article) => article.titleZh || article.title

export function NewsHome({ stories, loading, onArticle, hotPanel, sourceStatus }: {
  stories: RankedArticle[]
  loading: boolean
  onArticle: (article: Article) => void
  hotPanel: ReactNode
  sourceStatus?: Record<string, SourceStatus>
}) {
  const lead = stories[0]
  // Keep the highest-scoring article as lead and guarantee that the other
  // available category also gets a first-screen position.
  const counterpart = stories.find((story) => story.category !== lead?.category)
  const third = stories.find((story) => story.id !== lead?.id && story.id !== counterpart?.id)
  const supporting = [counterpart, third].filter((story): story is RankedArticle => Boolean(story))

  return <div className="news-layout">
    <div className="news-main">
      <section aria-labelledby="focus-title" className="focus-section">
        <div className="news-section-heading"><h2 id="focus-title">焦点报道<span>THE BIG PICTURE</span></h2><span className="edition-live"><i />综合热度精选</span></div>
        {loading ? <div className="news-skeleton" aria-label="正在加载新闻" /> : lead ? <div className="focus-grid">
          <NewsCard article={lead} onClick={() => onArticle(lead)} variant="lead" />
          <div className="focus-support">{supporting.map((article) => <NewsCard key={article.id} article={article} onClick={() => onArticle(article)} variant="support" />)}</div>
        </div> : <div className="home-empty"><strong>正在整理最新报道</strong><p>新闻同步后会自动显示在这里。</p></div>}
      </section>
      <NewsFeed stories={stories} loading={loading} onArticle={onArticle} />
      {sourceStatus?.news?.state === 'unavailable' && <p className="news-cache-note">部分 CS2 报道显示最近缓存，发布时间已标注。</p>}
      {sourceStatus?.bbcNews?.state === 'unavailable' && <p className="news-cache-note">世界新闻暂未更新，当前可阅读已同步报道。</p>}
    </div>
    <aside className="news-rail">
      <section className="news-heat-list" aria-labelledby="news-heat-title">
        <div className="news-section-heading"><h2 id="news-heat-title">新闻热度榜</h2><span>TOP 05</span></div>
        {loading ? <p className="quiet-state">正在整理榜单…</p> : <ol>{stories.slice(0, 5).map((article, index) => <li key={article.id}>
          <button onClick={() => onArticle(article)}><span className="news-heat-rank">{String(index + 1).padStart(2, '0')}</span><span className="news-heat-copy"><small>{categoryName(article.category)} · {sourceName(article.source)}</small><strong>{titleOf(article)}</strong><span className="heat-track"><i style={{ width: `${article.heatScore}%` }} /></span></span><span className="news-heat-value">{article.heatScore.toFixed(1)}</span></button>
        </li>)}</ol>}
        <details className="heat-method"><summary>热度如何计算 <span>＋</span></summary><p>LIFE TV 综合指数，范围 0–100。结合新闻时效（最高 55 分）、与百度／微博热搜的主题关联（最高 30 分）和近 7 天本站阅读记录（最高 15 分）。</p><p>时间越久，得分越低。同一会话重复打开同一报道只计一次阅读。首屏兼顾世界新闻与 CS2，热度榜按得分排序。</p></details>
      </section>
      {hotPanel}
      <div className="edition-note"><span>LIFE TV</span><p>从世界，到赛场。</p><small>重要的动态，在这里相遇。</small></div>
    </aside>
  </div>
}

export function NewsCollection({ stories, category, loading, onArticle }: { stories: RankedArticle[]; category: NewsCategory; loading: boolean; onArticle: (article: Article) => void }) {
  return <div className="news-collection"><NewsFeed key={category} stories={stories} loading={loading} onArticle={onArticle} fixedCategory={category} /></div>
}

function NewsFeed({ stories, loading, onArticle, fixedCategory }: { stories: RankedArticle[]; loading: boolean; onArticle: (article: Article) => void; fixedCategory?: NewsCategory }) {
  const [category, setCategory] = useState<NewsCategory | 'all'>(fixedCategory || 'all')
  const [sort, setSort] = useState<'heat' | 'latest'>('heat')
  const [visibleCount, setVisibleCount] = useState(12)
  const filtered = stories.filter((article) => category === 'all' || article.category === category)
  const ordered = [...filtered].sort((a, b) => sort === 'heat' ? b.heatScore - a.heatScore || Date.parse(b.publishedAt) - Date.parse(a.publishedAt) : Date.parse(b.publishedAt) - Date.parse(a.publishedAt) || b.id - a.id)
  const filters: { key: NewsCategory | 'all'; label: string }[] = [{ key: 'all', label: '全部' }, { key: 'world', label: '世界新闻' }, { key: 'cs2', label: 'CS2 新闻' }]

  return <section className="news-feed" aria-label={fixedCategory ? `${categoryName(fixedCategory)}报道` : '全部报道'}>
    <div className="news-section-heading"><h2>{fixedCategory ? `${categoryName(fixedCategory)}报道` : '全部报道'}<span>THE LATEST</span></h2><span>{filtered.length} 篇报道</span></div>
    <div className="news-feed-controls">
      {!fixedCategory ? <div className="news-filter-tabs" role="tablist" aria-label="新闻类型">{filters.map((filter) => <button key={filter.key} role="tab" aria-selected={category === filter.key} className={category === filter.key ? 'active' : ''} onClick={() => { setCategory(filter.key); setVisibleCount(12) }}>{filter.label}<span>{filter.key === 'all' ? stories.length : stories.filter((story) => story.category === filter.key).length}</span></button>)}</div> : <p>追踪{fixedCategory === 'world' ? '全球新闻' : '赛场内外'}，站内阅读全文。</p>}
      <label className="news-sort"><span className="sr-only">新闻排序</span><select aria-label="新闻排序" value={sort} onChange={(event) => { setSort(event.target.value as 'heat' | 'latest'); setVisibleCount(12) }}><option value="heat">综合热度 ↓</option><option value="latest">最新发布 ↓</option></select></label>
    </div>
    {loading ? <div className="news-skeleton" aria-label="正在加载报道" /> : ordered.length ? <div className="news-feed-grid">{ordered.slice(0, visibleCount).map((article) => <NewsCard article={article} key={article.id} variant="feed" onClick={() => onArticle(article)} />)}</div> : <p className="quiet-state">这个栏目正在同步，稍后会自动更新。</p>}
    {ordered.length > visibleCount && <button className="news-load-more" onClick={() => setVisibleCount((count) => count + 12)}>加载更多报道 <span>↓</span></button>}
  </section>
}

function NewsCard({ article, onClick, variant }: { article: RankedArticle; onClick: () => void; variant: 'lead' | 'support' | 'feed' }) {
  const summary = (article.summaryZh || article.summary).replace(/\s+/g, ' ').trim()
  const shortSummary = summary.length > 130 ? `${summary.slice(0, 130)}…` : summary
  const cardClass = variant === 'lead' ? 'news-card-lead' : variant === 'support' ? 'news-card-support' : 'news-card-feed'
  const categoryClass = article.category === 'cs2' ? 'news-category-cs2' : 'news-category-world'
  return <article className={`news-card ${cardClass}`} data-category={article.category} data-article-id={article.id}>
    <button onClick={onClick} className="news-card-link">
      <StoryVisual key={`${article.id}:${article.imageUrl}`} article={article} eager={variant === 'lead'} />
      <div className="news-card-content">
        <div className="news-card-topline"><span className={`news-category ${categoryClass}`}>{categoryName(article.category)}</span><HeatBadge article={article} /></div>
        <h3>{titleOf(article)}</h3>
        {summary && <p className="news-card-summary">{shortSummary}</p>}
        <div className="news-card-meta"><span>{sourceName(article.source)}</span><time dateTime={article.publishedAt}>{publishedDate.format(new Date(article.publishedAt))}</time></div>
        {variant === 'lead' && <span className="news-read-link">阅读完整报道 <span>→</span></span>}
      </div>
    </button>
  </article>
}

function StoryVisual({ article, eager }: { article: RankedArticle; eager: boolean }) {
  const [broken, setBroken] = useState(false)
  return <div className={`news-visual news-visual-${article.category}`} aria-hidden="true">
    {article.imageUrl && !broken ? <img src={article.imageUrl} alt="" loading={eager ? 'eager' : 'lazy'} onError={() => setBroken(true)} /> : <div className="news-visual-poster"><small>{sourceName(article.source)} / NEWSROOM</small><strong>{article.category === 'world' ? 'WORLD' : 'CS2'}</strong><span>{article.category === 'world' ? '全球动态' : article.source === 'steam' ? '游戏更新 · 官方动态' : '赛事 · 战队 · 选手'}</span></div>}
  </div>
}

function HeatBadge({ article }: { article: RankedArticle }) {
  const explanation = `LIFE 综合热度 ${article.heatScore.toFixed(1)} / 100；时效 ${article.heat.freshness}，话题 ${article.heat.topics}，阅读 ${article.heat.reading}${article.heat.matches.length ? `；关联话题：${article.heat.matches.join('、')}` : ''}`
  return <span className="news-heat-badge" title={explanation}><svg viewBox="0 0 16 16" aria-hidden="true"><path d="M2 11 6 7l3 2 5-6M10 3h4v4" /></svg>{article.heatScore.toFixed(1)}<span className="sr-only">综合热度</span></span>
}
