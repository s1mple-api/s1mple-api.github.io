package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const defaultTranslationURL = "https://uapis.cn/api/v1/translate/text?to_lang=zh"
const translationEndMarker = "LIFETV_END_SENTINEL_72"

var (
	bbCodePattern = regexp.MustCompile(`\[[^]]+]`)
	urlPattern    = regexp.MustCompile(`https?://\S+`)
)

type translator struct {
	client   *http.Client
	endpoint string
}

func newTranslator(endpoint string) *translator {
	if endpoint == "" {
		endpoint = defaultTranslationURL
	}
	return &translator{client: &http.Client{Timeout: 18 * time.Second}, endpoint: endpoint}
}

func (t *translator) translate(ctx context.Context, input string) (string, error) {
	usesUAPI := strings.Contains(t.endpoint, "/api/v1/translate/text")
	input = cleanForTranslation(input, usesUAPI)
	if input == "" || (!usesUAPI && hasChinese(input)) {
		return input, nil
	}
	if usesUAPI {
		return t.translateUAPI(ctx, input)
	}
	query := url.Values{
		"langpair": {"en|zh-CN"}, "q": {input},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer closeTranslationBody(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("translation returned %s", resp.Status)
	}
	var payload struct {
		ResponseData struct {
			TranslatedText string `json:"translatedText"`
		} `json:"responseData"`
		ResponseStatus  int    `json:"responseStatus"`
		ResponseDetails string `json:"responseDetails"`
		QuotaFinished   bool   `json:"quotaFinished"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", err
	}
	if payload.QuotaFinished || payload.ResponseStatus != http.StatusOK {
		return "", fmt.Errorf("translation quota/status: %d %s", payload.ResponseStatus, payload.ResponseDetails)
	}
	output := strings.TrimSpace(html.UnescapeString(payload.ResponseData.TranslatedText))
	if output == "" {
		return "", fmt.Errorf("translation response has no text")
	}
	return output, nil
}

func (t *translator) translateUAPI(ctx context.Context, input string) (string, error) {
	body, err := json.Marshal(map[string]string{"text": input})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer closeTranslationBody(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("translation returned %s", resp.Status)
	}
	var payload struct {
		Translate string `json:"translate"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", err
	}
	output := strings.TrimSpace(html.UnescapeString(payload.Translate))
	if output == "" {
		return "", fmt.Errorf("translation response has no text")
	}
	return output, nil
}

// translateLong preserves the whole article by sending bounded chunks through
// the existing translation provider rather than silently truncating a report.
func (t *translator) translateLong(ctx context.Context, input string) (string, error) {
	limit := 180
	usesUAPI := strings.Contains(t.endpoint, "/api/v1/translate/text")
	if usesUAPI {
		limit = 240
	}
	chunks := splitTranslationChunks(input, limit)
	translated := make([]string, 0, len(chunks))
	for index, chunk := range chunks {
		if strings.TrimSpace(chunk) == "" {
			translated = append(translated, chunk)
			continue
		}
		result, err := t.translateVerifiedChunk(ctx, chunk, usesUAPI)
		if err != nil {
			return "", err
		}
		translated = append(translated, result)
		if index < len(chunks)-1 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
	return strings.Join(translated, "\n\n"), nil
}

func (t *translator) translateVerifiedChunk(ctx context.Context, chunk string, verify bool) (string, error) {
	requestText := chunk
	if verify {
		requestText += "\n\n" + translationEndMarker
	}
	result, err := t.translate(ctx, requestText)
	if err != nil || !verify {
		return result, err
	}
	if markerAt := strings.Index(result, translationEndMarker); markerAt >= 0 {
		return strings.TrimSpace(result[:markerAt]), nil
	}
	if len([]rune(chunk)) <= 120 {
		return "", fmt.Errorf("translation omitted the end of article chunk")
	}
	parts := splitTranslationChunks(chunk, len([]rune(chunk))/2)
	translated := make([]string, 0, len(parts))
	for _, part := range parts {
		text, err := t.translateVerifiedChunk(ctx, part, true)
		if err != nil {
			return "", err
		}
		translated = append(translated, text)
	}
	return strings.Join(translated, "\n\n"), nil
}

func splitTranslationChunks(input string, limit int) []string {
	runes := []rune(input)
	if len(runes) <= limit {
		return []string{input}
	}
	var chunks []string
	for len(runes) > 0 {
		end := limit
		if end > len(runes) {
			end = len(runes)
		}
		if end < len(runes) {
			for split := end; split > limit*2/3; split-- {
				if runes[split] == ' ' || runes[split] == '\n' {
					end = split + 1
					break
				}
			}
		}
		chunks = append(chunks, string(runes[:end]))
		runes = runes[end:]
	}
	return chunks
}

func cleanForTranslation(input string, preserveParagraphs bool) string {
	input = html.UnescapeString(input)
	input = bbCodePattern.ReplaceAllString(input, " ")
	input = urlPattern.ReplaceAllString(input, " ")
	input = strings.ReplaceAll(input, "\\", " ")
	if preserveParagraphs {
		input = strings.TrimSpace(strings.ReplaceAll(input, "\r\n", "\n"))
	} else {
		input = strings.Join(strings.Fields(input), " ")
	}
	runes := []rune(input)
	limit := 180
	if preserveParagraphs {
		limit = 3000
	}
	if len(runes) > limit {
		input = string(runes[:limit])
	}
	return input
}

func hasChinese(input string) bool {
	for _, char := range input {
		if unicode.Is(unicode.Han, char) {
			return true
		}
	}
	return false
}

func closeTranslationBody(body io.ReadCloser) {
	if err := body.Close(); err != nil {
		log.Printf("close translation response body: %v", err)
	}
}
