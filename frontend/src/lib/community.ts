import type { CommunitySource } from './api'

export const communityPlatforms: { source: CommunitySource; name: string; description: string }[] = [
  { source: 'tieba', name: '百度贴吧', description: '热门话题 · 公开摘要' },
  { source: 'hupu', name: '虎扑', description: '热搜话题 · 讨论入口' },
  { source: 'xiaohongshu', name: '小红书', description: '热搜话题 · 笔记动态' },
]

// Match backend/model.CommunityKey so older dashboard responses that predate
// the `key` field can still open the same stored topic and post records.
export async function communityResourceKey(source: string, identity: string) {
  if (!source || !identity) throw new Error('内容来源信息不完整，请刷新后重试。')
  const input = new TextEncoder().encode(`${source}\n${identity}`)
  if (!globalThis.crypto?.subtle) throw new Error('当前浏览器无法生成内容链接，请使用 HTTPS 或 localhost 打开。')
  const digest = new Uint8Array(await globalThis.crypto.subtle.digest('SHA-256', input))
  return Array.from(digest.slice(0, 12), (byte) => byte.toString(16).padStart(2, '0')).join('')
}
