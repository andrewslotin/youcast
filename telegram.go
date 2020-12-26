package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

type TelegramProvider struct {
	api             *tgbotapi.BotAPI
	mediaServiceURL *url.URL
}

func NewTelegramProvider(token, apiEndpoint, mediaServiceURL string) (*TelegramProvider, error) {
	if apiEndpoint == "" {
		apiEndpoint = tgbotapi.APIEndpoint
	}

	if !strings.HasSuffix(apiEndpoint, "/bot%s/%s") {
		apiEndpoint += "/bot%s/%s"
	}

	api, err := tgbotapi.NewBotAPIWithAPIEndpoint(token, apiEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize telegram api: %w", err)
	}

	p := &TelegramProvider{api: api}

	if mediaServiceURL != "" {
		u, err := url.Parse(mediaServiceURL)
		if err != nil {
			return nil, fmt.Errorf("malformed media service URL %s: %w", mediaServiceURL, err)
		}

		p.mediaServiceURL = u
	}

	return p, nil
}

func (tg *TelegramProvider) Name() string {
	return "Telegram"
}

func (tg *TelegramProvider) HandleRequest(w http.ResponseWriter, req *http.Request) audioSource {
	msg := &tgbotapi.Message{}
	if err := json.NewDecoder(req.Body).Decode(&struct {
		Message *tgbotapi.Message `json:"message"`
	}{msg}); err != nil {
		log.Printf("failed to unmarshal telegram message: %s", err)
		tg.sendResponse(msg, "Could not add this item: Telegram sent nonsense")
		w.WriteHeader(http.StatusNoContent)

		return nil
	}

	switch src, err := tg.HandleMessage(msg); err {
	case ErrNoAudio:
		w.WriteHeader(http.StatusNoContent)
		return nil
	case nil:
		tg.sendResponse(msg, fmt.Sprintf(`Added "%s" to your feed`, src.Audio.Title))
		w.WriteHeader(http.StatusNoContent)
		return src
	default:
		tg.sendResponse(msg, "Could not add this item: "+err.Error())
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}

func (tg *TelegramProvider) HandleMessage(msg *tgbotapi.Message) (*TelegramMessage, error) {
	if msg.Audio == nil {
		return nil, ErrNoAudio
	}

	u, err := tg.api.GetFileDirectURL(msg.Audio.FileID)
	if err != nil {
		log.Printf("failed to fetch telegram audio url: %s", err)
		return nil, fmt.Errorf("failed to fetch file URL: %w", err)
	}

	if tg.mediaServiceURL != nil {
		u1, err := url.Parse(u)
		if err != nil {
			return nil, fmt.Errorf("telegram api returned malformed download url %s: %w", u, err)
		}

		u1.Scheme = tg.mediaServiceURL.Scheme
		u1.Host = tg.mediaServiceURL.Host

		u = u1.String()
	}

	if msg.Audio.Performer == "" {
		user := msg.ForwardFrom
		if user == nil {
			user = msg.From
		}

		msg.Audio.Performer = "@" + user.UserName
	}

	if msg.Caption == "" {
		msg.Caption = fmt.Sprintf("Audio from %s submitted on %s", msg.From.UserName, time.Unix(int64(msg.Date), 0))
	}

	if msg.Audio.Title == "" {
		msg.Audio.Title = msg.Caption
	}

	return &TelegramMessage{
		Audio:       msg.Audio,
		Description: msg.Caption,
		FileURL:     u,
	}, nil
}

func (tg *TelegramProvider) Updates(ctx context.Context) (<-chan *TelegramMessage, error) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := tg.api.GetUpdatesChan(u)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to updates: %w", err)
	}

	res := make(chan *TelegramMessage, 10)
	go func() {
		defer close(res)

		for {
			select {
			case upd := <-updates:
				switch src, err := tg.HandleMessage(upd.Message); err {
				case ErrNoAudio:
					log.Printf("no audio found in update %d", upd.UpdateID)
					tg.sendResponse(upd.Message, "This message does not seem to have any audio attached")
				case nil:
					res <- src
					tg.sendResponse(upd.Message, fmt.Sprintf(`Added "%s" to your feed`, src.Audio.Title))
				default:
					log.Printf("failed to handle Telegram update: %s", err)
					tg.sendResponse(upd.Message, "Could not add this item: "+err.Error())
				}
			case <-ctx.Done():
				log.Println("context cancelled, shutting down Telegram provider")
				return
			}
		}
	}()

	return res, nil
}

func (tg *TelegramProvider) sendResponse(msg *tgbotapi.Message, text string) {
	resp := tgbotapi.NewMessage(msg.Chat.ID, text)
	if msg != nil {
		resp.ReplyToMessageID = msg.MessageID
	}

	if _, err := tg.api.Send(resp); err != nil {
		log.Printf("failed to respond to %s: %s", msg.From.UserName, err)
	}
}

type TelegramMessage struct {
	Audio       *tgbotapi.Audio
	Description string
	FileURL     string
}

func (tg *TelegramMessage) Metadata(ctx context.Context) (Metadata, error) {
	return Metadata{
		Type:          TelegramItem,
		OriginalURL:   tg.FileURL,
		Title:         tg.Audio.Title,
		Author:        tg.Audio.Performer,
		Description:   tg.Description,
		Duration:      time.Duration(tg.Audio.Duration) * time.Second,
		MIMEType:      tg.Audio.MimeType,
		ContentLength: int64(tg.Audio.FileSize),
	}, nil
}

func (tg *TelegramMessage) DownloadURL(context.Context) (string, error) {
	return tg.FileURL, nil
}
