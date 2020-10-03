package youcast

import (
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"os"
	"sort"
	"strings"

	"github.com/kkdai/youtube/v2"
)

var ErrNoAudio = errors.New("no audio formats found")

func DownloadAudio(videoID string) error {
	log.Printf("downloading %s", videoID)

	yt := NewYouTubeVideo(videoID)

	stream, mimeType, err := yt.AudioStream()
	if err != nil {
		return fmt.Errorf("failed to download audio: %w", err)
	}
	defer stream.Close()

	var ext string
	if exts, err := mime.ExtensionsByType(mimeType); err == nil && len(exts) > 0 {
		ext = exts[0]
	}

	fd, err := os.Create(videoID + ext)
	if err != nil {
		return fmt.Errorf("failed to create the output file: %w", err)
	}
	defer fd.Close()

	n, err := io.Copy(fd, stream)
	if err != nil {
		return fmt.Errorf("failed to store downloaded audio: %w", err)
	}

	log.Printf("%s: downloaded %d bytes", videoID, n)

	return nil
}

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

func (y *YouTubeVideo) AudioStream() (io.ReadCloser, string, error) {
	video, err := y.c.GetVideo(y.videoID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get video info: %w", err)
	}
	y.log.Printf("got video info")

	bestAudio, mimeType, err := pickBestAudio(video.Formats)
	if err != nil {
		return nil, "", fmt.Errorf("failed to find audio: %w", err)
	}

	y.log.Printf("got the best audio stream %s @ %d bps", mimeType, bestAudio.Bitrate)

	s, err := y.c.GetStream(video, &bestAudio)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch %s stream: %w", bestAudio.MimeType, err)
	}

	return s.Body, mimeType, nil
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
