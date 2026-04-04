# yt-rss

Keep track of YouTube channels without actually subscribing to them.

## Why

YouTube's subscription page is a single unsorted list. If you follow a mix of tech channels, music labels, and indie artists, it turns into noise fast. And if you only care about one musician on a record label's channel, tough - you're getting every upload from every artist on the roster.

I got tired of that. yt-rss pulls YouTube channels via their public RSS feeds and gives you a CLI and a little web UI to browse recent uploads. You organise channels into categories and filter by them. No YouTube account, no algorithm, no notifications.

It's all stored in a local SQLite database. Compiles to a single binary, no external dependencies.

## Prerequisites

- Go 1.25 or later

## Getting started

Clone the repo and build:

```bash
git clone git@github.com:ohnotnow/yt-rss.git
cd yt-rss
go build -o yt-rss ./cmd/
```

Or just grab a binary from the [releases page](https://github.com/ohnotnow/yt-rss/releases) - there are builds for macOS, Linux, and Windows.

## Usage

### Categories

Create a few categories first:

```bash
yt-rss category add music
yt-rss category add tech
yt-rss category list
```

### Adding channels

Pass any YouTube channel URL - handles, `/channel/` URLs, legacy `/user/` URLs, whatever.

```bash
yt-rss add https://www.youtube.com/@somechannel
yt-rss add https://www.youtube.com/@somechannel --category music
```

Wrong category? Just edit it:

```bash
yt-rss edit 1 --category tech
```

### Fetching and browsing

Pull the latest uploads:

```bash
yt-rss fetch       # all channels
yt-rss fetch 3     # just channel 3
```

Then browse them in the terminal:

```bash
yt-rss videos        # latest 20 across all channels
yt-rss videos 3 50   # latest 50 from channel 3
```

### Web UI

```bash
yt-rss serve          # default port 8080
yt-rss serve 3000     # custom port
```

Search bar, category filters, grid of video cards that link straight to YouTube. The HTML is embedded in the binary so there's nothing else to deploy.

### Other commands

```bash
yt-rss list           # show all tracked channels
yt-rss remove 5       # stop tracking a channel
yt-rss help           # full command reference
```

## Running tests

```bash
go test ./...
```

## Contributing

Fork it, hack on it, run `go build ./cmd/` and `go test ./...`, open a PR.

## Licence

[MIT](LICENSE)
