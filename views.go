package main

import (
	"io"
	"log"
	"time"

	"github.com/eduncan911/podcast"
)

type Feed struct {
	URL, IconURL       string
	Title, Description string
	PubDate            time.Time
	Items              []PodcastItem
}

type AtomRenderer struct{}

func (AtomRenderer) ContentType() string {
	return "application/xml"
}

func (AtomRenderer) Render(w io.Writer, feed Feed) error {
	var pubDate *time.Time
	if !feed.PubDate.IsZero() {
		pubDate = &feed.PubDate
	}

	p := podcast.New(feed.Title, feed.URL, feed.Description, pubDate, nil)
	p.AddImage(feed.IconURL) // (scheme + "://" + req.Host + "/favicon.ico")

	for _, it := range feed.Items {
		item := podcast.Item{
			Title: it.Title,
			Author: &podcast.Author{
				Name:  it.Author,
				Email: "user@example.com",
			},
			Description: it.Description,
			Link:        it.OriginalURL,
			PubDate:     &it.AddedAt,
		}

		item.AddEnclosure(it.MediaURL, mimeTypeToEnclosureType(it.MIMEType), int64(it.ContentLength))

		if _, err := p.AddItem(item); err != nil {
			log.Printf("failed to add %s: %s", it.OriginalURL, err)
		}
	}

	return p.Encode(w)
}
