package main

import (
	"log"
	"mime"
)

var mimeTypes = map[string]string{
	"audio/mpeg": ".mp3",
	"audio/mp4":  ".m4a",
}

func init() {
	for typ, ext := range mimeTypes {
		if err := mime.AddExtensionType(ext, typ); err != nil {
			log.Printf("failed to associate %s with %s: %s", typ, ext, err)
		}
	}
}
