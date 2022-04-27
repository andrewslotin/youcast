package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/boltdb/bolt"
)

const DefaultDBPath = "feed.db"

var args struct {
	ListenAddr  string
	DBPath      string
	StoragePath string
	DevMode     bool
}

func main() {
	log.Println("YouCast version", Version)

	flag.StringVar(&args.ListenAddr, "l", os.Getenv("LISTEN_ADDR"), "Listen address")
	flag.StringVar(&args.DBPath, "db", os.Getenv("DB_PATH"), "Path to the database")
	flag.StringVar(&args.StoragePath, "storage-dir", os.Getenv("STORAGE_PATH"), "Path to the directory where to store downloaded files")
	flag.BoolVar(&args.DevMode, "dev", false, "Development mode (read assets from ./assets on each request)")
	flag.Parse()

	if p, ok := os.LookupEnv("PORT"); ok {
		args.ListenAddr = ":" + p
	}

	if args.ListenAddr == "" {
		log.Fatalln("missing LISTEN_ADDR")
	}

	if args.DBPath == "" {
		log.Println("missing DB_PATH, using", DefaultDBPath, "as a default")
		args.DBPath = DefaultDBPath
	}

	if args.StoragePath == "" {
		log.Fatalln("missing STORAGE_PATH")
	}

	db, err := bolt.Open(args.DBPath, 0600, nil)
	if err != nil {
		log.Fatalln("failed to open BoltDB file ", args.DBPath, " :", err)
	}

	svc := NewFeedService(newBoltStorage("feed", db), args.StoragePath, nil)
	srv := NewFeedServer(PodcastMetadata{
		Title:       "Listen Later",
		Description: "These videos could have been a podcast...",
	}, svc)

	srv.RegisterProvider("/yt", &YouTubeProvider{})

	cachePath := path.Join(os.TempDir(), "youcast")
	if err := os.MkdirAll(cachePath, os.ModePerm); err != nil && !os.IsExist(err) {
		log.Fatalf("failed to create temporary directory %s: %s", cachePath, err)
	}
	srv.RegisterProvider("/my", NewUploadedMediaProvider(cachePath))

	if token, ok := os.LookupEnv("TELEGRAM_API_TOKEN"); ok {
		p, err := NewTelegramProvider(token, os.Getenv("TELEGRAM_API_ENDPOINT"), os.Getenv("TELEGRAM_FILE_SERVER"))
		if err != nil {
			log.Printf("failed to initialize telegram provider: %s", err)
		} else {
			srv.RegisterProvider("/tg", p)

			for _, idStr := range strings.Split(os.Getenv("TELEGRAM_ALLOWED_USERS"), ",") {
				id, err := strconv.Atoi(strings.TrimSpace(idStr))
				if err != nil {
					log.Printf("failed to whitelist user with id '%s': %s", idStr, err)
					continue
				}

				p.WhitelistUser(id)
			}

			tgUpdates, err := p.Updates(context.Background())
			if err != nil {
				log.Printf("failed to start telegram updates consumption loop: %s", err)
			} else {
				go func() {
					for audio := range tgUpdates {
						meta, err := audio.Metadata(context.Background())
						if err != nil {
							log.Printf("failed to fetch %s data: %s", p.Name(), err)
							continue
						}

						u, err := audio.DownloadURL(context.Background())
						if err != nil {
							log.Printf("failed to fetch download URL for %s: %s", p.Name(), err)
							continue
						}

						if err := svc.AddItem(NewPodcastItem(meta, time.Now()), u); err != nil {
							log.Printf("failed to add %s item to the feed: %s", p.Name(), err)
							continue
						}
					}
				}()
			}
		}
	}

	log.Println("starting server on", args.ListenAddr, "...")
	if err := http.ListenAndServe(args.ListenAddr, CORSMiddleware(srv.ServeMux())); err != nil {
		log.Fatalln(err)
	}
}
