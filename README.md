# LIFE TV

面向个人学习的全球新闻、热搜与 CS2 中文资讯站。前端使用 React + Vite，后端使用 Gin，数据存入 MySQL。

源码仓库：[`s1mple-api.github.io`](https://github.com/s1mple-api/s1mple-api.github.io)。生产环境部署在阿里云：Nginx 监听 `8848` 端口提供前端静态文件，Gin 后端监听 `8080` 端口。前端生产构建的 API 地址为 `http://47.111.131.153:8080`。示例 Nginx 配置见 `deploy/nginx/life.conf`。

部署前端：在 `frontend` 目录运行 `npm ci && npm run build`，再把 `frontend/dist` 内容放到 Nginx 配置的站点目录。后端启动时设置 `APP_PORT=8080`、`MYSQL_DSN` 和 `CORS_ORIGIN=http://47.111.131.153:8848`。由于前后端端口不同，Gin 需要允许该前端源的跨域请求；阿里云安全组需开放 TCP `8080` 和 `8848`。请勿把真实数据库口令提交到仓库。

## 功能

- Steam `appid=730` 官方新闻每 15 分钟同步到 MySQL
- BBC World RSS 每 15 分钟同步；站内文章详情按需读取 BBC 公开报道正文和配图，自动分段翻译为中文，保留原始报道链接
- 百度官方热搜榜与微博热搜榜每 10 分钟同步；微博数据来自第三方 UAPI 聚合接口，页面明确标注采集路径
- “社区热议”每 10 分钟通过 UAPI 同步贴吧、虎扑、B 站、小红书和抖音热榜，每平台最多保留 30 条（小红书实际返回 20 条）；支持来源、分类及关键词筛选，话题、帖子和评论使用站内详情页
- 贴吧公开话题页按需读取相关帖子列表；虎扑公开搜索页读取帖子列表，公开详情页读取正文、图片、热评和分页评论；B 站通过 UAPI 读取视频详情及公开评论，支持官方播放器入口、评论排序和子回复预览
- HLTV 新闻和赛程每 5 分钟同步；HLTV 战队排名、阵容和近 90 天选手 Rating 每小时同步
- HLTV 赛事日历每 30 分钟同步后续赛事；按当前时间展示仍在规划或进行中的赛事，可按年份、系列和 HLTV 提供的赛事类型筛选
- Valve VRS 全球及区域快照从 [Valve 官方 Regional Standings 仓库](https://github.com/ValveSoftware/counter-strike_regional_standings)按小时检查并缓存；它是周期快照，不是实时榜单
- 选手 Rating 支持近 30、90、180 天和 1 年；切换周期后按需读取并缓存
- 新资讯标题与摘要自动翻译为中文；文章正文在打开站内详情页时按需读取、缓存并分段翻译
- Gin 仪表盘、站内文章详情、赛事、战队、选手和关键词搜索 API
- React 首页混合展示 CS2 与世界新闻：首屏兼顾两类报道，侧栏显示综合热度榜和实时热搜，新闻流支持按栏目筛选、按热度或时间排序
- 新闻细分栏目：世界新闻分为国际时政、财经商业、科技数码、社会民生、科学健康、体育赛事、文化娱乐及其他国际；CS2 分为赛事战报、转会阵容、游戏更新、专访观点、综合资讯。按中英文标题／摘要自动归类，非来源网站官方标签；筛选数量仅统计当前加载的报道
- LIFE TV 综合热度由发布时间（最高 55 分）、百度／微博话题关联（最高 30 分）和近 7 天站内阅读（最高 15 分）计算；一个浏览器会话对同一报道只计一次阅读
- 文章站内阅读并可跳转信息源；世界新闻、CS2 新闻、赛事、战队和选手等栏目使用 Tab 切换
- Docker Compose MySQL 开发环境

## 启动

1. 复制环境变量：`cp .env.example .env`，在 `.env` 填写 MySQL 密码。
2. 启动 MySQL：`docker compose up -d`。
3. 启动后端：`cd backend && go mod tidy && go run ./cmd/server`。
4. 新开终端启动前端：`cd frontend && npm install && npm run dev`。
5. 打开 `http://localhost:5173`。

后端默认地址为 `http://localhost:8080`；健康检查为 `GET /healthz`。

## 数据源设置

- Steam 新闻始终启用，读取公开的 `ISteamNews/GetNewsForApp/v2`，游戏 ID 为 `730`。
- 世界新闻读取 [BBC World RSS](https://feeds.bbci.co.uk/news/world/rss.xml)，正文只在打开站内详情时请求 BBC 公开文章页；视频或特殊页面没有可解析正文时显示明确状态及原文链接。
- 百度热搜读取 [百度官方实时榜单](https://top.baidu.com/board?tab=realtime)；微博热搜读取 [UAPI 微博热榜聚合接口](https://uapis.cn/docs/api-reference/get-misc-hotboard)。两种热度口径不同，仅各自榜单内排序。聚合接口的可用性取决于第三方服务。
- 五个平台使用同一 [UAPI 热榜接口](https://uapis.cn/docs/api-reference/get-misc-hotboard)，类型为 `tieba`、`hupu`、`bilibili`、`xiaohongshu`、`douyin`。热榜只提供话题信息；详情页会按需请求公开帖子数据。贴吧话题页可获取帖子标题、摘要、作者及回复数；帖子正文若要求安全验证，会保留摘要并显示访问受限状态。虎扑使用公开 HTML 中的帖子列表和 `__NEXT_DATA__` 结构读取正文、图片、热评、普通分页评论及引用内容，跳过隐藏／删除评论。
- B 站视频信息使用 [UAPI 视频详情](https://uapis.cn/docs/api-reference/get-social-bilibili-videoinfo)，评论使用 [UAPI 评论接口](https://uapis.cn/docs/api-reference/get-social-bilibili-replies)。描述与正文以纯文本／允许的图片链接呈现；播放器由用户点击后加载 B 站官方嵌入页面。UAPI 匿名接口存在额度和上游限制：本机实测热门评论可返回部分内容，但有些排序／后续页返回无效分页数据，页面会显示受限状态，不能保证读取全部评论。子回复只展示来源实际返回的预览，并显示原始回复总数。
- 小红书和抖音公开热榜已接入；当前未取得可用的匿名话题帖子列表、正文及评论数据。详情页明确显示获取范围和限制。未接入登录 Cookie、平台授权或付费第三方全文接口。
- 社区话题和帖子使用独立的永久键，热榜轮换不会删除已打开内容的地址；新增 `community_resources` 表保存元数据和按页缓存。帖子列表、正文、评论缓存 5 分钟，同一内容的并发读取合并；更新失败保留先前成功数据并标记为缓存。只请求预期平台域名，拒绝跨站重定向；不执行来源脚本或向前端发送未经处理的 HTML。
- 社区榜单保留上游更新时间，拒收超过 2 小时或明显来自未来的数据；拒绝非预期站点链接。同步失败不清空上次成功缓存，页面明确标注“历史缓存／更新受限”；不会生成演示数据补位。不同社区热度不进行跨站相加，也不影响现有新闻综合热度算法。
- HLTV 采集默认开启；如需停止，在 `.env` 中把 `ENABLE_HLTV_COLLECTOR` 改为 `false`。HLTV 页面请求使用 `COLLECTOR_USER_AGENT` 并遵守请求间隔。
- 中文翻译默认使用 [UAPI 翻译接口](https://uapis.cn/docs/api-reference/post-translate-text)，先译标题再补译摘要，并缓存结果；正文按段落分块翻译。尚未完成翻译的新闻会留在队列中重试。第三方接口有使用限制，持续使用建议配置自己的翻译服务；`TRANSLATION_URL` 仍可指定兼容 MyMemory 的端点。
- 采集器不包含验证码绕过、代理轮换或其他规避反爬措施。若页面返回拒绝访问，保持关闭即可；前端与 Steam 新闻仍可正常工作。

## API

- `GET /api/v1/dashboard`：首页数据
- 仪表盘包含 `newsSections`、带 `section` / `sectionLabel` 的 `newsFeed`，以及 `communityTopics`（按五个平台分组）、`communitySections`；社区同步状态使用 `sourceStatus.<平台>Topics`。兼容保留 `tiebaTopics`、`hupuTopics`
- `POST /api/v1/articles/:id/read`：匿名记录站内阅读，并过滤重复记录，用于计算综合热度
- `GET /api/v1/world-news?limit=30`：BBC 世界新闻
- `GET /api/v1/hot-search?source=baidu|weibo`：热搜榜单
- `GET /api/v1/community?source=tieba|hupu|bilibili|xiaohongshu|douyin&limit=30`：社区话题快照（分类筛选／搜索在前端对当前快照执行，保留各平台原始排名）
- `GET /api/v1/community/topics/:key?page=1`：话题相关帖子列表
- `GET /api/v1/community/posts/:key`：帖子正文或视频详情
- `GET /api/v1/community/posts/:key/comments?page=1&sort=hot|time`：公开评论页，页面范围 1–100；虎扑 `hot` 为公开页热评集合，普通分页使用 `time`
- 前端站内路由：`/topic/:key`、`/topic/:topicKey/post/:postKey`；帖子也可使用 `/post/:postKey` 独立地址
- `GET /api/v1/articles/:id`：站内文章详情
- `GET /api/v1/players?period=30d|90d|180d|365d`、`GET /api/v1/players/:id`：选手 Rating 排名与详情
- `GET /api/v1/events?year=2026&series=IEM&type=Big Events`：HLTV 后续赛事，可按年份、系列和页面提供的赛事类型筛选
- `GET /api/v1/rankings?source=hltv|valve&region=global|europe|americas|asia`：分来源战队排名
- `GET /api/v1/search?q=关键词`：搜索资讯和选手

HLTV 页面请求可能返回 403；本项目只使用 HLTV 来源，不会改用其他网站混补，也不包含验证码绕过、代理轮换或其他访问控制规避。发生拒绝访问时采集器冷却后重试，前端明确显示不可用状态，不填充演示数据。HLTV 榜单与 Valve VRS 使用不同算法和更新节奏，分别展示。

更新社区栏目版本时，需重启后端并重新构建前端。自动迁移会为 `trends` 表新增 `heat_label`、`summary` 字段，并创建 `community_resources` 表；会为现存社区榜单补存话题快照，无需清空数据库。
