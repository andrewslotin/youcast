package youcast

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/kkdai/youtube/v2"
)

var ErrNoAudio = errors.New("no audio formats found")

type YouTubeVideo struct {
	c       youtube.Client
	videoID string
	log     *log.Logger
}

func NewYouTubeVideo(videoID string) *YouTubeVideo {
	return &YouTubeVideo{
		videoID: videoID,
		log:     log.New(log.Writer(), videoID+": ", log.LstdFlags),
	}
}

func (y *YouTubeVideo) AudioStreamURL(ctx context.Context) (string, string, error) {
	video, err := y.c.GetVideoContext(ctx, y.videoID)
	if err != nil {
		return "", "", fmt.Errorf("failed to get video info: %w", err)
	}
	y.log.Printf("got video info")

	bestAudio, mimeType, err := pickBestAudio(video.Formats)
	if err != nil {
		return "", "", fmt.Errorf("failed to find audio: %w", err)
	}

	y.log.Printf("got the best audio stream %s @ %d bps", mimeType, bestAudio.Bitrate)

	u, err := y.c.GetStreamURLContext(ctx, video, &bestAudio)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch URL for %s stream: %w", bestAudio.MimeType, err)
	}

	return u, mimeType, nil
}

func pickBestAudio(formats youtube.FormatList) (youtube.Format, string, error) {
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
			return formats[i].Bitrate < formats[j].Bitrate
		})

		return formats[0], mimeType, nil
	}

	return youtube.Format{}, "", ErrNoAudio
}
