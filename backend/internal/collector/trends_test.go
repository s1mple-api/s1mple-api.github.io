package collector

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestParseHeatUnits(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int64
	}{
		{"212.2W讨论", 2_122_000}, {"163.2万", 1_632_000}, {"1.25亿热度", 125_000_000},
		{"17,417,908", 17_417_908}, {"热度 2.5k", 2500}, {"—", 0}, {"12345", 12345},
	} {
		t.Run(tc.value, func(t *testing.T) {
			if got := parseHeat(tc.value); got != tc.want {
				t.Errorf("parseHeat(%q) = %d, want %d", tc.value, got, tc.want)
			}
		})
	}
}

func TestDecodeCommunityHotboard(t *testing.T) {
	now := time.Date(2026, 9, 28, 6, 0, 0, 0, time.UTC)
	body := `{"type":"tieba","update_time":"2026-09-28T05:55:00Z","list":[
		{"index":2,"title":"手机新品","url":"https://tieba.baidu.com/p/2","hot_value":"12.5万","extra":{"desc":"  新品  讨论摘要  "}},
		{"index":1,"title":"篮球赛","url":"https://tieba.baidu.com/p/1","hot_value":"212.2W讨论","extra":null},
		{"index":3,"title":"重复链接","url":"https://tieba.baidu.com/p/1","hot_value":"1"},
		{"index":4,"title":"外站","url":"https://tieba.baidu.com.evil.example/p/1"},
		{"index":5,"title":"非安全链接","url":"javascript:alert(1)"},
		{"index":6,"title":"登录信息","url":"https://user:password@tieba.baidu.com/p/6"},
		{"index":7,"title":"异常端口","url":"https://tieba.baidu.com:1234/p/7"}
	]}`
	rows, err := decodeHotboard(strings.NewReader(body), "tieba", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Rank != 1 || rows[0].Heat != 2_122_000 {
		t.Fatalf("unexpected filtered/sorted rows: %+v", rows)
	}
	if rows[1].Summary != "新品 讨论摘要" || rows[0].HeatLabel != "212.2W讨论" || rows[0].Source != "tieba" || rows[0].Provider != "uapis.cn" {
		t.Fatalf("missing metadata: %+v", rows)
	}
	if !rows[0].FetchedAt.Equal(now.Add(-5 * time.Minute)) {
		t.Fatal("must retain upstream timestamp, not fetch time")
	}
}

func TestHotboardRejectsStaleFutureAndWrongSource(t *testing.T) {
	now := time.Date(2026, 9, 28, 6, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ name, source, updated, list string }{
		{"stale", "hupu", "2026-09-28T03:00:00Z", `[{"title":"体育","url":"https://bbs.hupu.com/1"}]`},
		{"future", "hupu", "2026-09-28T07:00:00Z", `[{"title":"体育","url":"https://bbs.hupu.com/1"}]`},
		{"bad-date", "hupu", "unknown", `[]`},
		{"wrong-source", "tieba", "2026-09-28T06:00:00Z", `[]`},
		{"empty", "hupu", "2026-09-28T06:00:00Z", `[]`},
		{"wrong-host", "hupu", "2026-09-28T06:00:00Z", `[{"title":"体育","url":"https://tieba.baidu.com/p/1"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"type":%q,"update_time":%q,"list":%s}`, tc.source, tc.updated, tc.list)
			if _, err := decodeHotboard(strings.NewReader(body), "hupu", now); err == nil {
				t.Fatal("invalid payload was accepted")
			}
		})
	}
}

func TestHotboardSupportsHupuAndPreservesWeibo(t *testing.T) {
	now := time.Date(2026, 9, 28, 6, 0, 0, 0, time.UTC)
	for _, source := range []string{"hupu", "weibo"} {
		host := "bbs.hupu.com"
		if source == "weibo" {
			host = "s.weibo.com"
		}
		body := fmt.Sprintf(`{"type":%q,"update_time":"2026-09-28T06:00:00Z","list":[{"title":"热门话题","url":"https://%s/search?q=test","hot_value":"17417908","extra":"optional metadata"}]}`, source, host)
		rows, err := decodeHotboard(strings.NewReader(body), source, now)
		if err != nil || len(rows) != 1 || rows[0].Rank != 1 || rows[0].Heat != 17417908 {
			t.Fatalf("%s: rows=%+v, error=%v", source, rows, err)
		}
	}
	if _, err := NewTrendCollector().FetchCommunity("unknown"); err == nil {
		t.Fatal("unknown source accepted")
	}
	if _, err := decodeHotboard(strings.NewReader(`{`), "tieba", now); err == nil {
		t.Fatal("malformed JSON accepted")
	}
}
