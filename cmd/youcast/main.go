package main

import (
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/andrewslotin/youcast"
)

func main() {
	if len(os.Args) == 1 {
		fmt.Fprintln(os.Stderr, "no video IDs provided")
		fmt.Fprintln(os.Stderr, "Usage: ", os.Args[0], " videoID1[,...]")
		os.Exit(2)
	}

	var wg sync.WaitGroup
	wg.Add(len(os.Args) - 1)

	for _, videoID := range os.Args[1:] {
		go func(videoID string) {
			defer wg.Done()

			if err := youcast.DownloadAudio(videoID); err != nil {
				log.Printf("failed to download %s: %s", videoID, err)
			}
		}(videoID)
	}

	wg.Wait()
}
