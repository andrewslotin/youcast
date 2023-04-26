package main

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/dhowden/tag"
)

// UploadedMediaProvider is an audio source provider that handles uploaded media files.
type UploadedMediaProvider struct {
	cachePath       string
	uploadServerURL string
}

// NewUploadedMediaProvider creates a new UploadedMediaProvider instance.
func NewUploadedMediaProvider(cachePath string) *UploadedMediaProvider {
	u, err := startUploadedMediaServer(cachePath)
	if err != nil {
		log.Fatalf("failed to start upload server: %s", err)
	}

	return &UploadedMediaProvider{
		cachePath:       cachePath,
		uploadServerURL: u,
	}
}

// Name returns the name of the provider.
func (*UploadedMediaProvider) Name() string {
	return "User media"
}

// HandleRequest handles an HTTP request and returns an audio source.
func (p *UploadedMediaProvider) HandleRequest(w http.ResponseWriter, req *http.Request) audioSource {
	fd, header, err := req.FormFile("media")
	if err != nil {
		log.Printf("failed to read uploaded file: %s", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return nil
	}

	tmpPath := path.Join(p.cachePath, header.Filename)

	tmpFd, err := os.Create(tmpPath)
	if err != nil {
		log.Printf("failed to create temp file %s: %s", tmpPath, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return nil
	}
	defer tmpFd.Close()

	if _, err := io.Copy(tmpFd, fd); err != nil {
		log.Printf("failed to copy uploaded file to %s: %s", tmpPath, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return nil
	}

	if err := tmpFd.Sync(); err != nil {
		log.Printf("failed to store uploaded file to %s: %s", tmpPath, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return nil
	}

	log.Printf("stored uploaded file to %s", tmpPath)

	http.Redirect(w, req, req.Referer(), http.StatusSeeOther)

	meta := UploadedMedia{
		FileName:    header.Filename,
		Title:       header.Filename,
		MIMEType:    header.Header.Get("Content-Type"),
		downloadURL: p.uploadServerURL + "/" + header.Filename,
	}

	tmpFd.Seek(0, 0)
	if m, err := tag.ReadFrom(tmpFd); err == nil {
		if a := m.Artist(); a != "" {
			meta.Author = a
		} else if a = m.AlbumArtist(); a != "" {
			meta.Author = a
		}

		if t := m.Title(); t != "" {
			meta.Title = m.Title()
		}

		if t := m.Album(); t != "" {
			meta.Title = t + ": " + meta.Title
		}
	} else {
		log.Printf("failed to read uploaded file metadata: %s", err)
	}

	return meta
}

// UploadedMedia is an audio source that represents an uploaded media file.
type UploadedMedia struct {
	Author   string
	Title    string
	Duration time.Duration
	FileName string
	MIMEType string

	downloadURL string
}

// Metadata returns the metadata of the audio source.
func (m UploadedMedia) Metadata(ctx context.Context) (Metadata, error) {
	return Metadata{
		Type:        UploadedItem,
		Author:      m.Author,
		Title:       m.Title,
		Description: m.Title,
		Duration:    m.Duration,
		MIMEType:    m.MIMEType,
	}, nil
}

// DownloadURL returns the download URL of the audio source.
func (m UploadedMedia) DownloadURL(ctx context.Context) (string, error) {
	return m.downloadURL, nil
}

func startUploadedMediaServer(storagePath string) (string, error) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return "", err
	}

	h := http.FileServer(http.Dir(storagePath))

	go func() {
		http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			defer func() {
				p := path.Join(storagePath, req.URL.Path)
				if err := os.Remove(p); err != nil {
					log.Printf("failed to remove uploaded file %s: %s", p, err)
				}
			}()

			h.ServeHTTP(w, req)
		}))
	}()

	return "http://" + ln.Addr().String(), nil
}
