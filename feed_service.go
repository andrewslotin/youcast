package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"mime"
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

type FileDownloader interface {
	DownloadFile(context.Context, string) (string, int64, error)
}

type FeedService struct {
	downloader  FileDownloader
	st          Storage
	storagePath string
}

func NewFeedService(st Storage, storagePath string, downloader FileDownloader) *FeedService {
	return &FeedService{
		st:          st,
		downloader:  downloader,
		storagePath: storagePath,
	}
}

func (s *FeedService) AddItem(item PodcastItem, audioURL string) error {
	ctx := context.Background()

	log.Printf("downloading %s", audioURL)

	tmpFile, written, err := s.downloader.DownloadFile(ctx, audioURL)
	if err != nil {
		return fmt.Errorf("failed to download item: %w", err)
	}
	defer os.Remove(tmpFile) // in case it still exists

	filePath := path.Join(s.storagePath, fmt.Sprintf("%x", sha256.Sum256([]byte(audioURL))))
	if exts, err := mime.ExtensionsByType(item.MIMEType); err != nil {
		log.Printf("failed to get file extensions list for %s: %s", item.MIMEType, err)
	} else if len(exts) == 0 {
		log.Printf("no file extension registered for %s", item.MIMEType)
	} else {
		filePath += exts[0]
	}

	if err := os.Rename(tmpFile, filePath); err != nil {
		return fmt.Errorf("failed to rename %s to %s: %w", tmpFile, filePath, err)
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
