package main

import (
	"context"
	"html/template"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/andrewslotin/youcast/assets"
	"github.com/eduncan911/podcast"
)

type PodcastMetadata struct {
	Title       string
	Link        string
	Description string
}

type Metadata struct {
	Type          PodcastItemType
	OriginalURL   string
	Title         string
	Description   string
	Author        string
	Duration      time.Duration
	MIMEType      string
	ContentLength int64
}

type audioSource interface {
	Metadata(context.Context) (Metadata, error)
	DownloadURL(context.Context) (string, error)
}

type audioSourceProvider interface {
	Name() string
	HandleRequest(http.ResponseWriter, *http.Request) audioSource
}

type FeedServer struct {
	svc       *FeedService
	meta      PodcastMetadata
	providers map[string]audioSourceProvider
}

func NewFeedServer(meta PodcastMetadata, svc *FeedService) *FeedServer {
	return &FeedServer{
		svc:       svc,
		meta:      meta,
		providers: make(map[string]audioSourceProvider),
	}
}

func (srv *FeedServer) ServeMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/", srv.ServeIndex)
	mux.HandleFunc("/add/", srv.HandleAddItem)
	mux.HandleFunc("/feed", srv.ServeFeed)
	mux.HandleFunc("/favicon.ico", srv.ServeIcon)
	//mux.Handle("/downloads/", http.StripPrefix("/downloads/", http.FileServer(http.Dir(srv.svc.storagePath))))
	mux.HandleFunc("/downloads/", srv.ServeMedia)

	return mux
}

func (srv *FeedServer) RegisterProvider(subPath string, p audioSourceProvider) {
	log.Printf("requests sent to /add%s will be handled by %s provider", subPath, p.Name())
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
        <a class="btn" href="javascript:(function(){window.location='{{ .Scheme }}://{{ .Host }}/add/yt?url='+encodeURIComponent(window.location);})();">Listen later</a>
      </div>
      <div class="row">
        Click it while on YouTube video page to add its audio version to your personal podcast.
      </div>
      <div class="row">
        And by the way, here is a button to subscribe to it. In case it did not work, use this link: <code class="language-markup">{{ .Scheme }}://{{ .Host }}/feed</code>.
      </div>
      <div class="row">
        <a class="waves-effect waves-light red btn" href="podcast://{{ .Host }}/feed"><i class="material-icons left">rss_feed</i>Subscribe</a>
      </div>
    </div>
    <script type="text/javascript" src="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/js/materialize.min.js"></script>
  </body>
</html>
`))

func (srv *FeedServer) ServeIndex(w http.ResponseWriter, req *http.Request) {
	if err := indexTemplate.Execute(w, struct {
		Scheme, Host, Title string
	}{reqScheme(req), req.Host, srv.meta.Title}); err != nil {
		log.Printf("failed to render index template: %s", err)
	}
}

func (srv *FeedServer) ServeFeed(w http.ResponseWriter, req *http.Request) {
	items, err := srv.svc.Items()
	if err != nil {
		log.Println("failed to fetch podcast items: ", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	var pubDate *time.Time
	if len(items) > 0 {
		pubDate = &items[len(items)-1].AddedAt
	}

	scheme := reqScheme(req)

	feedLink := srv.meta.Link
	if feedLink == "" {
		feedLink = scheme + "://" + req.Host
	}

	p := podcast.New(srv.meta.Title, feedLink, srv.meta.Description, pubDate, nil)
	p.AddImage(scheme + "://" + req.Host + "/favicon.ico")

	for _, it := range items {
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

		u := it.MediaURL
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			u = scheme + "://" + req.Host + it.MediaURL
		}

		item.AddEnclosure(u, mimeTypeToEnclosureType(it.MIMEType), int64(it.ContentLength))

		if _, err := p.AddItem(item); err != nil {
			log.Printf("failed to add %s: %s", it.OriginalURL, err)
		}
	}

	w.Header().Set("Content-Type", "application/xml")
	p.Encode(w)
}

func (srv *FeedServer) ServeIcon(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Write(assets.Icon)
}

func (srv *FeedServer) ServeMedia(w http.ResponseWriter, req *http.Request) {
	fileName := path.Base(req.URL.Path)
	filePath := path.Join(srv.svc.storagePath, fileName)

	fi, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}

		log.Printf("failed to stat %s: %s", filePath, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}

	fd, err := os.Open(filePath)
	if err != nil {
		log.Printf("failed to read %s: %s", filePath, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}
	defer fd.Close()

	mimeType := "application/octet-stream"
	if ind := strings.LastIndexByte(fileName, '.'); ind > -1 {
		if typ := mime.TypeByExtension(fileName[ind:]); typ != "" {
			mimeType = typ
		}
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Size", strconv.FormatInt(fi.Size(), 10))

	io.Copy(w, fd)
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

		u, err := audio.DownloadURL(ctx)
		if err != nil {
			log.Printf("failed to fetch download URL for %s: %s", p.Name(), err)
			return
		}

		if err := srv.svc.AddItem(NewPodcastItem(meta, time.Now()), u); err != nil {
			log.Printf("failed to add %s item to the feed: %s", p.Name(), err)
			return
		}
	}()
}

func reqScheme(req *http.Request) string {
	if req.TLS != nil {
		return "https"
	}

	return "http"
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
