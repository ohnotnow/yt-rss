package web

import (
	"embed"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/user/yt-rss/db"
	"github.com/user/yt-rss/models"
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
	switch r.URL.Path {
	case "/api/videos":
		s.handleVideos(w, r)
	case "/api/categories":
		s.handleCategories(w, r)
	case "/", "/index.html":
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
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(videos)
}

func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	categories, err := s.db.ListCategories()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(categories)
}
