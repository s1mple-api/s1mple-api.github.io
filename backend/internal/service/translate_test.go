package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranslateReadsChineseResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("langpair"); got != "en|zh-CN" {
			t.Errorf("langpair = %q", got)
		}
		if got := r.URL.Query().Get("q"); got != "Counter-Strike update" {
			t.Errorf("q = %q", got)
		}
		_, _ = w.Write([]byte(`{"responseData":{"translatedText":"CS2 更新"},"responseStatus":200}`))
	}))
	defer server.Close()

	got, err := newTranslator(server.URL).translate(context.Background(), "Counter-Strike update")
	if err != nil || got != "CS2 更新" {
		t.Fatalf("translate = %q, %v", got, err)
	}
}

func TestTranslateUAPIPreservesParagraphs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Query().Get("to_lang") != "zh" {
			t.Errorf("request = %s %s", r.Method, r.URL.String())
		}
		var payload struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(payload.Text, "\n\n") {
			t.Errorf("paragraph break lost: %q", payload.Text)
		}
		_, _ = w.Write([]byte(`{"translate":"第一段。\n\n第二段。"}`))
	}))
	defer server.Close()

	got, err := newTranslator(server.URL+"/api/v1/translate/text?to_lang=zh").translate(context.Background(), "First paragraph.\n\nSecond paragraph.")
	if err != nil || got != "第一段。\n\n第二段。" {
		t.Fatalf("translate = %q, %v", got, err)
	}
}

func TestTranslateLongRejectsMissingEndMarker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"translate":"译文前半段"}`))
	}))
	defer server.Close()

	_, err := newTranslator(server.URL+"/api/v1/translate/text?to_lang=zh").translateLong(context.Background(), "A complete news paragraph.")
	if err == nil || !strings.Contains(err.Error(), "omitted the end") {
		t.Fatalf("expected incomplete translation error, got %v", err)
	}
}

func TestCleanForTranslationRemovesBBCode(t *testing.T) {
	if got := cleanForTranslation("[b]Match result[/b]", false); got != "Match result" {
		t.Fatalf("cleanForTranslation returned %q", got)
	}
	if got := plainArticleBody("[p]Match result[/p]"); got != "Match result" {
		t.Fatalf("plainArticleBody returned %q", got)
	}
}
