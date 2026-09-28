package collector

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCommunityImageURLRejectsUnsafeTargets(t *testing.T) {
	for _, raw := range []string{"https://localhost/image", "http://127.0.0.1/a", "https://xhscdn.com.evil.example/a", "https://user:pass@xhscdn.com/a", "https://xhscdn.com:8080/a", "file:///tmp/image", "javascript:alert(1)"} {
		if CommunityImageURL(raw) != "" {
			t.Errorf("unsafe image target accepted: %s", raw)
		}
	}
	if CommunityImageURL("//sns-webpic.xhscdn.com/a") != "https://sns-webpic.xhscdn.com/a" {
		t.Fatal("protocol-relative image was not normalized")
	}
	blocks := contentBlocks(`<p>正文</p><img src="data:image/gif;base64,a" data-original="//tiebapic.baidu.com/a.jpg">`)
	if len(blocks) != 2 || blocks[1].URL != "https://tiebapic.baidu.com/a.jpg" {
		t.Fatalf("lazy image not extracted: %+v", blocks)
	}
}

func TestCommunityImageFetchChecksMimeAndSendsNoCookies(t *testing.T) {
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	http.DefaultTransport = communityTransport(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Cookie") != "" || req.Header.Get("Referer") != "https://www.xiaohongshu.com/" {
			t.Fatal("image request must have source referer, but no session cookies")
		}
		body := io.NopCloser(bytes.NewReader(pngBytes.Bytes()))
		if req.URL.Path == "/html" {
			body = io.NopCloser(strings.NewReader("<html>login page</html>"))
		}
		return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
	})
	data, mime, err := FetchCommunityImage("https://sns-webpic.xhscdn.com/image")
	if err != nil || len(data) == 0 || mime != "image/png" {
		t.Fatalf("valid image rejected: %s %v", mime, err)
	}
	if _, _, err := FetchCommunityImage("https://sns-webpic.xhscdn.com/html"); err == nil {
		t.Fatal("HTML masquerading as an image accepted")
	}
}
