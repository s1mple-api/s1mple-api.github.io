import { useEffect, useState } from 'react'
import { communityImageURL, getCommunityComments, getCommunityPost, getCommunityTopic, type CommunityComment, type CommunityComments, type CommunityPostDetail, type CommunityPostList, type CommunityState, type ContentBlock } from '../lib/api'
import { communityPlatforms } from '../lib/community'

const platformName = (source: string) => communityPlatforms.find((platform) => platform.source === source)?.name || source
const date = new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' })
const count = new Intl.NumberFormat('zh-CN', { notation: 'compact', maximumFractionDigits: 1 })
const formatDate = (value?: string) => value && Number.isFinite(Date.parse(value)) ? date.format(new Date(value)) : ''

export function CommunityTopicPage({ topicKey, onBack, onPost }: { topicKey: string; onBack: () => void; onPost: (post: CommunityPostList['posts'][number]) => void }) {
  const [page, setPage] = useState(1)
  const [attempt, setAttempt] = useState(0)
  const identity = `${topicKey}:${page}:${attempt}`
  const [response, setResponse] = useState<{ identity: string; data?: CommunityPostList; error?: string } | null>(null)
  useEffect(() => {
    let active = true
    getCommunityTopic(topicKey, page, attempt > 0).then((data) => { if (active) setResponse({ identity, data }) }).catch(() => { if (active) setResponse({ identity, error: '暂时无法打开这个话题，请稍后再试。' }) })
    return () => { active = false }
  }, [topicKey, page, identity, attempt])
  const busy = response?.identity !== identity
  const data = !busy ? response?.data : undefined
  return <section className="community-detail-page">
    <button className="community-back" onClick={onBack}>← 返回社区热议</button>
    <header className="community-detail-header"><span className="article-kicker">{data ? `${platformName(data.topic.source)} / 话题` : 'LIFE TV / COMMUNITY'}</span><h1>{data?.topic.title || '话题详情'}</h1>{data?.topic.summary && <p>{data.topic.summary}</p>}{data && <div className="community-detail-meta"><span>{data.topic.heatLabel || '热门话题'}</span><span>榜单更新 {formatDate(data.topic.fetchedAt)}</span><a href={data.topic.sourceUrl} target="_blank" rel="noreferrer">查看来源话题 ↗</a></div>}</header>
    <div className="news-section-heading"><h2>相关帖子</h2><span>第 {page} 页 <button disabled={busy} onClick={() => setAttempt((value) => value + 1)}>重新读取</button></span></div>
    {data && <DataState status={data.status} />}
    {busy ? <div className="loading-panel">正在读取相关帖子、正文及评论…<p>首次读取可能需要约一分钟，请保持已授权的浏览器和平台登录状态可用。</p></div> : response?.error ? <p className="community-warning" role="alert">{response.error}</p> : data?.posts.length ? <div className="community-post-grid">{data.posts.map((post, index) => <article className="community-post-card" key={post.key || post.sourceUrl || `${post.source}:${index}`}><button onClick={() => onPost(post)}>{post.coverUrl && <SafeImage url={post.coverUrl} className="community-post-cover" />}<div><small>{platformName(post.source)}{post.author && ` · ${post.author}`}</small><h3>{post.title}</h3>{post.summary && <p>{post.summary}</p>}<span>{post.publishedAt && `${formatDate(post.publishedAt)} · `}{count.format(post.commentCount)} 条评论 <i>阅读详情 →</i></span></div></button></article>)}</div> : <div className="community-empty"><strong>{data?.status.state === 'unavailable' ? '暂时无法读取相关帖子' : '暂无相关公开帖子'}</strong><p>{data?.status.state === 'unavailable' ? '请按上方提示检查连接或登录状态，再点击重新读取。' : '当前页没有可展示的帖子。'}</p></div>}
    <Pagination page={page} hasMore={data?.hasMore ?? false} busy={busy} onPage={setPage} />
  </section>
}

export function CommunityPostPage({ postKey, onBack }: { postKey: string; onBack: () => void }) {
  const [response, setResponse] = useState<{ data?: CommunityPostDetail; error?: string } | null>(null)
  useEffect(() => {
    let active = true
    getCommunityPost(postKey).then((data) => { if (active) setResponse({ data }) }).catch(() => { if (active) setResponse({ error: '暂时无法打开这篇帖子，请稍后再试。' }) })
    return () => { active = false }
  }, [postKey])
  const data = response?.data
  const post = data?.post
  return <section className="community-detail-page community-post-page">
    <button className="community-back" onClick={onBack}>← 返回话题帖子</button>
    {!response ? <div className="loading-panel">正在读取帖子详情…</div> : response.error ? <p className="community-warning" role="alert">{response.error}</p> : post && data && <>
      <header className="community-detail-header"><span className="article-kicker">{platformName(post.source)} / 帖子详情</span><h1>{post.title}</h1><div className="community-author">{post.avatarUrl && <SafeImage url={post.avatarUrl} className="community-avatar" />}<div><strong>{post.author || platformName(post.source)}</strong><span>{formatDate(post.publishedAt)}</span></div></div><div className="community-detail-meta">{post.views > 0 && <span>{count.format(post.views)} 次浏览</span>}{post.likes > 0 && <span>{count.format(post.likes)} 次赞同</span>}<span>{count.format(post.commentCount)} 条评论</span><a href={post.sourceUrl} target="_blank" rel="noreferrer">原始帖子 ↗</a></div></header>
      <DataState status={data.status} />
      <div className="community-post-body"><h2>帖子正文</h2>{post.body.length ? <RichContent blocks={post.body} /> : data.status.state === 'unavailable' ? <p className="quiet-state">正文暂时无法读取。{post.summary && '下方为话题页提供的帖子摘要。'}</p> : <p className="quiet-state">来源未提供文字正文。</p>}{!post.body.length && post.summary && <blockquote className="community-excerpt"><small>帖子列表摘要</small><p>{post.summary}</p></blockquote>}</div>
      {data.status.state === 'unavailable' ? <section className="community-comments"><h2>评论</h2><p className="quiet-state">当前来源暂时无法公开读取评论。</p></section> : <CommentSection key={post.key} postKey={post.key} source={post.source} />}
    </>}
  </section>
}

