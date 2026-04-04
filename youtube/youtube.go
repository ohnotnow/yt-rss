package youtube

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	channelIDRe = regexp.MustCompile(`UC[\w-]{22}`)
)

func ParseChannelURL(rawURL string) (channelID string, name string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid URL: %w", err)
	}

	path := strings.TrimPrefix(u.Path, "/")

	if strings.HasPrefix(path, "channel/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			cid := parts[1]
			if channelIDRe.MatchString(cid) {
				return cid, "", nil
			}
		}
	}

	if strings.HasPrefix(path, "@") {
		handle := strings.Split(path, "/")[0]
		return resolveHandle(handle)
	}

	if strings.HasPrefix(path, "c/") || strings.HasPrefix(path, "user/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			return resolveLegacyUser(parts[1])
		}
	}

	return "", "", fmt.Errorf("could not extract channel info from URL: %s", rawURL)
}

func resolveHandle(handle string) (string, string, error) {
	pageURL := fmt.Sprintf("https://www.youtube.com/%s", handle)
	return resolveFromPage(pageURL)
}

func resolveLegacyUser(username string) (string, string, error) {
	pageURL := fmt.Sprintf("https://www.youtube.com/user/%s", username)
	return resolveFromPage(pageURL)
}

func resolveFromPage(pageURL string) (string, string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(pageURL)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch channel page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("channel page returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", "", fmt.Errorf("failed to read page: %w", err)
	}

	html := string(body)
	match := channelIDRe.FindString(html)
	if match == "" {
		return "", "", fmt.Errorf("could not find channel ID on page")
	}

	name := extractChannelTitle(html)
	return match, name, nil
}

func extractChannelTitle(html string) string {
	idx := strings.Index(html, `<title>`)
	if idx == -1 {
		return ""
	}
	idx += len("<title>")
	end := strings.Index(html[idx:], "</title>")
	if end == -1 {
		return ""
	}
	title := html[idx : idx+end]
	title = strings.TrimSpace(title)
	title = strings.TrimSuffix(title, " - YouTube")
	return title
}

func RSSFeedURL(channelID string) string {
	return fmt.Sprintf("https://www.youtube.com/feeds/videos.xml?channel_id=%s", channelID)
}

func VideoURL(videoID string) string {
	return fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)
}

func ThumbnailURL(videoID string) string {
	return fmt.Sprintf("https://i.ytimg.com/vi/%s/hqdefault.jpg", videoID)
}
