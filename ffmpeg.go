package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
)

type FFMpeg struct{}

func NewFFMpeg() *FFMpeg {
	return &FFMpeg{}
}

// ffmpeg -i $filePath -c:a copy -vn $tempFile
func (svc *FFMpeg) TranscodeMedia(ctx context.Context, filePath string) (int64, error) {
	ext := path.Ext(filePath)
	tempFile := strings.TrimSuffix(filePath, ext) + ".tmp" + ext
	defer os.Remove(tempFile)

	out, err := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", filePath, "-c:a", "copy", "-vn", tempFile).CombinedOutput()
	if err != nil {
		log.Println("ffmpeg responded with", string(out))
		return 0, fmt.Errorf("failed to transcode file: %w", err)
	}

	fi, err := os.Stat(tempFile)
	if err != nil {
		return 0, fmt.Errorf("failed to get file info for %s: %w", tempFile, err)
	}

	if err := os.Rename(tempFile, filePath); err != nil {
		return 0, fmt.Errorf("failed to rename %s to %s: %w", tempFile, filePath, err)
	}

	return fi.Size(), nil
}
