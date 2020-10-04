package main

import (
	"sync"
	"time"
)

type PodcastItem struct {
	Title         string
	Author        string
	URL           string
	OriginalURL   string
	Duration      time.Duration
	MIMEType      string
	ContentLength int64
	AddedAt       time.Time
}

type memoryStorage struct {
	mu    sync.RWMutex
	items []PodcastItem
}

func (s *memoryStorage) Add(item PodcastItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.items = append(s.items, item)

	return nil
}

func (s *memoryStorage) Items() ([]PodcastItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]PodcastItem, len(s.items))
	copy(items, s.items)

	return items, nil
}
