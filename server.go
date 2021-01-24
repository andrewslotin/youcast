package main

import (
	"context"
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

	mux.HandleFunc("/", srv.ServeFeed)
	mux.HandleFunc("/add/", srv.HandleAddItem)
	mux.HandleFunc("/feed", srv.ServeFeed)
	mux.HandleFunc("/favicon.ico", srv.ServeIcon)
	mux.HandleFunc("/downloads/", srv.ServeMedia)

	return mux
}

func (srv *FeedServer) RegisterProvider(subPath string, p audioSourceProvider) {
	log.Printf("requests sent to /add%s will be handled by %s provider", subPath, p.Name())
	srv.providers[subPath] = p
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

	switch req.URL.Path {
	case "/feed":
		view = AtomRenderer{}
	default:
		view = HTMLRenderer{}
	}

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
