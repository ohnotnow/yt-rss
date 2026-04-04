package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/user/yt-rss/db"
	"github.com/user/yt-rss/models"
	"github.com/user/yt-rss/youtube"
)

func dbPath() string {
	dir, err := os.UserHomeDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, ".yt_rss.db")
}

func openDB() (*db.DB, error) {
	return db.New(dbPath())
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "add":
		cmdAdd()
	case "remove", "rm":
		cmdRemove()
	case "list", "ls":
		cmdList()
	case "fetch":
		cmdFetch()
	case "videos", "vids":
		cmdVideos()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`yt-rss - Track YouTube channels via RSS

Usage:
  yt-rss <command> [arguments]

Commands:
  add <url>          Add a YouTube channel (accepts any YouTube channel URL)
  remove <id>        Remove a channel by ID
  list               List all tracked channels
  fetch [id]         Fetch RSS feeds (all channels, or specific channel by ID)
  videos [id] [n]    Show recent videos (all channels, or specific channel, limit n)
  help               Show this help message`)
}

func cmdAdd() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: yt-rss add <channel-url>")
		os.Exit(1)
	}

	rawURL := os.Args[2]
	channelID, name, err := youtube.ParseChannelURL(rawURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	database, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	if name == "" {
		feedURL := youtube.RSSFeedURL(channelID)
		feed, err := youtube.FetchFeed(feedURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching channel info: %v\n", err)
			os.Exit(1)
		}
		name = feed.ChannelTitle
	}

	if err := database.AddChannel(channelID, name, rawURL); err != nil {
		fmt.Fprintf(os.Stderr, "Error adding channel: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Added channel: %s (%s)\n", name, channelID)
}

func cmdRemove() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: yt-rss remove <channel-id>")
		os.Exit(1)
	}

	id, err := strconv.ParseInt(os.Args[2], 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid channel ID: %s\n", os.Args[2])
		os.Exit(1)
	}

	database, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	channel, err := database.GetChannel(id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: channel not found: %d\n", id)
		os.Exit(1)
	}

	if err := database.RemoveChannel(id); err != nil {
		fmt.Fprintf(os.Stderr, "Error removing channel: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Removed channel: %s\n", channel.Name)
}

func cmdList() {
	database, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	channels, err := database.ListChannels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing channels: %v\n", err)
		os.Exit(1)
	}

	if len(channels) == 0 {
		fmt.Println("No channels tracked yet. Use 'yt-rss add <url>' to add one.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tCHANNEL\tCHANNEL ID\tURL\tLAST FETCHED\t")
	for _, c := range channels {
		lastFetched := "never"
		if c.LastFetched != nil {
			lastFetched = c.LastFetched.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t\n", c.ID, c.Name, c.ChannelID, c.URL, lastFetched)
	}
	w.Flush()
}

func cmdFetch() {
	database, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	var channels []models.Channel
	if len(os.Args) >= 3 {
		id, err := strconv.ParseInt(os.Args[2], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid channel ID: %s\n", os.Args[2])
			os.Exit(1)
		}
		c, err := database.GetChannel(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: channel not found: %d\n", id)
			os.Exit(1)
		}
		channels = append(channels, *c)
	} else {
		channels, err = database.ListChannels()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing channels: %v\n", err)
			os.Exit(1)
		}
	}

	if len(channels) == 0 {
		fmt.Println("No channels to fetch. Use 'yt-rss add <url>' to add one.")
		return
	}

	totalNew := 0
	for _, c := range channels {
		feedURL := youtube.RSSFeedURL(c.ChannelID)
		feed, err := youtube.FetchFeed(feedURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching %s: %v\n", c.Name, err)
			continue
		}

		entries := youtube.ParseEntries(feed, c.ID)
		newCount := 0
		for _, e := range entries {
			if err := database.UpsertVideo(&models.Video{
				ChannelID:   e.ChannelID,
				VideoID:     e.VideoID,
				Title:       e.Title,
				Description: e.Description,
				Thumbnail:   e.Thumbnail,
				URL:         e.URL,
				PublishedAt: e.PublishedAt,
				FetchedAt:   time.Now(),
			}); err != nil {
				fmt.Fprintf(os.Stderr, "  Error saving video: %v\n", err)
				continue
			}
			newCount++
		}

		if err := database.UpdateLastFetched(c.ID, time.Now()); err != nil {
			fmt.Fprintf(os.Stderr, "  Error updating last fetched: %v\n", err)
		}

		fmt.Printf("Fetched %s: %d videos\n", c.Name, newCount)
		totalNew += newCount
	}

	fmt.Printf("\nTotal: %d videos saved\n", totalNew)
}

func cmdVideos() {
	database, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	limit := 20
	if len(os.Args) >= 4 {
		limit, err = strconv.Atoi(os.Args[3])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid limit: %s\n", os.Args[3])
			os.Exit(1)
		}
	}

	var videos []models.Video
	var channelName string

	if len(os.Args) >= 3 {
		id, err := strconv.ParseInt(os.Args[2], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid channel ID: %s\n", os.Args[2])
			os.Exit(1)
		}
		channel, err := database.GetChannel(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: channel not found: %d\n", id)
			os.Exit(1)
		}
		channelName = channel.Name
		videos, err = database.ListVideos(id, limit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing videos: %v\n", err)
			os.Exit(1)
		}
	} else {
		videos, err = database.ListAllVideos(limit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing videos: %v\n", err)
			os.Exit(1)
		}
	}

	if len(videos) == 0 {
		if channelName != "" {
			fmt.Printf("No videos found for %s. Run 'yt-rss fetch %s' first.\n", channelName, os.Args[2])
		} else {
			fmt.Println("No videos found. Run 'yt-rss fetch' first.")
		}
		return
	}

	if channelName != "" {
		fmt.Printf("Recent videos for %s:\n\n", channelName)
	} else {
		fmt.Println("Recent videos:\n")
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PUBLISHED\tTITLE\tURL\t")
	for _, v := range videos {
		fmt.Fprintf(w, "%s\t%s\t%s\t\n",
			v.PublishedAt.Format("2006-01-02"),
			v.Title,
			v.URL,
		)
	}
	w.Flush()
}
