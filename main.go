package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/boltdb/bolt"
)

const DefaultDBPath = "feed.db"

var args struct {
	ListenAddr string
	DBPath     string
}

func main() {
	flag.StringVar(&args.ListenAddr, "l", os.Getenv("LISTEN_ADDR"), "Listen address")
	flag.StringVar(&args.DBPath, "db", os.Getenv("DB_PATH"), "Path to the database")
	flag.Parse()

	if p, ok := os.LookupEnv("PORT"); ok {
		args.ListenAddr = ":" + p
	}

	if args.ListenAddr == "" {
		log.Fatalln("missing listen address")
	}

	if args.DBPath == "" {
		log.Println("missing DB_PATH, using", DefaultDBPath, "as a default")
		args.DBPath = DefaultDBPath
	}

	db, err := bolt.Open(args.DBPath, 0600, nil)
	if err != nil {
		log.Fatalln("failed to open BoltDB file ", args.DBPath, " :", err)
	}

	srv := NewFeedServer(PodcastMetadata{
		Title:       "Listen Later",
		Description: "These videos could have been a podcast...",
	}, newBoltStorage("feed", db))

	srv.RegisterProvider("/yt", &YouTubeProvider{})

	if token, ok := os.LookupEnv("TELEGRAM_API_TOKEN"); ok {
		p, err := NewTelegramProvider(token, os.Getenv("TELEGRAM_API_ENDPOINT"))
		if err != nil {
			log.Printf("failed to initialize telegram provider: %s", err)
		} else {
			srv.RegisterProvider("/tg", p)
		}
	}

	log.Println("starting server on ", args.ListenAddr, "...")
	if err := http.ListenAndServe(args.ListenAddr, srv.ServeMux()); err != nil {
		log.Fatalln(err)
	}
}
