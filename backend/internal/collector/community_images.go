package collector

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func CommunityImageURL(raw string) string {
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.User != nil || u.Port() != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	allowed := false
	for _, root := range []string{"xhscdn.com", "hoopchina.com.cn", "hdslb.com", "bdimg.com", "bdstatic.com"} {
		if host == root || strings.HasSuffix(host, "."+root) {
			allowed = true
		}
	}
	for _, exact := range []string{"tiebapic.baidu.com", "imgsa.baidu.com", "imgsrc.baidu.com", "hiphotos.baidu.com"} {
		if host == exact {
			allowed = true
		}
	}
	if !allowed {
		return ""
	}
	u.Scheme = "https"
	u.Fragment = ""
	return u.String()
}

func FetchCommunityImage(raw string) ([]byte, string, error) {
	imageURL := CommunityImageURL(raw)
	if imageURL == "" {
		return nil, "", fmt.Errorf("unsupported image URL")
	}
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || CommunityImageURL(req.URL.String()) == "" {
			return fmt.Errorf("unsupported image redirect")
		}
		return nil
	}}
	req, err := http.NewRequest(http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	host := req.URL.Hostname()
	referer := "https://tieba.baidu.com/"
	if strings.HasSuffix(host, ".xhscdn.com") || host == "xhscdn.com" {
		referer = "https://www.xiaohongshu.com/"
	}
	if strings.HasSuffix(host, ".hoopchina.com.cn") {
		referer = "https://bbs.hupu.com/"
	}
	req.Header.Set("Referer", referer)
	response, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("image request unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, "", fmt.Errorf("image source returned %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil || len(body) > 8<<20 {
		return nil, "", fmt.Errorf("image response invalid")
	}
	contentType := http.DetectContentType(body)
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("source did not return an image")
	}
	return body, contentType, nil
}
