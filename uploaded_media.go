package main

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path"
)

type UploadedMediaProvider struct {
	cachePath       string
	uploadServerURL string
}

func NewUploadedMediaProvider() *UploadedMediaProvider {
	tmp := path.Join(os.TempDir(), "youcast")
	if err := os.MkdirAll(tmp, os.ModePerm); err != nil && !os.IsExist(err) {
		log.Fatalf("failed to create temporary directory %s: %s", tmp, err)
	}

	u, err := startUploadedMediaServer(tmp)
	if err != nil {
		log.Fatalf("failed to start upload server: %s", err)
	}

	return &UploadedMediaProvider{
		cachePath:       tmp,
		uploadServerURL: u,
	}
}

func (*UploadedMediaProvider) Name() string {
	return "User media"
}

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

	if _, err := io.Copy(tmpFd, fd); err != nil {
		log.Printf("failed to copy uploaded file to %s: %s", tmpPath, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return nil
	}

	if err := tmpFd.Close(); err != nil {
		log.Printf("failed to store uploaded file to %s: %s", tmpPath, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return nil
	}

	log.Printf("stored uploaded file to %s", tmpPath)

	return UploadedMedia{
		FileName:    header.Filename,
		MIMEType:    header.Header.Get("Content-Type"),
		downloadURL: p.uploadServerURL + "/" + header.Filename,
	}
}

type UploadedMedia struct {
	FileName string
	MIMEType string

	downloadURL string
}

func (m UploadedMedia) Metadata(ctx context.Context) (Metadata, error) {
	return Metadata{
		Type:     UploadedItem,
		Title:    m.FileName,
		Author:   "Uploaded media",
		MIMEType: m.MIMEType,
	}, nil
}

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
