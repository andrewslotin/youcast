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
	"path"
	"strings"
)

type Storage interface {
	Add(PodcastItem) error
	Remove(string) (PodcastItem, error)
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
	log.Printf("downloading %s", audioURL)

	filename, written, err := s.downloadFile(context.Background(), audioURL, item.MIMEType)
	if err != nil {
		return fmt.Errorf("failed to download item: %w", err)
	}

	log.Printf("downloaded %s to %s (%s written)", audioURL, filename, FileSize(written))
	item.MediaURL = "/downloads/" + filename

	if err := s.st.Add(item); err != nil {
		return fmt.Errorf("failed to add item to the feed: %w", err)
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

func (s *FeedService) downloadFile(ctx context.Context, u, mimeType string) (string, int64, error) {
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

	fileName := fmt.Sprintf("%x", sha256.Sum256([]byte(u)))
	if exts, err := mime.ExtensionsByType(mimeType); err != nil {
		log.Printf("failed to get file extensions list for %s: %s", mimeType, err)
	} else if len(exts) == 0 {
		log.Printf("no file extension registered for %s", mimeType)
	} else {
		fileName += exts[0]
	}

	fd, err := os.Create(path.Join(s.storagePath, fileName))
	if err != nil {
		return "", 0, fmt.Errorf("failed to create %s: %w", fileName, err)
	}
	defer fd.Close()

	written, err := io.Copy(fd, resp.Body)
	if err != nil {
		return "", written, fmt.Errorf("failed to download %s to %s: %w", u, fileName, err)
	}

	return fileName, written, nil
}
