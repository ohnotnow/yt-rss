package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/user/yt-rss/db"
	"github.com/user/yt-rss/models"
	"github.com/user/yt-rss/web"
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
	case "edit":
		cmdEdit()
	case "list", "ls":
		cmdList()
	case "fetch":
		cmdFetch()
	case "videos", "vids":
		cmdVideos()
	case "category", "cat":
		cmdCategory()
	case "serve":
		cmdServe()
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
  add <url> [--category <name>]   Add a YouTube channel
  remove <id>                     Remove a channel by ID
  edit <id> --category <name>     Edit a channel (set or clear category)
  list                            List all tracked channels
  fetch [id]                      Fetch RSS feeds (all or specific channel)
  videos [id] [n]                 Show recent videos (limit n, default 20)
  category add <name>             Add a category
  category remove <name>          Remove a category
  category list                   List all categories
  serve [port]                    Start the web UI (default port 8080)
  help                            Show this help message`)
}

// parseFlag finds --flag value in os.Args and returns the value, removing both from remaining args.
func parseFlag(flag string) (string, []string) {
	var remaining []string
	var value string
	args := os.Args[2:] // skip binary + command
	for i := 0; i < len(args); i++ {
		if args[i] == "--"+flag && i+1 < len(args) {
			value = args[i+1]
			i++ // skip next
		} else {
			remaining = append(remaining, args[i])
		}
	}
	return value, remaining
}

func cmdAdd() {
	categoryName, remaining := parseFlag("category")

	if len(remaining) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: yt-rss add <channel-url> [--category <name>]")
		os.Exit(1)
	}

	rawURL := remaining[0]
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

	var categoryID *int64
	if categoryName != "" {
		cat, err := database.GetCategoryByName(categoryName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: category %q not found. Use 'yt-rss category add %s' first.\n", categoryName, categoryName)
			os.Exit(1)
		}
		categoryID = &cat.ID
	}

	if err := database.AddChannel(channelID, name, rawURL, categoryID); err != nil {
		fmt.Fprintf(os.Stderr, "Error adding channel: %v\n", err)
		os.Exit(1)
	}

	msg := fmt.Sprintf("Added channel: %s (%s)", name, channelID)
	if categoryName != "" {
		msg += fmt.Sprintf(" [%s]", categoryName)
	}
	fmt.Println(msg)
}

func cmdEdit() {
	categoryName, remaining := parseFlag("category")

	if len(remaining) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: yt-rss edit <id> --category <name>")
		fmt.Fprintln(os.Stderr, "       yt-rss edit <id> --category \"\"    (clear category)")
		os.Exit(1)
	}

	id, err := strconv.ParseInt(remaining[0], 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid channel ID: %s\n", remaining[0])
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

	// Handle --category flag (present even if empty string to clear)
	flagFound := false
	for _, arg := range os.Args[2:] {
		if arg == "--category" {
			flagFound = true
			break
		}
	}
	if !flagFound {
		fmt.Fprintln(os.Stderr, "Usage: yt-rss edit <id> --category <name>")
		os.Exit(1)
	}

	var categoryID *int64
	if categoryName != "" {
		cat, err := database.GetCategoryByName(categoryName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: category %q not found. Use 'yt-rss category add %s' first.\n", categoryName, categoryName)
			os.Exit(1)
		}
		categoryID = &cat.ID
	}

	if err := database.UpdateChannelCategory(id, categoryID); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating channel: %v\n", err)
		os.Exit(1)
	}

	if categoryName != "" {
		fmt.Printf("Updated %s: category set to %q\n", channel.Name, categoryName)
	} else {
		fmt.Printf("Updated %s: category cleared\n", channel.Name)
	}
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
	fmt.Fprintln(w, "ID\tCHANNEL\tCATEGORY\tCHANNEL ID\tLAST FETCHED\t")
	for _, c := range channels {
		lastFetched := "never"
		if c.LastFetched != nil {
			lastFetched = c.LastFetched.Format("2006-01-02 15:04")
		}
		category := "-"
		if c.CategoryName != "" {
			category = c.CategoryName
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t\n", c.ID, c.Name, category, c.ChannelID, lastFetched)
	}
	w.Flush()
}

func cmdCategory() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: yt-rss category <add|remove|list> [name]")
		os.Exit(1)
	}

	sub := os.Args[2]

	switch sub {
	case "add":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: yt-rss category add <name>")
			os.Exit(1)
		}
		name := strings.Join(os.Args[3:], " ")

		database, err := openDB()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		if _, err := database.AddCategory(name); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				fmt.Fprintf(os.Stderr, "Error: category %q already exists\n", name)
			} else {
				fmt.Fprintf(os.Stderr, "Error adding category: %v\n", err)
			}
			os.Exit(1)
		}
		fmt.Printf("Added category: %s\n", name)

	case "remove", "rm":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: yt-rss category remove <name>")
			os.Exit(1)
		}
		name := strings.Join(os.Args[3:], " ")

		database, err := openDB()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		cat, err := database.GetCategoryByName(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: category %q not found\n", name)
			os.Exit(1)
		}

		if err := database.RemoveCategory(cat.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Error removing category: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Removed category: %s (channels in this category are now uncategorised)\n", name)

	case "list", "ls":
		database, err := openDB()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		categories, err := database.ListCategories()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing categories: %v\n", err)
			os.Exit(1)
		}

		if len(categories) == 0 {
			fmt.Println("No categories yet. Use 'yt-rss category add <name>' to create one.")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\t")
		for _, c := range categories {
			fmt.Fprintf(w, "%d\t%s\t\n", c.ID, c.Name)
		}
		w.Flush()

	default:
		fmt.Fprintf(os.Stderr, "Unknown category subcommand: %s\n", sub)
		fmt.Fprintln(os.Stderr, "Usage: yt-rss category <add|remove|list> [name]")
		os.Exit(1)
	}
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

	fetcher := youtube.NewFetcher(database.UpsertVideo, database.UpdateLastFetched)
	results := fetcher.FetchChannels(channels)

	totalNew := 0
	for _, r := range results {
		if r.Error != "" {
			fmt.Fprintf(os.Stderr, "Error fetching %s: %s\n", r.ChannelName, r.Error)
			continue
		}
		fmt.Printf("Fetched %s: %d videos\n", r.ChannelName, r.VideoCount)
		totalNew += r.VideoCount
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
		fmt.Printf("Recent videos:\n\n")
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

func cmdServe() {
	port := "8080"
	if len(os.Args) >= 3 {
		port = os.Args[2]
	}

	database, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	srv := web.NewServer(database)

	addr := fmt.Sprintf(":%s", port)
	fmt.Printf("yt-rss web UI starting at http://localhost%s\n", addr)
	if err := http.ListenAndServe(addr, srv); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
