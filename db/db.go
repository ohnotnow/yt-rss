package db

import (
	"database/sql"
	"strings"
	"time"

	"github.com/user/yt-rss/models"
	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

func New(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}

	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

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
	CREATE TABLE IF NOT EXISTS categories (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE
	);

	CREATE TABLE IF NOT EXISTS channels (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		channel_id TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		url TEXT NOT NULL,
		category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
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
	if _, err := db.conn.Exec(query); err != nil {
		return err
	}

	// Add category_id column if upgrading from older schema
	db.conn.Exec("ALTER TABLE channels ADD COLUMN category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL")

	return nil
}

// --- Categories ---

func (db *DB) AddCategory(name string) (int64, error) {
	result, err := db.conn.Exec("INSERT INTO categories (name) VALUES (?)", name)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (db *DB) RemoveCategory(id int64) error {
	result, err := db.conn.Exec("DELETE FROM categories WHERE id = ?", id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (db *DB) ListCategories() ([]models.Category, error) {
	rows, err := db.conn.Query("SELECT id, name FROM categories ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []models.Category
	for rows.Next() {
		var c models.Category
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			return nil, err
		}
		categories = append(categories, c)
	}
	return categories, rows.Err()
}

func (db *DB) GetCategoryByName(name string) (*models.Category, error) {
	var c models.Category
	err := db.conn.QueryRow("SELECT id, name FROM categories WHERE name = ?", name).Scan(&c.ID, &c.Name)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// --- Channels ---

func (db *DB) AddChannel(channelID, name, url string, categoryID *int64) error {
	_, err := db.conn.Exec(
		"INSERT OR IGNORE INTO channels (channel_id, name, url, category_id) VALUES (?, ?, ?, ?)",
		channelID, name, url, categoryID,
	)
	return err
}

func (db *DB) UpdateChannelCategory(id int64, categoryID *int64) error {
	_, err := db.conn.Exec("UPDATE channels SET category_id = ? WHERE id = ?", categoryID, id)
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
	rows, err := db.conn.Query(`
		SELECT c.id, c.channel_id, c.name, c.url, c.category_id, COALESCE(cat.name, ''), c.last_fetched, c.created_at
		FROM channels c
		LEFT JOIN categories cat ON c.category_id = cat.id
		ORDER BY c.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanChannels(rows)
}

func (db *DB) GetChannel(id int64) (*models.Channel, error) {
	var c models.Channel
	var lastFetched sql.NullTime
	var categoryID sql.NullInt64
	err := db.conn.QueryRow(`
		SELECT c.id, c.channel_id, c.name, c.url, c.category_id, COALESCE(cat.name, ''), c.last_fetched, c.created_at
		FROM channels c
		LEFT JOIN categories cat ON c.category_id = cat.id
		WHERE c.id = ?
	`, id).Scan(&c.ID, &c.ChannelID, &c.Name, &c.URL, &categoryID, &c.CategoryName, &lastFetched, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	if lastFetched.Valid {
		c.LastFetched = &lastFetched.Time
	}
	if categoryID.Valid {
		c.CategoryID = &categoryID.Int64
	}
	return &c, nil
}

func (db *DB) GetChannelByChannelID(channelID string) (*models.Channel, error) {
	var c models.Channel
	var lastFetched sql.NullTime
	var categoryID sql.NullInt64
	err := db.conn.QueryRow(`
		SELECT c.id, c.channel_id, c.name, c.url, c.category_id, COALESCE(cat.name, ''), c.last_fetched, c.created_at
		FROM channels c
		LEFT JOIN categories cat ON c.category_id = cat.id
		WHERE c.channel_id = ?
	`, channelID).Scan(&c.ID, &c.ChannelID, &c.Name, &c.URL, &categoryID, &c.CategoryName, &lastFetched, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	if lastFetched.Valid {
		c.LastFetched = &lastFetched.Time
	}
	if categoryID.Valid {
		c.CategoryID = &categoryID.Int64
	}
	return &c, nil
}

func (db *DB) UpdateLastFetched(id int64, t time.Time) error {
	_, err := db.conn.Exec("UPDATE channels SET last_fetched = ? WHERE id = ?", t, id)
	return err
}

// --- Videos ---

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
	return db.queryVideos(`
		SELECT v.id, v.channel_id, v.video_id, v.title, v.description, v.thumbnail, v.url, v.published_at, v.fetched_at, c.name, COALESCE(cat.name, '')
		FROM videos v
		JOIN channels c ON v.channel_id = c.id
		LEFT JOIN categories cat ON c.category_id = cat.id
		ORDER BY v.published_at DESC LIMIT ?
	`, limit)
}

func (db *DB) ListVideosByCategory(categoryID int64, limit int) ([]models.Video, error) {
	return db.queryVideos(`
		SELECT v.id, v.channel_id, v.video_id, v.title, v.description, v.thumbnail, v.url, v.published_at, v.fetched_at, c.name, COALESCE(cat.name, '')
		FROM videos v
		JOIN channels c ON v.channel_id = c.id
		LEFT JOIN categories cat ON c.category_id = cat.id
		WHERE c.category_id = ?
		ORDER BY v.published_at DESC LIMIT ?
	`, categoryID, limit)
}

func (db *DB) SearchVideos(query string, limit int) ([]models.Video, error) {
	pattern := "%" + query + "%"
	return db.queryVideos(`
		SELECT v.id, v.channel_id, v.video_id, v.title, v.description, v.thumbnail, v.url, v.published_at, v.fetched_at, c.name, COALESCE(cat.name, '')
		FROM videos v
		JOIN channels c ON v.channel_id = c.id
		LEFT JOIN categories cat ON c.category_id = cat.id
		WHERE v.title LIKE ? OR c.name LIKE ? OR v.description LIKE ?
		ORDER BY v.published_at DESC LIMIT ?
	`, pattern, pattern, pattern, limit)
}

func (db *DB) SearchVideosByCategory(query string, categoryID int64, limit int) ([]models.Video, error) {
	pattern := "%" + query + "%"
	return db.queryVideos(`
		SELECT v.id, v.channel_id, v.video_id, v.title, v.description, v.thumbnail, v.url, v.published_at, v.fetched_at, c.name, COALESCE(cat.name, '')
		FROM videos v
		JOIN channels c ON v.channel_id = c.id
		LEFT JOIN categories cat ON c.category_id = cat.id
		WHERE c.category_id = ? AND (v.title LIKE ? OR c.name LIKE ? OR v.description LIKE ?)
		ORDER BY v.published_at DESC LIMIT ?
	`, categoryID, pattern, pattern, pattern, limit)
}

// AllVideoIDs returns every video_id stored in the database.
func (db *DB) AllVideoIDs() ([]string, error) {
	rows, err := db.conn.Query("SELECT video_id FROM videos")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeleteVideosByIDs deletes videos matching the given video_ids.
func (db *DB) DeleteVideosByIDs(videoIDs []string) (int64, error) {
	if len(videoIDs) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(videoIDs))
	args := make([]any, len(videoIDs))
	for i, id := range videoIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := "DELETE FROM videos WHERE video_id IN (" + strings.Join(placeholders, ",") + ")"
	result, err := db.conn.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// --- Helpers ---

func (db *DB) queryVideos(query string, args ...any) ([]models.Video, error) {
	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var videos []models.Video
	for rows.Next() {
		var v models.Video
		if err := rows.Scan(&v.ID, &v.ChannelID, &v.VideoID, &v.Title, &v.Description, &v.Thumbnail, &v.URL, &v.PublishedAt, &v.FetchedAt, &v.ChannelName, &v.CategoryName); err != nil {
			return nil, err
		}
		videos = append(videos, v)
	}
	return videos, rows.Err()
}

func scanChannels(rows *sql.Rows) ([]models.Channel, error) {
	var channels []models.Channel
	for rows.Next() {
		var c models.Channel
		var lastFetched sql.NullTime
		var categoryID sql.NullInt64
		if err := rows.Scan(&c.ID, &c.ChannelID, &c.Name, &c.URL, &categoryID, &c.CategoryName, &lastFetched, &c.CreatedAt); err != nil {
			return nil, err
		}
		if lastFetched.Valid {
			c.LastFetched = &lastFetched.Time
		}
		if categoryID.Valid {
			c.CategoryID = &categoryID.Int64
		}
		channels = append(channels, c)
	}
	return channels, rows.Err()
}
