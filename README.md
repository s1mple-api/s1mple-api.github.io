# LIFE TV

面向个人学习的全球新闻、热搜与 CS2 中文资讯站。前端使用 React + Vite，后端使用 Gin，数据存入 MySQL。

源码仓库：[`s1mple-api.github.io`](https://github.com/s1mple-api/s1mple-api.github.io)。生产环境部署在阿里云：Nginx 监听 `8848` 端口提供前端静态文件，Gin 后端监听 `8080` 端口。前端生产构建的 API 地址为 `http://47.111.131.153:8080`。示例 Nginx 配置见 `deploy/nginx/life.conf`。

部署前端：在 `frontend` 目录运行 `npm ci && npm run build`，再把 `frontend/dist` 内容放到 Nginx 配置的站点目录。后端启动时设置 `APP_PORT=8080`、`MYSQL_DSN` 和 `CORS_ORIGIN=http://47.111.131.153:8848`。由于前后端端口不同，Gin 需要允许该前端源的跨域请求；阿里云安全组需开放 TCP `8080` 和 `8848`。请勿把真实数据库口令提交到仓库。

## 功能

- Steam `appid=730` 官方新闻每 15 分钟同步到 MySQL
- BBC World RSS 每 15 分钟同步；站内文章详情按需读取 BBC 公开报道正文和配图，自动分段翻译为中文，保留原始报道链接
- 百度官方热搜榜与微博热搜榜每 10 分钟同步；微博数据来自第三方 UAPI 聚合接口，页面明确标注采集路径
- HLTV 新闻和赛程每 5 分钟同步；HLTV 战队排名、阵容和近 90 天选手 Rating 每小时同步
- HLTV 赛事日历每 30 分钟同步后续赛事；按当前时间展示仍在规划或进行中的赛事，可按年份、系列和 HLTV 提供的赛事类型筛选
- Valve VRS 全球及区域快照从 [Valve 官方 Regional Standings 仓库](https://github.com/ValveSoftware/counter-strike_regional_standings)按小时检查并缓存；它是周期快照，不是实时榜单
- 选手 Rating 支持近 30、90、180 天和 1 年；切换周期后按需读取并缓存
- 新资讯标题与摘要自动翻译为中文；文章正文在打开站内详情页时按需读取、缓存并分段翻译
- Gin 仪表盘、站内文章详情、赛事、战队、选手和关键词搜索 API
- React 首页混合展示 CS2 与世界新闻：首屏兼顾两类报道，侧栏显示综合热度榜和实时热搜，新闻流支持按栏目筛选、按热度或时间排序
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
- HLTV 采集默认开启；如需停止，在 `.env` 中把 `ENABLE_HLTV_COLLECTOR` 改为 `false`。HLTV 页面请求使用 `COLLECTOR_USER_AGENT` 并遵守请求间隔。
- 中文翻译默认使用 [UAPI 翻译接口](https://uapis.cn/docs/api-reference/post-translate-text)，先译标题再补译摘要，并缓存结果；正文按段落分块翻译。尚未完成翻译的新闻会留在队列中重试。第三方接口有使用限制，持续使用建议配置自己的翻译服务；`TRANSLATION_URL` 仍可指定兼容 MyMemory 的端点。
- 采集器不包含验证码绕过、代理轮换或其他规避反爬措施。若页面返回拒绝访问，保持关闭即可；前端与 Steam 新闻仍可正常工作。

## API

- `GET /api/v1/dashboard`：首页数据
- `POST /api/v1/articles/:id/read`：匿名记录站内阅读，并过滤重复记录，用于计算综合热度
- `GET /api/v1/world-news?limit=30`：BBC 世界新闻
- `GET /api/v1/hot-search?source=baidu|weibo`：热搜榜单
- `GET /api/v1/articles/:id`：站内文章详情
- `GET /api/v1/players?period=30d|90d|180d|365d`、`GET /api/v1/players/:id`：选手 Rating 排名与详情
- `GET /api/v1/events?year=2026&series=IEM&type=Big Events`：HLTV 后续赛事，可按年份、系列和页面提供的赛事类型筛选
- `GET /api/v1/rankings?source=hltv|valve&region=global|europe|americas|asia`：分来源战队排名
- `GET /api/v1/search?q=关键词`：搜索资讯和选手

HLTV 页面请求可能返回 403；本项目只使用 HLTV 来源，不会改用其他网站混补，也不包含验证码绕过、代理轮换或其他访问控制规避。发生拒绝访问时采集器冷却后重试，前端明确显示不可用状态，不填充演示数据。HLTV 榜单与 Valve VRS 使用不同算法和更新节奏，分别展示。
