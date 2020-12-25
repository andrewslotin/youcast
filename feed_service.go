package main

import (
	"fmt"
	"net/http"
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
