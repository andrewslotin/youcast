package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/boltdb/bolt"
)

var ErrItemNotFound = errors.New("no such item")

type PodcastItemType uint8

const (
	YouTubeItem PodcastItemType = iota + 1
	TelegramItem
)

type Description struct {
	Title string
	Body  string
}

type PodcastItem struct {
	Description
	Type          PodcastItemType
	Author        string
	OriginalURL   string
	MediaURL      string
	Duration      time.Duration
	MIMEType      string
	ContentLength int64
	AddedAt       time.Time
}

func NewPodcastItem(meta Metadata, addedAt time.Time) PodcastItem {
	return PodcastItem{
		Description: Description{
			Title: meta.Title,
			Body:  meta.Description,
		},
		Type:          meta.Type,
		Author:        meta.Author,
		OriginalURL:   meta.OriginalURL,
		Duration:      meta.Duration,
		MIMEType:      meta.MIMEType,
		ContentLength: meta.ContentLength,
		AddedAt:       addedAt,
	}
}

func (item PodcastItem) ID() string {
	return item.AddedAt.UTC().Format(time.RFC3339Nano)
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

func (s *memoryStorage) Remove(itemID string) (PodcastItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, item := range s.items {
		if item.ID() == itemID {
			copy(s.items[i:], s.items[i+1:])
			s.items = s.items[:len(s.items)-1]

			return item, nil
		}
	}

	return PodcastItem{}, ErrItemNotFound
}

func (s *memoryStorage) UpdateDescription(itemID string, desc Description) (PodcastItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.items {
		if s.items[i].ID() == itemID {
			s.items[i].Description = desc

			return s.items[i], nil
		}
	}

	return PodcastItem{}, ErrItemNotFound
}

func (s *memoryStorage) Items() ([]PodcastItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]PodcastItem, len(s.items))
	copy(items, s.items)

	return items, nil
}

type boltPodcastItem struct {
	Type          PodcastItemType `json:",omitempty"`
	Title         string          `json:",omitempty"`
	Author        string          `json:",omitempty"`
	Description   string          `json:",omitempty"`
	OriginalURL   string          `json:",omitempty"`
	MediaURL      string          `json:",omitempty"`
	Duration      time.Duration   `json:",omitempty"`
	MIMEType      string          `json:",omitempty"`
	ContentLength int64           `json:",omitempty"`
}

func newBoltPodcastItem(item PodcastItem) boltPodcastItem {
	return boltPodcastItem{
		item.Type,
		item.Title,
		item.Author,
		item.Body,
		item.OriginalURL,
		item.MediaURL,
		item.Duration,
		item.MIMEType,
		item.ContentLength,
	}
}

type boltStorage struct {
	Bucket []byte
	db     *bolt.DB
}

func newBoltStorage(bucket string, db *bolt.DB) *boltStorage {
	return &boltStorage{[]byte(bucket), db}
}

func (s *boltStorage) Add(item PodcastItem) error {
	key := []byte(item.ID())
	data, err := json.Marshal(newBoltPodcastItem(item))
	if err != nil {
		return fmt.Errorf("failed to marshal podcast item: %w", err)
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(s.Bucket)
		if err != nil {
			return fmt.Errorf("failed to open bucket %q: %w", s.Bucket, err)
		}

		if err := b.Put(key, data); err != nil {
			return fmt.Errorf("failed to store podcast item into %q: %w", s.Bucket, err)
		}

		return nil
	})
}

func (s *boltStorage) Remove(itemID string) (PodcastItem, error) {
	var item PodcastItem

	return item, s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(s.Bucket)
		if b == nil {
			return ErrItemNotFound
		}

		k := []byte(itemID)
		v := b.Get(k)
		if v == nil {
			return ErrItemNotFound
		}

		addedAt, err := time.Parse(time.RFC3339Nano, string(k))
		if err != nil {
			return fmt.Errorf("failed to parse podcast item key %q in %q: %w", k, s.Bucket, err)
		}

		if err := b.Delete(k); err != nil {
			return fmt.Errorf("failed to remove podcast item: %w", err)
		}

		var it boltPodcastItem
		if err := json.Unmarshal(v, &it); err != nil {
			return fmt.Errorf("failed to unmarshal podcast item %q in %q: %w", k, s.Bucket, err)
		}

		item = PodcastItem{
			Description{it.Title, it.Description},
			it.Type,
			it.Author,
			it.OriginalURL,
			it.MediaURL,
			it.Duration,
			it.MIMEType,
			it.ContentLength,
			addedAt,
		}

		return nil
	})
}

func (s *boltStorage) UpdateDescription(itemID string, desc Description) (PodcastItem, error) {
	var item PodcastItem

	return item, s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(s.Bucket)
		if b == nil {
			return ErrItemNotFound
		}

		k := []byte(itemID)
		v := b.Get(k)
		if v == nil {
			return ErrItemNotFound
		}

		addedAt, err := time.Parse(time.RFC3339Nano, string(k))
		if err != nil {
			return fmt.Errorf("failed to parse podcast item key %q in %q: %w", k, s.Bucket, err)
		}

		var it boltPodcastItem
		if err := json.Unmarshal(v, &it); err != nil {
			return fmt.Errorf("failed to unmarshal podcast item %q in %q: %w", k, s.Bucket, err)
		}

		it.Title, it.Description = desc.Title, desc.Body

		v, err = json.Marshal(it)
		if err != nil {
			return fmt.Errorf("failed to marsha podcast item %q in %q: %w", k, s.Bucket, err)
		}

		if err := b.Put(k, v); err != nil {
			return fmt.Errorf("failed to store podcast item: %w", err)
		}

		item = PodcastItem{
			Description{it.Title, it.Description},
			it.Type,
			it.Author,
			it.OriginalURL,
			it.MediaURL,
			it.Duration,
			it.MIMEType,
			it.ContentLength,
			addedAt,
		}

		return nil
	})
}

func (s *boltStorage) Items() ([]PodcastItem, error) {
	var items []PodcastItem
	return items, s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(s.Bucket)
		if b == nil {
			return nil
		}

		c := b.Cursor()
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			addedAt, err := time.Parse(time.RFC3339Nano, string(k))
			if err != nil {
				return fmt.Errorf("failed to parse podcast item key %q in %q: %w", k, s.Bucket, err)
			}

			var item boltPodcastItem
			if err := json.Unmarshal(v, &item); err != nil {
				return fmt.Errorf("failed to unmarshal podcast item %q in %q: %w", k, s.Bucket, err)
			}

			items = append(items, PodcastItem{
				Description{item.Title, item.Description},
				item.Type,
				item.Author,
				item.OriginalURL,
				item.MediaURL,
				item.Duration,
				item.MIMEType,
				item.ContentLength,
				addedAt,
			})
		}

		return nil
	})
}