function CommentSection({ postKey, source }: { postKey: string; source: string }) {
  const [page, setPage] = useState(1)
  const [sort, setSort] = useState(source === 'tieba' ? 'time' : 'hot')
  const [response, setResponse] = useState<{ identity: string; data?: CommunityComments; error?: string } | null>(null)
  const identity = `${sort}:${page}`
  useEffect(() => {
    let active = true
    getCommunityComments(postKey, page, sort).then((data) => { if (active) setResponse({ identity, data }) }).catch(() => { if (active) setResponse({ identity, error: '评论暂时无法读取，请稍后再试。' }) })
    return () => { active = false }
  }, [postKey, page, sort, identity])
  const busy = response?.identity !== identity
  const data = !busy ? response?.data : undefined
  return <section className="community-comments" aria-label="帖子评论">
    <div className="news-section-heading"><h2>评论<span>{data && data.status.state !== 'unavailable' ? `${data.total} 条${sort === 'hot' && source === 'hupu' ? '公开热评' : ''}` : ''}</span></h2>{source !== 'tieba' && <div className="news-filter-tabs" role="group" aria-label="评论排序"><button className={sort === 'hot' ? 'active' : ''} aria-pressed={sort === 'hot'} onClick={() => { setSort('hot'); setPage(1) }}>热门评论</button><button className={sort === 'time' ? 'active' : ''} aria-pressed={sort === 'time'} onClick={() => { setSort('time'); setPage(1) }}>按时间</button></div>}</div>
    {data && <DataState status={data.status} />}
    {data?.partial && <p className="quiet-state">目前仅展示已获取的部分评论及回复，并非全部 {data.total} 条评论。</p>}
    {busy ? <p className="quiet-state">正在读取评论…</p> : response?.error ? <p className="community-warning" role="alert">{response.error}</p> : data?.comments.length ? <ol className="community-comments-list">{data.comments.map((comment) => <li key={comment.id}><Comment comment={comment} /></li>)}</ol> : <p className="quiet-state">{data?.status.state === 'unavailable' ? '当前未能获取评论。' : '当前页暂无公开评论。'}</p>}
    <Pagination page={page} hasMore={data?.hasMore ?? false} busy={busy} onPage={setPage} />
  </section>
}

function Comment({ comment }: { comment: CommunityComment }) {
  return <article className="community-comment">
    <div className="community-comment-heading">{comment.avatarUrl && <SafeImage url={comment.avatarUrl} className="community-avatar" />}<div><strong>{comment.author || '社区用户'}</strong><span>{formatDate(comment.publishedAt)}</span></div><small>{count.format(comment.likes)} 赞</small></div>
    {comment.quote && <blockquote className="community-comment-quote"><strong>引用 {comment.quote.author}</strong><RichContent blocks={comment.quote.body} /></blockquote>}
    <RichContent blocks={comment.body} />
    {comment.replies.length > 0 && <details className="community-comment-replies"><summary>查看已获取回复（{comment.replies.length} 条{comment.replyCount > comment.replies.length ? ` / 共 ${comment.replyCount} 条` : ''}）</summary>{comment.replies.map((reply) => <Comment comment={reply} key={reply.id} />)}</details>}
    {comment.replyCount > 0 && !comment.replies.length && <small className="community-reply-count">来源标注 {comment.replyCount} 条回复，当前页面未提供子回复内容。</small>}
  </article>
}

function RichContent({ blocks }: { blocks: ContentBlock[] }) {
  return <div className="community-rich-content">{blocks.map((block, index) => block.type === 'image' && block.url ? <SafeImage url={block.url} key={`${index}:${block.url}`} className="community-inline-image" /> : block.text ? <p key={index}>{block.text}</p> : null)}</div>
}

function SafeImage({ url, className }: { url: string; className: string }) {
  return <SourceImage key={url} url={url} className={className} />
}

function SourceImage({ url, className }: { url: string; className: string }) {
  const [failed, setFailed] = useState(false)
  return !failed ? <img src={communityImageURL(url)} alt="" className={className} loading="lazy" referrerPolicy="no-referrer" onError={() => setFailed(true)} /> : <span className={`${className} community-image-unavailable`}>图片暂时不可用</span>
}

function DataState({ status }: { status: CommunityState }) {
  return <div className="community-data-state">{status.message && <p className="community-warning" role="status">{status.message}</p>}<small>读取于 {formatDate(status.fetchedAt)}{status.cached ? ' · 缓存内容' : ''}</small></div>
}

function Pagination({ page, hasMore, busy, onPage }: { page: number; hasMore: boolean; busy: boolean; onPage: (page: number) => void }) {
  return page > 1 || hasMore ? <div className="community-pagination"><button disabled={page === 1 || busy} onClick={() => onPage(page - 1)}>← 上一页</button><span>第 {page} 页</span><button disabled={!hasMore || busy || page >= 100} onClick={() => onPage(page + 1)}>下一页 →</button></div> : null
}
