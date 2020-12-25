package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
)

type Storage interface {
	Add(PodcastItem) error
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
	filename, written, err := s.downloadFile(context.Background(), audioURL)
	if err != nil {
		return fmt.Errorf("failed to download item: %w", err)
	}

	log.Println("downloaded %s to %s (%d bytes written)", audioURL, filename, written)
	item.OriginalURL = "/downloads/" + filename

	if err := s.st.Add(item); err != nil {
		return fmt.Errorf("failed to add item to the feed: %w", err)
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

func (s *FeedService) downloadFile(ctx context.Context, u string) (string, int64, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", 0, fmt.Errorf("failed to build a request to %s: %w", u, err)
	}

	resp, err := s.c.Do(req.WithContext(ctx))
	if err != nil {
		return "", 0, fmt.Errorf("failed to build a request to %s: %w", u, err)
	}
	defer resp.Body.Close()

	fileName := path.Join(s.storagePath, fmt.Sprintf("%x", sha256.Sum256([]byte(u))))
	fd, err := os.Create(fileName)
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
