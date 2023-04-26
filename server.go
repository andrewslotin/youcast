package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/andrewslotin/youcast/assets"
	"github.com/eduncan911/podcast"
)

// PodcastMetadata contains metadata for the podcast feed.
type PodcastMetadata struct {
	Title       string
	Link        string
	Description string
}

// Metadata contains metadata for a podcast item
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

// FeedServer is an HTTP server that serves podcast feeds and manages podcast items.
type FeedServer struct {
	svc       *FeedService
	meta      PodcastMetadata
	providers map[string]audioSourceProvider
}

// NewFeedServer creates a new FeedServer instance.
func NewFeedServer(meta PodcastMetadata, svc *FeedService) *FeedServer {
	return &FeedServer{
		svc:       svc,
		meta:      meta,
		providers: make(map[string]audioSourceProvider),
	}
}

// ServeMux returns a ServeMux instance that can be used to serve the podcast feed.
func (srv *FeedServer) ServeMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/", srv.ServeFeed)
	mux.HandleFunc("/add/", srv.HandleAddItem)
	mux.HandleFunc("/feed", srv.ServeFeed)
	mux.HandleFunc("/feed/", srv.HandleItem)
	mux.HandleFunc("/favicon.ico", srv.ServeIcon)
	mux.HandleFunc("/style.css", srv.ServeStylesheet)
	mux.HandleFunc("/downloads/", srv.ServeMedia)

	return mux
}

// RegisterProvider registers a new audio source provider.
func (srv *FeedServer) RegisterProvider(subPath string, p audioSourceProvider) {
	log.Printf("requests sent to /add%s will be handled by %s provider", subPath, p.Name())
	srv.providers[subPath] = p
}

// ServeFeed serves the podcast feed.
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

	switch req.URL.Path {
	case "/feed":
		view = AtomRenderer{}
	default:
		tmpl := Templates
		if args.DevMode {
			tmpl = ParseTemplates(os.DirFS("./assets"))
		}
		view = HTMLRenderer{
			Template: tmpl.Lookup("index.html.tmpl"),
		}
	}

	w.Header().Set("Content-Type", view.ContentType())
	if err := view.Render(w, feed); err != nil {
		log.Println("failed to render feed to", view.ContentType(), ":", err)
	}
}

// ServeIcon serves the favicon.ico file for the podcast feed.
func (srv *FeedServer) ServeIcon(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Write(assets.Icon)
}

func (srv *FeedServer) ServeStylesheet(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/css")
	w.Write(assets.Stylesheet)
}

// ServeMedia serves the podcast media files.
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

	http.ServeContent(w, req, fileName, fi.ModTime(), fd)
}

// HandleItem handles requests to add a new podcast item.
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

// HandleItem handles requests to update or remove a podcast item.
func (srv *FeedServer) HandleItem(w http.ResponseWriter, req *http.Request) {
	switch {
	case req.Method == http.MethodDelete:
		fallthrough
	case req.Method == http.MethodPost && strings.ToLower(req.FormValue("action")) == "delete":
		srv.HandleRemoveItem(w, req)
	case req.Method == http.MethodPatch:
		fallthrough
	case req.Method == http.MethodPost && strings.ToLower(req.FormValue("action")) == "patch":
		srv.HandleUpdateItem(w, req)
	}
}

// HandleRemoveItem handles requests to remove a podcast item.
func (srv *FeedServer) HandleRemoveItem(w http.ResponseWriter, req *http.Request) {
	itemID := req.URL.Path[strings.LastIndexByte(req.URL.Path, '/')+1:]
	if err := srv.svc.RemoveItem(itemID); err != nil {
		log.Println("failed to remove podcast item", itemID, ":", err)
	}

	http.Redirect(w, req, req.Referer(), http.StatusSeeOther)
}

// HandleUpdateItem handles requests to update a podcast item.
func (srv *FeedServer) HandleUpdateItem(w http.ResponseWriter, req *http.Request) {
	itemID := req.URL.Path[strings.LastIndexByte(req.URL.Path, '/')+1:]

	desc := Description{
		Title: strings.TrimSpace(req.FormValue("title")),
		Body:  strings.TrimSpace(req.FormValue("description")),
	}

	if desc.Title == "" {
		http.Error(w, "Missing title", http.StatusBadRequest)
		return
	}

	if err := srv.svc.UpdateItem(itemID, desc); err != nil {
		log.Println("failed to update podcast item", itemID, ":", err)
	}

	http.Redirect(w, req, req.Referer(), http.StatusSeeOther)
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
