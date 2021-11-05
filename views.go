package main

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/andrewslotin/youcast/assets"
	"github.com/eduncan911/podcast"
)

type Feed struct {
	URL, IconURL       string
	Title, Description string
	PubDate            time.Time
	Items              []PodcastItem
}

var Templates = template.Must(template.New("").Funcs(template.FuncMap{
	"stripScheme": func(s string) string {
		if ind := strings.Index(s, "://"); ind > -1 {
			return s[ind+3:]
		}

		return s
	},
	"formatDuration": func(d time.Duration) string {
		d = d.Round(time.Second)

		var s string
		if d >= 1*time.Hour {
			s = strconv.Itoa(int(d/time.Hour)) + ":"
			d -= (d / time.Hour) * time.Hour
		}

		return s + fmt.Sprintf("%02d:%02d", int(d/time.Minute), int(d%time.Minute/time.Second))
	},
}).ParseFS(assets.Templates, "*.html.tmpl"))

type HTMLRenderer struct{}

func (HTMLRenderer) ContentType() string {
	return "text/html; charset=utf-8"
}

func (HTMLRenderer) Render(w io.Writer, feed Feed) error {
	return Templates.ExecuteTemplate(w, "index.html.tmpl", feed)
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
	p.AddImage(feed.IconURL)

	for _, it := range feed.Items {
		item := podcast.Item{
			Title: it.Title,
			Author: &podcast.Author{
				Name:  it.Author,
				Email: "user@example.com",
			},
			Description: it.Body,
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
