package main

import (
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/eduncan911/podcast"
)

type Feed struct {
	URL, IconURL       string
	Title, Description string
	PubDate            time.Time
	Items              []PodcastItem
}

var indexTemplate = template.Must(template.New("index").Funcs(template.FuncMap{
	"stripScheme": func(s string) string {
		if ind := strings.Index(s, "://"); ind > -1 {
			return s[ind+3:]
		}

		return s
	},
	"formatDuration": func(d time.Duration) string {
		d = d.Round(time.Second)

		var s string
		if d >= 1*time.Hour {
			s = strconv.Itoa(int(d/time.Hour)) + ":"
			d -= (d / time.Hour) * time.Hour
		}

		return s + fmt.Sprintf("%02d:%02d", int(d/time.Minute), int(d%time.Minute/time.Second))
	},
}).Parse(`<!DOCTYPE html>
<html>

<head>
    <title>{{ .Title }} - listen videos later</title>
    <link href="https://fonts.googleapis.com/icon?family=Material+Icons" rel="stylesheet">
    <link type="text/css" rel="stylesheet"
        href="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/css/materialize.min.css"
        media="screen,projection" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <style type="text/css">
    #playlist i.material-icons.circle {
        cursor: pointer;
    }

    #playlist audio {
        display: none;
	}
	
	#playlist .collection-item .title {
		font-weight: bold;
	}

	#playlist .collection-item .metadata,
	#playlist .collection-item .description {
		margin-top: 0.5em;
	}
    </style>
</head>

<body>
    <div class="container">
        <header>
            <h1>{{ .Title }}</h1>
        </header>
        <div class="row">
            Drag &amp; drop this bookmarklet to your favorites bar.
        </div>
        <div class="row">
            <a class="btn"
                href="javascript:(function(){window.location='{{ .URL }}/add/yt?url='+encodeURIComponent(window.location);})();">Listen
                later</a>
        </div>
        <div class="row">
            Click it while on YouTube video page to add its audio version to your personal podcast.
        </div>
        <div class="row">
            And by the way, here is a button to subscribe to it. In case it did not work, use this link: <code
                class="language-markup">{{ .URL }}/feed</code>.
        </div>
        <div class="row">
            <a class="waves-effect waves-light red btn" href="podcast://{{ .URL | stripScheme }}/feed"><i
                    class="material-icons left">rss_feed</i>Subscribe</a>
        </div>
        {{ if .Items }}
        <div class="row">
            <h2>Feed</h2>
        </div>
        <div class="row">
            <ul id="playlist" class="collection">
                {{ range $i, $item := .Items }}
                <li class="collection-item avatar">
                    <i id="audio-control-{{ $i }}" data-audio-id="audio-{{ $i }}" class="material-icons circle red">play_circle_filled</i>
                    <span class="title">{{ $item.Title }}</span>
                    <p class="metadata grey-text text-lighten-1">
                        <em>{{ .Duration | formatDuration }}, added on {{ $item.AddedAt.Format "2006-01-02" }}</em>
                    </p>
                    {{ if $item.MediaURL }}
                    <audio id="audio-{{ $i }}" preload="none" controls="" type="{{ $item.MIMEType }}">
                        <source type="{{ $item.MIMEType }}" src="{{ $item.MediaURL }}">
                            Sorry, your browser does not support HTML5 audio.
                    </audio>
                    {{ end }}
                    {{ if $item.Description }}
                    {{ if ne .Title $item.Description }}
                    <p class="description">
                        {{ $item.Description }}
                    </p>
                    {{ end }}
                    {{ end }}
                </li>
                {{ end }}
            </ul>
        </div>
        {{ end }}
    </div>
    <script type="text/javascript"
        src="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/js/materialize.min.js"></script>
    <script type="text/javascript">
    (function (w) {
		var activeItem;

		function play(el) {
			activeItem = el;
			
			let audio = document.getElementById(activeItem.dataset["audioId"]);

			activeItem.innerText = "pause_circle_filled";
			audio.play();
		}

		function pause() {
			if (!activeItem) {
				return;
			}

			let audio = document.getElementById(activeItem.dataset["audioId"]);

			activeItem.innerText = "play_circle_filled";
			audio.pause();
			activeItem = null;
		}

		function togglePlayButton(event) {
			let changeTrack = event.target !== activeItem
			pause();

			if (changeTrack) {
				play(event.target);
			}
		}

        document.querySelectorAll("#playlist [id^='audio-control-']").forEach(function (el) {
            el.addEventListener("click", togglePlayButton);
        });
    })(window);
    </script>
</body>

</html>`))

type HTMLRenderer struct{}

func (HTMLRenderer) ContentType() string {
	return "text/html"
}

func (HTMLRenderer) Render(w io.Writer, feed Feed) error {
	return indexTemplate.Execute(w, feed)
}

type AtomRenderer struct{}

func (AtomRenderer) ContentType() string {
	return "application/xml"
}

func (AtomRenderer) Render(w io.Writer, feed Feed) error {
	var pubDate *time.Time
	if !feed.PubDate.IsZero() {
		pubDate = &feed.PubDate
	}

	p := podcast.New(feed.Title, feed.URL, feed.Description, pubDate, nil)
	p.AddImage(feed.IconURL) // (scheme + "://" + req.Host + "/favicon.ico")

	for _, it := range feed.Items {
		item := podcast.Item{
			Title: it.Title,
			Author: &podcast.Author{
				Name:  it.Author,
				Email: "user@example.com",
			},
			Description: it.Description,
			Link:        it.OriginalURL,
			PubDate:     &it.AddedAt,
		}

		item.AddEnclosure(it.MediaURL, mimeTypeToEnclosureType(it.MIMEType), int64(it.ContentLength))

		if _, err := p.AddItem(item); err != nil {
			log.Printf("failed to add %s: %s", it.OriginalURL, err)
		}
	}

	return p.Encode(w)
}
