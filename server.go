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

var indexTemplate = template.Must(template.New("index").Parse(`<!DOCTYPE html>
<html>

<head>
    <title>{{ .Title }} - listen videos later</title>
    <link href="https://fonts.googleapis.com/icon?family=Material+Icons" rel="stylesheet">
    <link type="text/css" rel="stylesheet"
        href="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/css/materialize.min.css"
        media="screen,projection" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
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
            <a class="btn"
                href="javascript:(function(){window.location='{{ .Scheme }}://{{ .Host }}/add/yt?url='+encodeURIComponent(window.location);})();">Listen
                later</a>
        </div>
        <div class="row">
            Click it while on YouTube video page to add its audio version to your personal podcast.
        </div>
        <div class="row">
            And by the way, here is a button to subscribe to it. In case it did not work, use this link: <code
                class="language-markup">{{ .Scheme }}://{{ .Host }}/feed</code>.
        </div>
        <div class="row">
            <a class="waves-effect waves-light red btn" href="podcast://{{ .Host }}/feed"><i
                    class="material-icons left">rss_feed</i>Subscribe</a>
        </div>
        {{ if .Feed }}
        <div class="row">
            <h2>Feed</h2>
        </div>
        <div class="row">
            <ul class="collection">
                {{ range .Feed }}
                <li class="collection-item avatar">
                    <i class="material-icons circle red">Play</i>
                    <span class="title">{{ .Title }}</span>
                    <p>
                        <em>added on {{ .AddedAt.Format "2006-01-02" }}</em>
                    </p>
                    {{ if .Description }}
                    <p>
                        {{ .Description }}
                    </p>
                    {{ end }}
                </li>
                {{ end }}
            </ul>
        </div>
        {{ end }}
    </div>
    <script type="text/javascript"
        src="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/js/materialize.min.js"></script>
</body>

</html>`))

type indexTemplateArgs struct {
	Scheme, Host, Title string
	Feed                []PodcastItem
}

func (srv *FeedServer) ServeIndex(w http.ResponseWriter, req *http.Request) {
	if err := indexTemplate.Execute(w, indexTemplateArgs{
		Scheme: reqScheme(req),
		Host:   req.Host,
		Title:  srv.meta.Title,
	}); err != nil {
		log.Printf("failed to render index template: %s", err)
	}
}

func (srv *FeedServer) ServeFeed(w http.ResponseWriter, req *http.Request) {
	scheme := reqScheme(req)

	feed := Feed{
		URL:         srv.meta.Link,
		IconURL:     scheme + "://" + req.Host + "/favicon.ico",
		Title:       srv.meta.Title,
		Description: srv.meta.Description,
	}

	if feed.URL == "" {
		feed.URL = scheme + "://" + req.Host
	}

	items, err := srv.svc.Items()
	if err != nil {
		log.Println("failed to fetch podcast items: ", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	for _, item := range items {
		if !strings.HasPrefix(item.MediaURL, "http://") && !strings.HasPrefix(item.MediaURL, "https://") {
			item.MediaURL = scheme + "://" + req.Host + item.MediaURL
		}

		feed.Items = append(feed.Items, item)
	}

	if len(items) > 0 {
		feed.PubDate = items[len(items)-1].AddedAt
	}

	var view interface {
		ContentType() string
		Render(io.Writer, Feed) error
	}

	view = AtomRenderer{}

	w.Header().Set("Content-Type", view.ContentType())
	if err := view.Render(w, feed); err != nil {
		log.Println("failed to render feed to", view.ContentType(), ":", err)
	}
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
			mimeType = mimeTypeToEnclosureType(typ).String()
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
	case "audio/mp4", "audio/mp4a.20.2", "audio/x-m4a":
		return podcast.M4A
	case "video/mp4", "video/x-m4v":
		return podcast.M4V
	case "audio/mpeg":
		return podcast.MP3
	default:
		log.Printf("unknown MIME type %s, falling back to mp3", mime)
		return podcast.MP3
	}
}
