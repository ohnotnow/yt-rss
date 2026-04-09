package web

import (
	"embed"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/user/yt-rss/db"
	"github.com/user/yt-rss/models"
	"github.com/user/yt-rss/youtube"
)

//go:embed static/*
var staticFS embed.FS

type Server struct {
	db *db.DB
}

func NewServer(database *db.DB) *Server {
	return &Server{db: database}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/videos" && r.Method == http.MethodGet:
		s.handleVideos(w, r)
	case r.URL.Path == "/api/categories" && r.Method == http.MethodGet:
		s.handleCategories(w, r)
	case r.URL.Path == "/api/categories" && r.Method == http.MethodPost:
		s.handleAddCategory(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/categories/") && r.Method == http.MethodDelete:
		s.handleDeleteCategory(w, r)
	case r.URL.Path == "/api/channels" && r.Method == http.MethodGet:
		s.handleListChannels(w, r)
	case r.URL.Path == "/api/channels" && r.Method == http.MethodPost:
		s.handleAddChannel(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/channels/") && r.Method == http.MethodDelete:
		s.handleDeleteChannel(w, r)
	case strings.HasSuffix(r.URL.Path, "/watch") && strings.HasPrefix(r.URL.Path, "/api/videos/") && r.Method == http.MethodPost:
		s.handleWatchVideo(w, r)
	case strings.HasSuffix(r.URL.Path, "/unwatch") && strings.HasPrefix(r.URL.Path, "/api/videos/") && r.Method == http.MethodPost:
		s.handleUnwatchVideo(w, r)
	case strings.HasSuffix(r.URL.Path, "/watch-before") && strings.HasPrefix(r.URL.Path, "/api/videos/") && r.Method == http.MethodPost:
		s.handleWatchBefore(w, r)
	case r.URL.Path == "/api/fetch" && r.Method == http.MethodPost:
		s.handleFetch(w, r)
	case r.URL.Path == "/" || r.URL.Path == "/index.html":
		s.serveIndex(w, r)
	default:
		http.FileServer(http.FS(staticFS)).ServeHTTP(w, r)
	}
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func (s *Server) handleVideos(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	query := r.URL.Query().Get("q")
	categoryParam := r.URL.Query().Get("category")

	var categoryID int64
	var hasCategory bool
	if categoryParam != "" {
		if parsed, err := strconv.ParseInt(categoryParam, 10, 64); err == nil {
			categoryID = parsed
			hasCategory = true
		}
	}

	var videos []models.Video
	var err error

	switch {
	case query != "" && hasCategory:
		videos, err = s.db.SearchVideosByCategory(query, categoryID, limit)
	case query != "":
		videos, err = s.db.SearchVideos(query, limit)
	case hasCategory:
		videos, err = s.db.ListVideosByCategory(categoryID, limit)
	default:
		videos, err = s.db.ListAllVideos(limit)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, videos)
}

func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := s.db.ListCategories()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, categories)
}

func (s *Server) handleAddCategory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	id, err := s.db.AddCategory(strings.TrimSpace(req.Name))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			http.Error(w, "category already exists", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]any{"id": id, "name": req.Name})
}

func (s *Server) handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/categories/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid category ID", http.StatusBadRequest)
		return
	}
	if err := s.db.RemoveCategory(id); err != nil {
		http.Error(w, "category not found", http.StatusNotFound)
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"})
}

func (s *Server) handleListChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := s.db.ListChannels()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, channels)
}

func (s *Server) handleAddChannel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL        string `json:"url"`
		CategoryID *int64 `json:"category_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}

	channelID, name, err := youtube.ParseChannelURL(strings.TrimSpace(req.URL))
	if err != nil {
		http.Error(w, "could not resolve channel: "+err.Error(), http.StatusBadRequest)
		return
	}

	if name == "" {
		feedURL := youtube.RSSFeedURL(channelID)
		feed, err := youtube.FetchFeed(feedURL)
		if err == nil {
			name = feed.ChannelTitle
		}
		if name == "" {
			name = channelID
		}
	}

	if err := s.db.AddChannel(channelID, name, req.URL, req.CategoryID); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			http.Error(w, "channel already exists", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ch, _ := s.db.GetChannelByChannelID(channelID)
	jsonResponse(w, ch)
}

func (s *Server) handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/channels/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid channel ID", http.StatusBadRequest)
		return
	}
	if err := s.db.RemoveChannel(id); err != nil {
		http.Error(w, "channel not found", http.StatusNotFound)
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"})
}

func (s *Server) handleFetch(w http.ResponseWriter, r *http.Request) {
	var channels []models.Channel
	var err error

	if idStr := r.URL.Query().Get("id"); idStr != "" {
		id, parseErr := strconv.ParseInt(idStr, 10, 64)
		if parseErr != nil {
			http.Error(w, "invalid channel ID", http.StatusBadRequest)
			return
		}
		ch, getErr := s.db.GetChannel(id)
		if getErr != nil {
			http.Error(w, "channel not found", http.StatusNotFound)
			return
		}
		channels = []models.Channel{*ch}
	} else {
		channels, err = s.db.ListChannels()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	fetcher := youtube.NewFetcher(s.db.UpsertVideo, s.db.UpdateLastFetched)
	results := fetcher.FetchChannels(channels)
	jsonResponse(w, results)
}

func (s *Server) extractVideoID(path, suffix string) (int64, error) {
	trimmed := strings.TrimPrefix(path, "/api/videos/")
	trimmed = strings.TrimSuffix(trimmed, "/"+suffix)
	return strconv.ParseInt(trimmed, 10, 64)
}

func (s *Server) handleWatchVideo(w http.ResponseWriter, r *http.Request) {
	id, err := s.extractVideoID(r.URL.Path, "watch")
	if err != nil {
		http.Error(w, "invalid video ID", http.StatusBadRequest)
		return
	}
	if err := s.db.MarkVideoWatched(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"})
}

func (s *Server) handleUnwatchVideo(w http.ResponseWriter, r *http.Request) {
	id, err := s.extractVideoID(r.URL.Path, "unwatch")
	if err != nil {
		http.Error(w, "invalid video ID", http.StatusBadRequest)
		return
	}
	if err := s.db.MarkVideoUnwatched(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"})
}

func (s *Server) handleWatchBefore(w http.ResponseWriter, r *http.Request) {
	id, err := s.extractVideoID(r.URL.Path, "watch-before")
	if err != nil {
		http.Error(w, "invalid video ID", http.StatusBadRequest)
		return
	}
	count, err := s.db.MarkVideosWatchedBefore(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]any{"status": "ok", "count": count})
}

func jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
