package main

import (
	"encoding/json"
	"errors"

	"github.com/boltdb/bolt"
)

// ErrNoInactiveJobs is returned when there are no inactive jobs in the queue.
var ErrNoInactiveJobs = errors.New("no inactive jobs")

// DownloadStatus represents a status of a download job.
type DownloadStatus uint8

// Download job statuses.
const (
	StatusAdded DownloadStatus = iota + 1
	StatusDownloaded
	StatusReady
)

// String returns a string representation of the status.
func (st DownloadStatus) String() string {
	switch st {
	case StatusAdded:
		return "added"
	case StatusDownloaded:
		return "downloaded"
	case StatusReady:
		return "ready"
	default:
		return "unknown"
	}
}

// DownloadJob represents a job to be performed on a podcast item.
type DownloadJob struct {
	ItemID    string
	Status    DownloadStatus
	SourceURI string
	TargetURI string
}

// NewDownloadJob returns a new instance of DownloadJob.
func NewDownloadJob(itemID, sourceURI, targetURI string) DownloadJob {
	return DownloadJob{
		ItemID:    itemID,
		Status:    StatusAdded,
		SourceURI: sourceURI,
		TargetURI: targetURI,
	}
}

// DownloadJobQueue is a queue of download jobs that allows adding, updating and getting jobs.
type DownloadJobQueue struct {
	db *bolt.DB
}

// NewDownloadJobQueue returns a new instance of Queue.
func NewDownloadJobQueue(db *bolt.DB) *DownloadJobQueue {
	return &DownloadJobQueue{db: db}
}

type boltJob struct {
	Status    DownloadStatus `json:",omitempty"`
	SourceURI string         `json:",omitempty"`
	TargetURI string         `json:",omitempty"`
	Active    bool           `json:",omitempty"`
}

func newBoltJob(job DownloadJob) boltJob {
	return boltJob{
		Status:    job.Status,
		SourceURI: job.SourceURI,
		TargetURI: job.TargetURI,
	}
}

func (j boltJob) MarshalBinary() []byte {
	b, _ := json.Marshal(j)
	return b
}

// Push adds a value to the end of the queue.
func (q *DownloadJobQueue) Add(job DownloadJob) error {
	return q.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("downloads"))
		if err != nil {
			return err
		}

		return b.Put([]byte(job.ItemID), newBoltJob(job).MarshalBinary())
	})
}

// Next returns the next inactive job in the queue.
func (q *DownloadJobQueue) Next() (DownloadJob, error) {
	var job DownloadJob

	err := q.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("downloads"))
		if err != nil {
			return err
		}

		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var j boltJob
			if err := json.Unmarshal(v, &j); err != nil {
				return err
			}

			if j.Active {
				continue
			}

			job = DownloadJob{
				ItemID:    string(k),
				Status:    j.Status,
				SourceURI: j.SourceURI,
				TargetURI: j.TargetURI,
			}

			j.Active = true

			return b.Put(k, j.MarshalBinary())
		}

		return ErrNoInactiveJobs
	})

	return job, err
}

// Update updates the job in the queue resetting its active status. It deletes any completed jobs.
func (q *DownloadJobQueue) Update(job DownloadJob) error {
	return q.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("downloads"))
		if err != nil {
			return err
		}

		if job.Status == StatusReady {
			return b.Delete([]byte(job.ItemID))
		}

		return b.Put([]byte(job.ItemID), newBoltJob(job).MarshalBinary())
	})
}

// All returns all jobs in the queue.
func (q *DownloadJobQueue) All() ([]DownloadJob, error) {
	var jobs []DownloadJob

	err := q.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("downloads"))
		if b == nil {
			return nil
		}

		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var j boltJob
			if err := json.Unmarshal(v, &j); err != nil {
				return err
			}

			jobs = append(jobs, DownloadJob{
				ItemID:    string(k),
				Status:    j.Status,
				SourceURI: j.SourceURI,
				TargetURI: j.TargetURI,
			})
		}

		return nil
	})

	return jobs, err
}
