package main

import (
	"crypto/sha256"
	"fmt"
	"log"
	"mime"
	"os"
	"path"
)

type storage interface {
	Add(PodcastItem) error
	Remove(string) (PodcastItem, error)
	UpdateDescription(string, Description) (PodcastItem, error)
	Items() ([]PodcastItem, error)
}

// FeedService is a service that manages podcast items.
type FeedService struct {
	q           *DownloadJobQueue
	st          storage
	storagePath string
}

// NewFeedService creates a new FeedService instance.
func NewFeedService(
	st storage,
	storagePath string,
	q *DownloadJobQueue,
	downloader fileDownloader,
	converter mediaTranscoder,
) *FeedService {
	return &FeedService{
		st:          st,
		storagePath: storagePath,
		q:           q,
	}
}

// AddItem adds a new podcast item to the feed.
func (s *FeedService) AddItem(item PodcastItem, audioURL string) error {
	filePath := path.Join(s.storagePath, fmt.Sprintf("%x", sha256.Sum256([]byte(audioURL))))
	if exts, err := mime.ExtensionsByType(item.MIMEType); err != nil {
		log.Printf("failed to get file extensions list for %s: %s", item.MIMEType, err)
	} else if len(exts) == 0 {
		log.Printf("no file extension registered for %s", item.MIMEType)
	} else {
		filePath += exts[0]
	}

	item.FileName, item.Status = path.Base(filePath), ItemAdded
	if err := s.st.Add(item); err != nil {
		return fmt.Errorf("failed to add item to the feed: %w", err)
	}

	if err := s.q.Add(DownloadJob{SourceURI: audioURL, TargetURI: filePath}); err != nil {
		return fmt.Errorf("failed to add download job for %s: %w", audioURL, err)
	}

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

	filePath := path.Join(s.storagePath, item.FileName)
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
