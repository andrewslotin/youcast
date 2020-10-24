package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/andrewslotin/youcast/assets"
	"github.com/eduncan911/podcast"
)

type Storage interface {
	Add(PodcastItem) error
	Items() ([]PodcastItem, error)
}

type PodcastMetadata struct {
	Title       string
	Link        string
	Description string
}

type audioSource interface {
	Metadata(context.Context) (Metadata, error)
	AudioStreamURL(context.Context) (string, error)
}

type audioSourceProvider interface {
	Name() string
	HandleRequest(http.ResponseWriter, *http.Request) audioSource
}

type FeedServer struct {
	st        Storage
	meta      PodcastMetadata
	providers map[string]audioSourceProvider
}

func NewFeedServer(meta PodcastMetadata, st Storage) *FeedServer {
	return &FeedServer{
		st:        st,
		meta:      meta,
		providers: make(map[string]audioSourceProvider),
	}
}

func (srv *FeedServer) ServeMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/", srv.ServeIndex)
	mux.HandleFunc("/add/", srv.HandleAddItem)
	mux.HandleFunc("/feed", srv.ServeFeed)
	mux.HandleFunc("/audio/youtube", srv.ServeYoutubeAudio)
	mux.HandleFunc("/favicon.ico", srv.ServeIcon)

	return mux
}

func (srv *FeedServer) RegisterProvider(subPath string, p audioSourceProvider) {
	srv.providers[subPath] = p
}

var indexTemplate = template.Must(template.New("index").Parse(`
<!DOCTYPE html>
<html>
  <head>
    <title>{{ .Title }} - listen videos later</title>
    <link href="https://fonts.googleapis.com/icon?family=Material+Icons" rel="stylesheet">
    <link type="text/css" rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/css/materialize.min.css" media="screen,projection"/>
    <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
  </head>
  <body>
    <div class="container">
      <header>
        <h1>{{ .Title }}</h1>
      </header>
      <div class="row">
        Drag &amp; drop this bookmarklet to your favorites bar.
      </div>
      <div class="row">
        <a class="btn" href="javascript:(function(){window.location='https://{{ .Host }}/?url='+encodeURIComponent(window.location);})();">Listen later</a>
      </div>
      <div class="row">
        Click it while on YouTube video page to add its audio version to your personal podcast.
      </div>
    </div>
    <script type="text/javascript" src="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/js/materialize.min.js"></script>
  </body>
</html>
`))

func (srv *FeedServer) ServeIndex(w http.ResponseWriter, req *http.Request) {
	if err := indexTemplate.Execute(w, struct {
		Host, Title string
	}{req.Host, srv.meta.Title}); err != nil {
		log.Printf("failed to render index template: %s", err)
	}
}

func (srv *FeedServer) ServeFeed(w http.ResponseWriter, req *http.Request) {
	items, err := srv.st.Items()
	if err != nil {
		log.Println("failed to fetch podcast items: ", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	var pubDate *time.Time
	if len(items) > 0 {
		pubDate = &items[len(items)-1].AddedAt
	}

	scheme := "http"
	if req.TLS != nil {
		scheme = "https"
	}

	feedLink := srv.meta.Link
	if feedLink == "" {
		feedLink = scheme + "://" + req.Host
	}

	p := podcast.New(srv.meta.Title, feedLink, srv.meta.Description, pubDate, nil)
	p.AddImage("https://" + req.Host + "/favicon.ico")

	for _, it := range items {
		id, err := extractYouTubeID(it.OriginalURL)
		if err != nil {
			log.Printf("failed to extract YouTube video ID from %s: %s", it.OriginalURL, err)
			continue
		}

		item := podcast.Item{
			Title: it.Title,
			Author: &podcast.Author{
				Name:  it.Author,
				Email: "user@example.com",
			},
			Description: it.Title,
			Link:        it.OriginalURL,
			PubDate:     &it.AddedAt,
		}
		item.AddEnclosure(
			scheme+"://"+req.Host+"/audio/youtube?v="+id,
			mimeTypeToEnclosureType(it.MIMEType),
			int64(it.ContentLength),
		)

		if _, err := p.AddItem(item); err != nil {
			log.Printf("failed to add %s: %s", it.OriginalURL, err)
		}
	}

	w.Header().Set("Content-Type", "application/xml")
	p.Encode(w)
}

func (srv *FeedServer) ServeIcon(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Context-Type", "image/png")
	w.Write(assets.Icon)
}

func (srv *FeedServer) HandleAddItem(w http.ResponseWriter, req *http.Request) {
	p, ok := srv.providers[strings.TrimPrefix(req.URL.Path, "/add")]
	if !ok {
		http.NotFound(w, req)
		return
	}

	audio := p.HandleRequest(w, req)
	if audio == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		meta, err := audio.Metadata(ctx)
		if err != nil {
			log.Printf("failed to fetch %s data: %s", p.Name(), err)
			return
		}

		if err := srv.st.Add(PodcastItem{
			Type:          YouTubeItem,
			Title:         meta.Title,
			Author:        meta.Author,
			OriginalURL:   meta.Link,
			Duration:      meta.Duration,
			MIMEType:      meta.MIMEType,
			ContentLength: meta.ContentLength,
			AddedAt:       time.Now(),
		}); err != nil {
			log.Printf("failed to add %s item to the feed: %s", p.Name(), err)
			return
		}
	}()
}

func (srv *FeedServer) ServeYoutubeAudio(w http.ResponseWriter, req *http.Request) {
	id := req.FormValue("v")
	if id == "" {
		http.Error(w, "missing v=<youtubeID> parameter", http.StatusBadRequest)
		return
	}

	u, err := NewYouTubeVideo(id).AudioStreamURL(req.Context())
	if err != nil {
		if err == ErrNoAudio {
			http.Error(w, "no audio found for "+id, http.StatusNotFound)
			return
		}

		log.Println("failed to open YouTube video stream:", err)
		return
	}

	http.Redirect(w, req, u, http.StatusSeeOther)
}

func mimeTypeToEnclosureType(mime string) podcast.EnclosureType {
	kv := strings.SplitN(mime, ";", 2)
	switch kv[0] {
	case "audio/mp4":
		return podcast.M4A
	case "video/mp4":
		return podcast.M4V
	default:
		return podcast.MP3
	}
}
