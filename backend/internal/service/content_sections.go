package service

import (
	"strings"
	"unicode"

	"github.com/example/cs-pulse/backend/internal/model"
)

type ContentSection struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Category string `json:"category"`
}

type sectionRule struct {
	ContentSection
	keywords string
}

// These are local, explainable keyword categories, not upstream editorial tags.
// Classify original and translated text so pending translations still work.
var contentRules = []sectionRule{
	{ContentSection{"cs2-roster", "转会阵容", "cs2"}, "roster|transfer|transfers|sign|signs|signed|signing|bench|benched|join|joins|joining|depart|departs|departure|retirement|retires|转会|阵容|签约|加盟|离队|退役|替补"},
	{ContentSection{"cs2-interviews", "专访观点", "cs2"}, "interview|interviews|opinion|analysis|专访|采访|访谈|观点|分析|谈到|表示"},
	{ContentSection{"cs2-updates", "游戏更新", "cs2"}, "update|updates|patch|patches|release notes|vac|anti cheat|游戏更新|更新日志|补丁|反作弊|地图改动|平衡调整"},
	{ContentSection{"cs2-events", "赛事战报", "cs2"}, "tournament|playoffs|final|finals|win|wins|won|beat|defeat|qualify|qualifier|eliminate|eliminated|champion|championship|blast|iem|major|pgl|esl|赛事|决赛|夺冠|冠军|晋级|淘汰|战胜|击败|赛程|战报"},
	{ContentSection{"cs2-general", "综合资讯", "cs2"}, ""},
	{ContentSection{"world-technology", "科技数码", "world"}, "technology|tech|ai|artificial intelligence|openai|smartphone|iphone|chip|chips|cyber|software|robot|robots|科技|数码|人工智能|芯片|手机|机器人|网络安全"},
	{ContentSection{"world-business", "财经商业", "world"}, "economy|economic|economics|inflation|tariff|tariffs|stocks|stock market|trade|bank|banking|business|finance|financial|interest rates|gdp|财经|经济|通胀|关税|股市|金融|银行|商业|贸易"},
	{ContentSection{"world-science", "科学健康", "world"}, "science|scientist|scientists|space|nasa|health|medical|cancer|vaccine|disease|climate|科学|研究发现|太空|航天|健康|医疗|癌症|疫苗|疾病|气候"},
	{ContentSection{"world-sports", "体育赛事", "world"}, "sport|sports|football|basketball|tennis|olympics|world cup|fifa|nba|体育|足球|篮球|网球|奥运|世界杯"},
	{ContentSection{"world-culture", "文化娱乐", "world"}, "film|movie|music|singer|actor|actress|cinema|concert|oscar|culture|电影|音乐|歌手|演员|演唱会|文化|娱乐"},
	{ContentSection{"world-society", "社会民生", "world"}, "police|crime|court|arrest|arrested|school|education|flood|earthquake|rescue|accident|murder|社会|民生|警方|法院|逮捕|学校|教育|洪水|地震|救援|事故"},
	{ContentSection{"world-politics", "国际时政", "world"}, "election|president|minister|parliament|government|war|military|missile|diplomatic|ceasefire|选举|总统|总理|议会|政府|战争|军事|导弹|外交|停火"},
	{ContentSection{"world-general", "其他国际", "world"}, ""},
	{ContentSection{"community-esports", "游戏电竞", "community"}, "cs2|hltv|valorant|lol|lpl|lck|jdg|电竞|游戏|无畏契约|英雄联盟|三角洲|黑神话|原神|崩坏|瓦洛兰特|cn瓦"},
	{ContentSection{"community-sports", "体育运动", "community"}, "nba|cba|fifa|f1|体育|篮球|足球|网球|羽毛球|乒乓|欧冠|英超|西甲|中超|世界杯|奥运|湖人|勇士|詹姆斯|库里|梅西|c罗|皇马|巴萨"},
	{ContentSection{"community-tech", "科技数码", "community"}, "ai|openai|deepseek|iphone|android|科技|数码|手机|芯片|电脑|华为|小米|苹果|显卡|人工智能|鸿蒙"},
	{ContentSection{"community-culture", "影视娱乐", "community"}, "电影|影视|娱乐|电视剧|音乐|演唱会|歌手|综艺|动漫|动画|明星|神曲"},
	{ContentSection{"community-business", "财经汽车", "community"}, "财经|经济|股票|股市|基金|黄金|汽车|比亚迪|特斯拉|新能源|油价|车评"},
	{ContentSection{"community-society", "社会生活", "community"}, "社会|生活|高考|考研|学校|大学|教育|就业|工作|结婚|旅游|美食|跑操|校园|房价|房租"},
	{ContentSection{"community-general", "综合话题", "community"}, ""},
}

func NewsSections() []ContentSection      { return sectionsFor(false) }
func CommunitySections() []ContentSection { return sectionsFor(true) }

func sectionsFor(community bool) []ContentSection {
	sections := make([]ContentSection, 0)
	for _, rule := range contentRules {
		if (rule.Category == "community") == community {
			sections = append(sections, rule.ContentSection)
		}
	}
	return sections
}

func classifyContent(category, title, summary string) ContentSection {
	title, summary = strings.ToLower(title), strings.ToLower(summary)
	var selected, fallback ContentSection
	best := 0
	for _, rule := range contentRules {
		if rule.Category != category {
			continue
		}
		if rule.keywords == "" {
			fallback = rule.ContentSection
			continue
		}
		score := 0
		for _, keyword := range strings.Split(rule.keywords, "|") {
			if containsKeyword(title, keyword) {
				score += 4
			}
			if containsKeyword(summary, keyword) {
				score++
			}
		}
		if score > best {
			best, selected = score, rule.ContentSection
		}
	}
	if best == 0 {
		return fallback
	}
	return selected
}

func containsKeyword(text, keyword string) bool {
	for _, r := range keyword {
		if unicode.Is(unicode.Han, r) {
			return strings.Contains(text, keyword)
		}
	}
	// English tokens must be whole words: "AI" must not match "said".
	words := " " + strings.Join(strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsPunct(r) || unicode.IsSpace(r) || unicode.Is(unicode.Han, r)
	}), " ") + " "
	return strings.Contains(words, " "+keyword+" ")
}

type CommunityTopic struct {
	model.Trend
	Key          string `json:"key"`
	Section      string `json:"section"`
	SectionLabel string `json:"sectionLabel"`
}

func CategorizeCommunity(rows []model.Trend) []CommunityTopic {
	topics := make([]CommunityTopic, 0, len(rows))
	for _, row := range rows {
		section := classifyContent("community", row.Keyword, row.Summary)
		topics = append(topics, CommunityTopic{Trend: row, Key: model.CommunityKey(row.Source, row.SourceURL), Section: section.ID, SectionLabel: section.Label})
	}
	return topics
}
