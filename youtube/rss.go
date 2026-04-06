package youtube

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"
)

// IsShort checks whether a video is a YouTube Short by probing the shorts URL.
// YouTube returns 200 for actual Shorts and redirects to /watch for regular videos.
func IsShort(videoID string) bool {
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequest("HEAD", fmt.Sprintf("https://www.youtube.com/shorts/%s", videoID), nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; yt-rss/1.0)")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

type Feed struct {
	XMLName      xml.Name `xml:"feed"`
	ChannelID    string   `xml:"http://www.youtube.com/xml/schemas/2015 channelId"`
	ChannelTitle string   `xml:"title"`
	Entries      []Entry  `xml:"entry"`
}

type Entry struct {
	VideoID   string     `xml:"http://www.youtube.com/xml/schemas/2015 videoId"`
	Title     string     `xml:"title"`
	Link      Link       `xml:"link"`
	Author    Author     `xml:"author"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
	Group     MediaGroup `xml:"http://search.yahoo.com/mrss/ group"`
}

type Link struct {
	Href string `xml:"href,attr"`
}

type Author struct {
	Name string `xml:"name"`
}

type MediaGroup struct {
	Title       string         `xml:"title"`
	Description string         `xml:"description"`
	Thumbnail   MediaThumbnail `xml:"thumbnail"`
}

type MediaThumbnail struct {
	URL string `xml:"url,attr"`
}

func FetchFeed(feedURL string) (*Feed, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(feedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("feed returned status %d: %s", resp.StatusCode, string(body))
	}

	var feed Feed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("failed to parse feed: %w", err)
	}

	return &feed, nil
}

func ParseEntries(feed *Feed, channelDBID int64) []VideoEntry {
	entries := make([]VideoEntry, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		published, _ := time.Parse(time.RFC3339, e.Published)
		thumbnail := e.Group.Thumbnail.URL
		if thumbnail == "" {
			thumbnail = ThumbnailURL(e.VideoID)
		}
		description := e.Group.Description
		if description == "" {
			description = e.Group.Title
		}
		entries = append(entries, VideoEntry{
			ChannelID:   channelDBID,
			VideoID:     e.VideoID,
			Title:       e.Title,
			Description: description,
			Thumbnail:   thumbnail,
			URL:         e.Link.Href,
			PublishedAt: published,
		})
	}
	return entries
}

type VideoEntry struct {
	ChannelID   int64
	VideoID     string
	Title       string
	Description string
	Thumbnail   string
	URL         string
	PublishedAt time.Time
}
