package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kkdai/youtube/v2"
)

var ErrNoAudio = errors.New("no audio formats found")

type YouTubeVideo struct {
	c       youtube.Client
	videoID string
	log     *log.Logger
}

type Metadata struct {
	Link          string
	Title         string
	Author        string
	Duration      time.Duration
	MIMEType      string
	ContentLength int64
}

func NewYouTubeVideo(videoID string) *YouTubeVideo {
	return &YouTubeVideo{
		videoID: videoID,
		log:     log.New(log.Writer(), videoID+": ", log.LstdFlags),
	}
}

func (y *YouTubeVideo) Metadata(ctx context.Context) (Metadata, error) {
	video, err := y.c.GetVideoContext(ctx, y.videoID)
	if err != nil {
		return Metadata{}, fmt.Errorf("failed to get video info: %w", err)
	}

	bestAudio, err := pickBestAudio(video.Formats)
	if err != nil {
		return Metadata{}, fmt.Errorf("failed to find audio: %w", err)
	}

	y.log.Printf("got the best audio stream %s @ %d bps", bestAudio.MimeType, bestAudio.Bitrate)

	cl, err := strconv.ParseInt(bestAudio.ContentLength, 10, 64)
	if err != nil {
		log.Printf("failed to parse content length, will estimate: %s", err)
		cl = int64(math.Ceil(float64(bestAudio.Bitrate) * video.Duration.Seconds()))
	}

	return Metadata{
		Link:          "https://www.youtube.com/watch?v=" + y.videoID,
		Title:         video.Title,
		Author:        video.Author,
		Duration:      video.Duration,
		MIMEType:      bestAudio.MimeType,
		ContentLength: cl,
	}, nil
}

func (y *YouTubeVideo) AudioStream(ctx context.Context) (io.ReadCloser, http.Header, error) {
	video, err := y.c.GetVideoContext(ctx, y.videoID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get video info: %w", err)
	}

	bestAudio, err := pickBestAudio(video.Formats)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find audio: %w", err)
	}

	y.log.Printf("found %s stream for %s @ %d bps", bestAudio.MimeType, y.videoID, bestAudio.Bitrate)

	resp, err := y.c.GetStreamContext(ctx, video, &bestAudio)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch %s stream: %w", bestAudio.MimeType, err)
	}

	return resp.Body, resp.Header, nil
}

func (y *YouTubeVideo) AudioStreamURL(ctx context.Context) (string, error) {
	video, err := y.c.GetVideoContext(ctx, y.videoID)
	if err != nil {
		return "", fmt.Errorf("failed to get video info: %w", err)
	}

	bestAudio, err := pickBestAudio(video.Formats)
	if err != nil {
		return "", fmt.Errorf("failed to find audio: %w", err)
	}

	u, err := y.c.GetStreamURLContext(ctx, video, &bestAudio)
	if err != nil {
		return "", fmt.Errorf("failed to fetch %s stream: %w", bestAudio.MimeType, err)
	}

	return u, nil
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
