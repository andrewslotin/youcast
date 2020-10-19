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

	http.HandleFunc("/", srv.HandleAddItem)
	http.HandleFunc("/feed", srv.ServeFeed)
	http.HandleFunc("/audio/youtube", srv.ServeYoutubeAudio)
	http.HandleFunc("/favicon.ico", srv.ServeIcon)

	log.Println("starting server on ", args.ListenAddr, "...")
	if err := http.ListenAndServe(args.ListenAddr, nil); err != nil {
		log.Fatalln(err)
	}
}
