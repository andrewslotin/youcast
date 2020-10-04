package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"

	"github.com/andrewslotin/youcast"
)

func main() {
	if len(os.Args) == 1 {
		fmt.Fprintln(os.Stderr, "no video IDs provided")
		fmt.Fprintln(os.Stderr, "Usage: ", os.Args[0], " videoID1[,...]")
		os.Exit(2)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, os.Kill)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-sigs
		cancel()
	}()

	var wg sync.WaitGroup
	wg.Add(len(os.Args) - 1)

	for _, videoID := range os.Args[1:] {
		go func(videoID string) {
			defer wg.Done()

			yt := youcast.NewYouTubeVideo(videoID)
			u, _, err := yt.AudioStreamURL(ctx)
			if err != nil {
				log.Printf("failed to fetch audio URL for %s: %s", videoID, err)
				return
			}
			fmt.Println(u)
		}(videoID)
	}

	wg.Wait()
}
