package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"mime"
	"os"
	"path"
	"strings"
)

type storage interface {
	Add(PodcastItem) error
	Remove(string) (PodcastItem, error)
	UpdateDescription(string, Description) (PodcastItem, error)
	Items() ([]PodcastItem, error)
}

type fileDownloader interface {
	DownloadFile(context.Context, string) (string, int64, error)
}

type mediaTranscoder interface {
	TranscodeMedia(context.Context, string) (int64, error)
}

// FeedService is a service that manages podcast items.
type FeedService struct {
	downloader  fileDownloader
	converter   mediaTranscoder
	st          storage
	storagePath string
}

// NewFeedService creates a new FeedService instance.
func NewFeedService(st storage, storagePath string, downloader fileDownloader, converter mediaTranscoder) *FeedService {
	return &FeedService{
		st:          st,
		downloader:  downloader,
		converter:   converter,
		storagePath: storagePath,
	}
}

// AddItem adds a new podcast item to the feed.
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
	transcodedSize, err := s.converter.TranscodeMedia(ctx, filePath)
	if err != nil {
		return fmt.Errorf("failed to transcode file: %w", err)
	}

	log.Printf("transcoded %s (new size %s)", audioURL, FileSize(transcodedSize))

	item.MediaURL = "/downloads/" + path.Base(filePath)
	item.Status = ItemReady

	if err := s.st.Add(item); err != nil {
		return fmt.Errorf("failed to add item to the feed: %w", err)
	}

	log.Println("added", audioURL, "to the feed")

	return nil
}

// UpdateItem updates an existing podcast item.
func (s *FeedService) UpdateItem(itemID string, desc Description) error {
	log.Printf("updating %s", itemID)

	_, err := s.st.UpdateDescription(itemID, desc)
	if err != nil {
		return err
	}

	return nil
}

// RemoveItem removes an existing podcast item.
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

// Items returns a list of podcast items.
func (s *FeedService) Items() ([]PodcastItem, error) {
	items, err := s.st.Items()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch podcast items: %w", err)
	}

	return items, nil
}
