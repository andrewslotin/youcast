package main

import (
	"context"
	"fmt"
	"io"
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

	if err := w.resetStaleJobs(ctx); err != nil {
		log.Printf("failed to reset stale jobs: %s", err)
	}

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
				go w.handleFileDownload(ctx, job)
			case StatusDownloaded:
				go w.handleFileConversion(ctx, job)
			case StatusFailed:
				go w.handleDownloadFailure(ctx, job)
			default:
				log.Printf("unexpected job status %q (job id %s)", job.Status, job.ItemID)
				continue
			}
		}
	}
}

func (w *DownloadWorker) resetStaleJobs(ctx context.Context) error {
	log.Println("resetting stale jobs")

	jobs, err := w.q.All()
	if err != nil {
		return fmt.Errorf("failed to fetch jobs: %w", err)
	}

	for _, job := range jobs {
		if err := w.q.Update(job); err != nil {
			return fmt.Errorf("failed to reset job %s: %w", job.ItemID, err)
		}
	}

	log.Printf("reset %d stale jobs", len(jobs))

	return nil
}

func (w *DownloadWorker) handleFileDownload(ctx context.Context, job DownloadJob) {
	defer func() {
		if err := w.q.Update(job); err != nil {
			log.Printf("failed to update job status to %s (job id %s): %s", job.ItemID, job.Status, err)
		}
	}()

	newItemStatus := ItemDownloaded
	job.Status = StatusDownloaded

	if err := w.downloadFile(ctx, job.SourceURI, job.TargetURI); err != nil {
		log.Printf("failed to download %s: %s", job.SourceURI, err)
		newItemStatus = ItemDownloadFailed
		job.Status = StatusFailed
	}

	if _, err := w.st.UpdateStatus(job.ItemID, newItemStatus); err != nil {
		if err != ErrItemNotFound {
			log.Printf("failed to update podcast item status for %s: %s", job.ItemID, err)
			job.Status = StatusFailed
			return
		}

		log.Printf("podcast item %s was deleted, cancelling job", job.ItemID)
		job.Status = StatusCancelled // item was deleted, cancel job
	}
}

func (w *DownloadWorker) downloadFile(ctx context.Context, sourceURL, destinationPath string) error {
	log.Printf("downloading %s", sourceURL)

	tmpFile, written, err := w.c.DownloadFile(ctx, sourceURL)
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile)

	if err := moveFile(tmpFile, destinationPath); err != nil {
		return fmt.Errorf("failed to rename %s to %s: %w", tmpFile, destinationPath, err)
	}

	log.Printf("downloaded %s to %s (%s written)", sourceURL, destinationPath, FileSize(written))

	return nil
}

func (w *DownloadWorker) handleFileConversion(ctx context.Context, job DownloadJob) {
	defer func() {
		if err := w.q.Update(job); err != nil {
			log.Printf("failed to update job status to %s (job id %s): %s", job.ItemID, job.Status, err)
			return
		}
	}()

	newItemStatus := ItemReady
	job.Status = StatusReady

	if err := w.convertFile(ctx, job.TargetURI); err != nil {
		log.Printf("failed to convert %s: %s", job.TargetURI, err)
		job.Status = StatusFailed
		newItemStatus = ItemDownloadFailed
	}

	if _, err := w.st.UpdateStatus(job.ItemID, newItemStatus); err != nil {
		if err != ErrItemNotFound {
			log.Printf("failed to update podcast item status for %s: %s", job.ItemID, err)
			job.Status = StatusFailed
			return
		}

		log.Printf("podcast item %s was deleted, cancelling job", job.ItemID)
		job.Status = StatusCancelled // item was deleted, cancel job
	}
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

func (w *DownloadWorker) handleDownloadFailure(ctx context.Context, job DownloadJob) {
	if _, err := w.st.UpdateStatus(job.ItemID, ItemDownloadFailed); err == ErrItemNotFound {
		log.Printf("podcast item %s was deleted, cancelling job", job.ItemID)

		job.Status = StatusCancelled
		if err := w.q.Update(job); err != nil {
			log.Printf("failed to update job status to %s (job id %s): %s", job.ItemID, job.Status, err)
			return
		}

		return
	}

	log.Printf("ignoring failed job %s", job.ItemID)
}

// moveFile moves a file from srcPath to destPath even if these path are on different filesystems.
func moveFile(srcPath, destPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", srcPath, err)
	}
	defer src.Close()

	dest, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", destPath, err)
	}
	defer dest.Close()

	if _, err := io.Copy(dest, src); err != nil {
		return fmt.Errorf("failed to copy %s to %s: %w", srcPath, destPath, err)
	}

	if err := os.Remove(srcPath); err != nil {
		return fmt.Errorf("failed to remove %s: %w", srcPath, err)
	}

	return nil
}
