package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/boltdb/bolt"
)

// ErrItemNotFound is returned when an item is not found in the storage.
var ErrItemNotFound = errors.New("no such item")

// PodcastItemType is a type of a podcast item.
type PodcastItemType uint8

// Supported podcast item types.
const (
	YouTubeItem PodcastItemType = iota + 1
	TelegramItem
	UploadedItem
)

func (it PodcastItemType) String() string {
	switch it {
	case YouTubeItem:
		return "YouTube"
	case TelegramItem:
		return "Telegram"
	case UploadedItem:
		return "Uploaded"
	default:
		return "Unknown"
	}
}

// Description is a description of a podcast item.
type Description struct {
	Title string
	Body  string
}

// Status is a status of a podcast item.
type Status uint8

// Supported podcast item statuses.
const (
	ItemAdded Status = iota + 1
	ItemDownloaded
	ItemReady
)

// PodcastItem is a podcast item.
type PodcastItem struct {
	Description
	Type          PodcastItemType
	Author        string
	OriginalURL   string
	FileName      string
	Duration      time.Duration
	MIMEType      string
	ContentLength int64
	AddedAt       time.Time
	Status        Status
}

// NewPodcastItem creates a new podcast item from the given metadata.
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
		Status:        ItemAdded,
	}
}

// ID returns a unique ID of the podcast item.
func (item PodcastItem) ID() string {
	return item.AddedAt.UTC().Format(time.RFC3339Nano)
}

// Playable returns true i/*f the podcast item is ready to be played.
func (item PodcastItem) Playable() bool {
	return item.Status == ItemReady
}

type boltPodcastItem struct {
	Type          PodcastItemType `json:",omitempty"`
	Title         string          `json:",omitempty"`
	Author        string          `json:",omitempty"`
	Description   string          `json:",omitempty"`
	OriginalURL   string          `json:",omitempty"`
	MediaURL      string          `json:",omitempty"` // obsolete
	FileName      string          `json:",omitempty"`
	Duration      time.Duration   `json:",omitempty"`
	MIMEType      string          `json:",omitempty"`
	ContentLength int64           `json:",omitempty"`
	Status        Status          `json:",omitempty"`
}

func newBoltPodcastItem(item PodcastItem) boltPodcastItem {
	return boltPodcastItem{
		item.Type,
		item.Title,
		item.Author,
		item.Body,
		item.OriginalURL,
		"",
		item.FileName,
		item.Duration,
		item.MIMEType,
		item.ContentLength,
		item.Status,
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

		migrateMediaURL(&it)

		item = PodcastItem{
			Description{it.Title, it.Description},
			it.Type,
			it.Author,
			it.OriginalURL,
			it.FileName,
			it.Duration,
			it.MIMEType,
			it.ContentLength,
			addedAt,
			it.Status,
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
		migrateMediaURL(&it)

		v, err = json.Marshal(it)
		if err != nil {
			return fmt.Errorf("failed to marshal podcast item %q in %q: %w", k, s.Bucket, err)
		}

		if err := b.Put(k, v); err != nil {
			return fmt.Errorf("failed to store podcast item: %w", err)
		}

		item = PodcastItem{
			Description{it.Title, it.Description},
			it.Type,
			it.Author,
			it.OriginalURL,
			it.FileName,
			it.Duration,
			it.MIMEType,
			it.ContentLength,
			addedAt,
			it.Status,
		}

		return nil
	})
}

func (s *boltStorage) UpdateStatus(itemID string, newStatus Status) (PodcastItem, error) {
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

		it.Status = newStatus
		migrateMediaURL(&it)

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
			it.FileName,
			it.Duration,
			it.MIMEType,
			it.ContentLength,
			addedAt,
			it.Status,
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

			var it boltPodcastItem
			if err := json.Unmarshal(v, &it); err != nil {
				return fmt.Errorf("failed to unmarshal podcast item %q in %q: %w", k, s.Bucket, err)
			}

			if it.Status == 0 { // legacy items, assume they are ready
				it.Status = ItemReady
			}

			migrateMediaURL(&it)

			items = append(items, PodcastItem{
				Description{it.Title, it.Description},
				it.Type,
				it.Author,
				it.OriginalURL,
				it.FileName,
				it.Duration,
				it.MIMEType,
				it.ContentLength,
				addedAt,
				it.Status,
			})
		}

		return nil
	})
}

func migrateMediaURL(it *boltPodcastItem) {
	if it.FileName == "" {
		it.FileName = path.Base(it.MediaURL)
	}
}
