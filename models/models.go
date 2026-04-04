package models

import "time"

type Category struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Channel struct {
	ID           int64      `json:"id"`
	ChannelID    string     `json:"channel_id"`
	Name         string     `json:"name"`
	URL          string     `json:"url"`
	CategoryID   *int64     `json:"category_id"`
	CategoryName string     `json:"category_name,omitempty"`
	LastFetched  *time.Time `json:"last_fetched"`
	CreatedAt    time.Time  `json:"created_at"`
}

type Video struct {
	ID           int64     `json:"id"`
	ChannelID    int64     `json:"channel_id"`
	VideoID      string    `json:"video_id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Thumbnail    string    `json:"thumbnail"`
	URL          string    `json:"url"`
	PublishedAt  time.Time `json:"published_at"`
	FetchedAt    time.Time `json:"fetched_at"`
	ChannelName  string    `json:"channel_name"`
	CategoryName string    `json:"category_name,omitempty"`
}
