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
	if r.URL.Path == "/api/videos" {
		s.handleVideos(w, r)
		return
	}

	if r.URL.Path == "/" || r.URL.Path == "/index.html" {
		s.serveIndex(w, r)
		return
	}

	http.FileServer(http.FS(staticFS)).ServeHTTP(w, r)
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

	var videos []models.Video
	var err error
	if query != "" {
		videos, err = s.db.SearchVideos(query, limit)
	} else {
		videos, err = s.db.ListAllVideos(limit)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(videos)
}
