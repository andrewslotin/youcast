package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
)

type fileDownloader interface {
	DownloadFile(context.Context, string) (string, int64, error)
}

type mediaTranscoder interface {
	TranscodeMedia(context.Context, string) (int64, error)
}

type statusUpdater interface {
	UpdateStatus(string, Status) (PodcastItem, error)
}

// DownloadWorker is a worker that monitors the download job queue and executes download jobs.
type DownloadWorker struct {
	q         *DownloadJobQueue
	st        statusUpdater
	c         fileDownloader
	converter mediaTranscoder
}

// NewDownloadWorker returns a new instance of DownloadWorker.
func NewDownloadWorker(
	q *DownloadJobQueue,
	st statusUpdater,
	c fileDownloader,
	converter mediaTranscoder,
) *DownloadWorker {
	return &DownloadWorker{q: q, st: st, c: c, converter: converter}
}

func (w *DownloadWorker) Run(ctx context.Context, pollDuration time.Duration) {
	log.Printf("starting download worker with poll duration %s", pollDuration)
	defer log.Print("download worker stopped")

	c := time.Tick(pollDuration)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c:
			job, err := w.q.Next()
			if err != nil {
				if err != ErrNoInactiveJobs {
					log.Printf("failed to get next job: %v", err)
				}

				continue
			}

			switch job.Status {
			case StatusAdded:
				if err := w.downloadFile(ctx, job.SourceURI, job.TargetURI); err != nil {
					log.Printf("failed to download %s: %s", job.SourceURI, err)
					continue
				}

				if _, err := w.st.UpdateStatus(job.ItemID, ItemDownloaded); err != nil {
					log.Printf("failed to update podcast item status for %s: %s", job.ItemID, err)
					continue
				}

				job.Status = StatusDownloaded
				if err := w.q.Update(job); err != nil {
					log.Printf("failed to update job status to %s (job id %s): %s", job.ItemID, job.Status, err)
					continue
				}
			case StatusDownloaded:
				if err := w.convertFile(ctx, job.TargetURI); err != nil {
					log.Printf("failed to convert %s: %s", job.TargetURI, err)
					continue
				}

				if _, err := w.st.UpdateStatus(job.ItemID, ItemReady); err != nil {
					log.Printf("failed to update podcast item status for %s: %s", job.ItemID, err)
					continue
				}

				job.Status = StatusReady
				if err := w.q.Update(job); err != nil {
					log.Printf("failed to update job status to %s (job id %s): %s", job.ItemID, job.Status, err)
					continue
				}
			default:
				log.Printf("unexpected job status %q (job id %s)", job.Status, job.ItemID)
				continue
			}
		}
	}
}

func (w *DownloadWorker) downloadFile(ctx context.Context, sourceURL, destinationPath string) error {
	log.Printf("downloading %s", sourceURL)

	tmpFile, written, err := w.c.DownloadFile(ctx, sourceURL)
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile) // in case it still exists

	if err := os.Rename(tmpFile, destinationPath); err != nil {
		return fmt.Errorf("failed to rename %s to %s: %w", tmpFile, destinationPath, err)
	}

	log.Printf("downloaded %s to %s (%s written)", sourceURL, destinationPath, FileSize(written))

	return nil
}

func (w *DownloadWorker) convertFile(ctx context.Context, filePath string) error {
	log.Println("transcoding", filePath)

	transcodedSize, err := w.converter.TranscodeMedia(ctx, filePath)
	if err != nil {
		return fmt.Errorf("failed to transcode file: %w", err)
	}

	log.Printf("transcoded %s (new size %s)", filePath, FileSize(transcodedSize))

	return nil
}
