package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
)

// HTTPDownloader is a service that downloads files via HTTP.
type HTTPDownloader struct {
	tmpDir string
	c      *http.Client
}

// NewHTTPDownloader creates a new HTTPDownloader instance.
func NewHTTPDownloader(tmpDir string, c *http.Client) *HTTPDownloader {
	if c == nil {
		c = http.DefaultClient
	}

	return &HTTPDownloader{
		tmpDir: tmpDir,
		c:      c,
	}
}

// DownloadFile downloads a file from the given URL.
func (svc *HTTPDownloader) DownloadFile(ctx context.Context, u string) (string, int64, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", 0, fmt.Errorf("failed to build a request to %s: %w", u, err)
	}

	resp, err := svc.c.Do(req.WithContext(ctx))
	if err != nil {
		return "", 0, fmt.Errorf("failed to build a request to %s: %w", u, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return "", 0, fmt.Errorf("failed to download %s: server responded with %s", u, resp.Status)
	}

	fd, err := os.CreateTemp(svc.tmpDir, "youcast*")
	if err != nil {
		return "", 0, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer fd.Close()

	written, err := io.Copy(fd, resp.Body)
	if err != nil {
		return "", written, fmt.Errorf("failed to download %s to %s: %w", u, fd.Name(), err)
	}

	return fd.Name(), written, nil
}
