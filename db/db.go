package db

import (
	"database/sql"
	"time"

	"github.com/user/yt-rss/models"
	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

func New(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	query := `
	CREATE TABLE IF NOT EXISTS channels (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		channel_id TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		url TEXT NOT NULL,
		last_fetched DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS videos (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		channel_id INTEGER NOT NULL REFERENCES channels(id),
		video_id TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT,
		thumbnail TEXT,
		url TEXT NOT NULL,
		published_at DATETIME NOT NULL,
		fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(channel_id, video_id)
	);

	CREATE INDEX IF NOT EXISTS idx_videos_channel_id ON videos(channel_id);
	CREATE INDEX IF NOT EXISTS idx_videos_published_at ON videos(published_at);
	`
	_, err := db.conn.Exec(query)
	return err
}

func (db *DB) AddChannel(channelID, name, url string) error {
	_, err := db.conn.Exec(
		"INSERT OR IGNORE INTO channels (channel_id, name, url) VALUES (?, ?, ?)",
		channelID, name, url,
	)
	return err
}

func (db *DB) RemoveChannel(id int64) error {
	result, err := db.conn.Exec("DELETE FROM channels WHERE id = ?", id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (db *DB) ListChannels() ([]models.Channel, error) {
	rows, err := db.conn.Query("SELECT id, channel_id, name, url, last_fetched, created_at FROM channels ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []models.Channel
	for rows.Next() {
		var c models.Channel
		var lastFetched sql.NullTime
		if err := rows.Scan(&c.ID, &c.ChannelID, &c.Name, &c.URL, &lastFetched, &c.CreatedAt); err != nil {
			return nil, err
		}
		if lastFetched.Valid {
			c.LastFetched = &lastFetched.Time
		}
		channels = append(channels, c)
	}
	return channels, rows.Err()
}

func (db *DB) GetChannel(id int64) (*models.Channel, error) {
	var c models.Channel
	var lastFetched sql.NullTime
	err := db.conn.QueryRow(
		"SELECT id, channel_id, name, url, last_fetched, created_at FROM channels WHERE id = ?",
		id,
	).Scan(&c.ID, &c.ChannelID, &c.Name, &c.URL, &lastFetched, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	if lastFetched.Valid {
		c.LastFetched = &lastFetched.Time
	}
	return &c, nil
}

func (db *DB) GetChannelByChannelID(channelID string) (*models.Channel, error) {
	var c models.Channel
	var lastFetched sql.NullTime
	err := db.conn.QueryRow(
		"SELECT id, channel_id, name, url, last_fetched, created_at FROM channels WHERE channel_id = ?",
		channelID,
	).Scan(&c.ID, &c.ChannelID, &c.Name, &c.URL, &lastFetched, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	if lastFetched.Valid {
		c.LastFetched = &lastFetched.Time
	}
	return &c, nil
}

func (db *DB) UpdateLastFetched(id int64, t time.Time) error {
	_, err := db.conn.Exec("UPDATE channels SET last_fetched = ? WHERE id = ?", t, id)
	return err
}

func (db *DB) UpsertVideo(v *models.Video) error {
	_, err := db.conn.Exec(`
		INSERT OR IGNORE INTO videos (channel_id, video_id, title, description, thumbnail, url, published_at, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, v.ChannelID, v.VideoID, v.Title, v.Description, v.Thumbnail, v.URL, v.PublishedAt, v.FetchedAt)
	return err
}

func (db *DB) ListVideos(channelID int64, limit int) ([]models.Video, error) {
	rows, err := db.conn.Query(
		"SELECT id, channel_id, video_id, title, description, thumbnail, url, published_at, fetched_at FROM videos WHERE channel_id = ? ORDER BY published_at DESC LIMIT ?",
		channelID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var videos []models.Video
	for rows.Next() {
		var v models.Video
		if err := rows.Scan(&v.ID, &v.ChannelID, &v.VideoID, &v.Title, &v.Description, &v.Thumbnail, &v.URL, &v.PublishedAt, &v.FetchedAt); err != nil {
			return nil, err
		}
		videos = append(videos, v)
	}
	return videos, rows.Err()
}

func (db *DB) ListAllVideos(limit int) ([]models.Video, error) {
	rows, err := db.conn.Query(
		"SELECT v.id, v.channel_id, v.video_id, v.title, v.description, v.thumbnail, v.url, v.published_at, v.fetched_at, c.name FROM videos v JOIN channels c ON v.channel_id = c.id ORDER BY v.published_at DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var videos []models.Video
	for rows.Next() {
		var v models.Video
		var channelName string
		if err := rows.Scan(&v.ID, &v.ChannelID, &v.VideoID, &v.Title, &v.Description, &v.Thumbnail, &v.URL, &v.PublishedAt, &v.FetchedAt, &channelName); err != nil {
			return nil, err
		}
		videos = append(videos, v)
	}
	return videos, rows.Err()
}
