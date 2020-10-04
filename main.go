package main

import (
	"flag"
	"log"
	"net/http"
	"os"
)

var args struct {
	ListenAddr string
}

func main() {
	flag.StringVar(&args.ListenAddr, "l", os.Getenv("LISTEN_ADDR"), "Listen address")
	flag.Parse()

	if args.ListenAddr == "" {
		log.Fatalln("missing listen address")
	}

	srv := NewFeedServer(PodcastMetadata{
		Title:       "LaterTube",
		Link:        "http://localhost:5000",
		Description: "YouTube audio as a podcast",
	}, &memoryStorage{})

	http.HandleFunc("/", srv.HandleAddItem)
	http.HandleFunc("/feed", srv.ServeFeed)
	http.HandleFunc("/audio/youtube", srv.ServeYoutubeAudio)

	log.Println("starting server on ", args.ListenAddr, "...")
	if err := http.ListenAndServe(args.ListenAddr, nil); err != nil {
		log.Fatalln(err)
	}
}
