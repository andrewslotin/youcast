package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
)

type Storage interface {
	Add(PodcastItem) error
	Remove(string) (PodcastItem, error)
	UpdateDescription(string, Description) (PodcastItem, error)
	Items() ([]PodcastItem, error)
}

type FeedService struct {
	c           *http.Client
	st          Storage
	storagePath string
}

func NewFeedService(st Storage, storagePath string, c *http.Client) *FeedService {
	if c == nil {
		c = http.DefaultClient
	}

	return &FeedService{
		c:           c,
		st:          st,
		storagePath: storagePath,
	}
}

func (s *FeedService) AddItem(item PodcastItem, audioURL string) error {
	ctx := context.Background()

	log.Printf("downloading %s", audioURL)

	filePath := path.Join(s.storagePath, fmt.Sprintf("%x", sha256.Sum256([]byte(audioURL))))
	if exts, err := mime.ExtensionsByType(item.MIMEType); err != nil {
		log.Printf("failed to get file extensions list for %s: %s", item.MIMEType, err)
	} else if len(exts) == 0 {
		log.Printf("no file extension registered for %s", item.MIMEType)
	} else {
		filePath += exts[0]
	}

	_, written, err := s.downloadFile(ctx, audioURL, filePath)
	if err != nil {
		return fmt.Errorf("failed to download item: %w", err)
	}

	log.Printf("downloaded %s to %s (%s written)", audioURL, filePath, FileSize(written))

	log.Println("transcoding", filePath)
	if err := s.transcodeFile(ctx, filePath); err != nil {
		return fmt.Errorf("failed to transcode file: %w", err)
	}

	item.MediaURL = "/downloads/" + path.Base(filePath)

	if err := s.st.Add(item); err != nil {
		return fmt.Errorf("failed to add item to the feed: %w", err)
	}

	log.Println("added", audioURL, "to the feed")

	return nil
}

func (s *FeedService) UpdateItem(itemID string, desc Description) error {
	log.Printf("updating %s", itemID)

	_, err := s.st.UpdateDescription(itemID, desc)
	if err != nil {
		return err
	}

	return nil
}

func (s *FeedService) RemoveItem(itemID string) error {
	log.Printf("removing %s", itemID)

	item, err := s.st.Remove(itemID)
	if err != nil {
		return err
	}

	filePath := path.Join(s.storagePath, strings.TrimPrefix(item.MediaURL, "/downloads/"))
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to delete %s: %w", filePath, err)
	}

	return nil
}

func (s *FeedService) Items() ([]PodcastItem, error) {
	items, err := s.st.Items()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch podcast items: %w", err)
	}

	return items, nil
}

func (s *FeedService) downloadFile(ctx context.Context, u, filePath string) (string, int64, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", 0, fmt.Errorf("failed to build a request to %s: %w", u, err)
	}

	resp, err := s.c.Do(req.WithContext(ctx))
	if err != nil {
		return "", 0, fmt.Errorf("failed to build a request to %s: %w", u, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return "", 0, fmt.Errorf("failed to download %s: server responded with %s", u, resp.Status)
	}

	fd, err := os.Create(filePath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create %s: %w", filePath, err)
	}
	defer fd.Close()

	written, err := io.Copy(fd, resp.Body)
	if err != nil {
		return "", written, fmt.Errorf("failed to download %s to %s: %w", u, filePath, err)
	}

	return filePath, written, nil
}

// ffmpeg -i $filePath -c:a copy -vn $tempFile
func (s *FeedService) transcodeFile(ctx context.Context, filePath string) error {
	ext := path.Ext(filePath)
	tempFile := strings.TrimSuffix(filePath, ext) + ".tmp" + ext
	defer os.Remove(tempFile)

	out, err := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", filePath, "-c:a", "copy", "-vn", tempFile).CombinedOutput()
	if err != nil {
		log.Println("ffmpeg responded with", string(out))
		return fmt.Errorf("failed to transcode file: %w", err)
	}

	if err := os.Rename(tempFile, filePath); err != nil {
		return fmt.Errorf("failed to rename %s to %s: %w", tempFile, filePath, err)
	}

	return nil
}
