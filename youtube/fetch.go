package youtube

import (
	"time"

	"github.com/user/yt-rss/models"
)

// FetchResult holds the outcome of fetching a single channel.
type FetchResult struct {
	ChannelName string `json:"channel_name"`
	VideoCount  int    `json:"video_count"`
	Error       string `json:"error,omitempty"`
}

// Fetcher provides methods to fetch RSS feeds and store videos.
type Fetcher struct {
	upsertVideo     func(v *models.Video) error
	updateLastFetch func(id int64, t time.Time) error
}

// NewFetcher creates a Fetcher with the given DB callbacks.
func NewFetcher(upsertVideo func(v *models.Video) error, updateLastFetch func(id int64, t time.Time) error) *Fetcher {
	return &Fetcher{
		upsertVideo:     upsertVideo,
		updateLastFetch: updateLastFetch,
	}
}

// FetchChannel fetches the RSS feed for a single channel and stores the videos.
func (f *Fetcher) FetchChannel(ch models.Channel) FetchResult {
	feedURL := RSSFeedURL(ch.ChannelID)
	feed, err := FetchFeed(feedURL)
	if err != nil {
		return FetchResult{ChannelName: ch.Name, Error: err.Error()}
	}

	entries := ParseEntries(feed, ch.ID)
	count := 0
	for _, e := range entries {
		if err := f.upsertVideo(&models.Video{
			ChannelID:   e.ChannelID,
			VideoID:     e.VideoID,
			Title:       e.Title,
			Description: e.Description,
			Thumbnail:   e.Thumbnail,
			URL:         e.URL,
			PublishedAt: e.PublishedAt,
			FetchedAt:   time.Now(),
		}); err != nil {
			continue
		}
		count++
	}

	f.updateLastFetch(ch.ID, time.Now())

	return FetchResult{ChannelName: ch.Name, VideoCount: count}
}

// FetchChannels fetches feeds for multiple channels.
func (f *Fetcher) FetchChannels(channels []models.Channel) []FetchResult {
	results := make([]FetchResult, 0, len(channels))
	for _, ch := range channels {
		results = append(results, f.FetchChannel(ch))
	}
	return results
}
