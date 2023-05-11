package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/kkdai/youtube/v2"
)

// ErrNoAudio is returned when no suitable audio formats are found for a YouTube video.
var ErrNoAudio = errors.New("no audio formats found")

// YouTubeVideo is a YouTube video that provides audio files to the podcast feed.
type YouTubeVideo struct {
	c       youtube.Client
	videoID string
	log     *log.Logger
}

// YouTubeProvider is a YouTube video that provides audio files to the podcast feed.
type YouTubeProvider struct{}

// NewYouTubeProvider creates a new YouTubeProvider instance.
func (yt *YouTubeProvider) Name() string {
	return "YouTube video"
}

// HandleRequest handles a request for a YouTube video.
func (yt *YouTubeProvider) HandleRequest(w http.ResponseWriter, req *http.Request) audioSource {
	u := req.FormValue("url")
	if u == "" {
		http.Error(w, "missing url= parameter", http.StatusBadRequest)
		return nil
	}

	id, err := extractYouTubeID(u)
	if err != nil {
		http.Error(w, "failed to parse YouTube video URL: "+err.Error(), http.StatusBadRequest)
		return nil
	}

	if u := req.Referer(); u != "" { // added via the UI form field
		http.Redirect(w, req, u, http.StatusSeeOther)
	}

	http.Redirect(w, req, u, http.StatusSeeOther)

	return NewYouTubeVideo(id)
}

func extractYouTubeID(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("failed to parse YouTube link: %w", err)
	}

	id := u.Query().Get("v")
	if id == "" {
		return "", fmt.Errorf("unsupported YouTube link %s", s)
	}

	return id, nil
}

// NewYouTubeVideo creates a new YouTubeVideo instance.
func NewYouTubeVideo(videoID string) *YouTubeVideo {
	return &YouTubeVideo{
		videoID: videoID,
		log:     log.New(log.Writer(), videoID+": ", log.LstdFlags),
	}
}

// Metadata returns the metadata for the YouTube video.
func (y *YouTubeVideo) Metadata(ctx context.Context) (Metadata, error) {
	video, err := y.c.GetVideoContext(ctx, y.videoID)
	if err != nil {
		return Metadata{}, fmt.Errorf("failed to get video info: %w", err)
	}

	_, bestAudio, err := y.bestAudio(ctx)
	if err != nil {
		return Metadata{}, fmt.Errorf("failed to find audio: %w", err)
	}

	y.log.Printf("got the best audio stream %s @ %d bps", bestAudio.MimeType, bestAudio.Bitrate)

	mimeType := bestAudio.MimeType
	if ind := strings.IndexByte(mimeType, ';'); ind >= 0 {
		mimeType = mimeType[:ind]
	}

	return Metadata{
		Type:          YouTubeItem,
		OriginalURL:   "https://youtube.com/watch?v=" + y.videoID,
		Title:         video.Title,
		Author:        video.Author,
		Duration:      video.Duration,
		MIMEType:      mimeType,
		ContentLength: bestAudio.ContentLength,
	}, nil
}

// DownloadURL returns the URL to download the YouTube video.
func (y *YouTubeVideo) DownloadURL(ctx context.Context) (string, error) {
	u, _, err := y.bestAudio(ctx)
	return u, err
}

func (y *YouTubeVideo) bestAudio(ctx context.Context) (string, youtube.Format, error) {
	video, err := y.c.GetVideoContext(ctx, y.videoID)
	if err != nil {
		return "", youtube.Format{}, fmt.Errorf("failed to get video info: %w", err)
	}

	bestAudio, err := pickBestAudio(video.Formats)
	if err != nil {
		return "", youtube.Format{}, fmt.Errorf("failed to find audio: %w", err)
	}

	u, err := y.c.GetStreamURLContext(ctx, video, &bestAudio)
	if err != nil {
		return "", youtube.Format{}, fmt.Errorf("failed to fetch %s stream: %w", bestAudio.MimeType, err)
	}

	return u, bestAudio, nil
}

func pickBestAudio(formats youtube.FormatList) (youtube.Format, error) {
	audio := make(map[string]youtube.FormatList)
	for _, format := range formats {
		if !strings.HasPrefix(format.MimeType, "audio/") {
			continue
		}

		kv := strings.SplitN(format.MimeType, ";", 2)
		audio[kv[0]] = append(audio[kv[0]], format)
	}

	for _, mimeType := range [...]string{"audio/mp4", "audio/mp3"} {
		formats, ok := audio[mimeType]
		if !ok || len(formats) == 0 {
			continue
		}

		sort.Slice(formats, func(i, j int) bool {
			if formats[i].AudioChannels == formats[j].AudioChannels {
				return parseAudioQuality(formats[i].AudioQuality) > parseAudioQuality(formats[j].AudioQuality)
			}

			return formats[i].AudioChannels > formats[j].AudioChannels
		})

		return formats[0], nil
	}

	return youtube.Format{}, ErrNoAudio
}

type audioQuality uint8

const (
	unknownQuality audioQuality = iota
	lowQuality
	mediumQuality
	highQuality
)

func parseAudioQuality(s string) audioQuality {
	switch s {
	case "AUDIO_QUALITY_LOW":
		return lowQuality
	case "AUDIO_QUALITY_MEDIUM":
		return mediumQuality
	case "AUDIO_QUALITY_HIGH":
		return highQuality
	default:
		return unknownQuality
	}
}
