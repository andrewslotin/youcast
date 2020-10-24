package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

type TelegramProvider struct {
	api *tgbotapi.BotAPI
}

func NewTelegramProvider(token string) (*TelegramProvider, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize telegram api: %w", err)
	}

	return &TelegramProvider{api: api}, nil
}

func (tg *TelegramProvider) Name() string {
	return "Telegram"
}

func (tg *TelegramProvider) HandleRequest(w http.ResponseWriter, req *http.Request) audioSource {
	var msg tgbotapi.Message
	if err := json.NewDecoder(req.Body).Decode(&struct {
		Message *tgbotapi.Message `json:"message"`
	}{&msg}); err != nil {
		log.Printf("failed to unmarshal telegram message: %s", err)
		tg.sendResponse(msg, "Could not add this item: Telegram sent nonsense")
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)

		return nil
	}

	if msg.Audio == nil {
		tg.sendResponse(msg, "Could not add this item: there is no audio")
		w.WriteHeader(http.StatusNoContent)

		return nil
	}

	u, err := tg.api.GetFileDirectURL(msg.Audio.FileID)
	if err != nil {
		log.Printf("failed to fetch telegram audio url: %s", err)
		tg.sendResponse(msg, "Could not add this item: "+err.Error())
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return nil
	}

	tg.sendResponse(msg, u)

	if msg.Audio.Performer == "" {
		user := msg.ForwardFrom
		if user == nil {
			user = msg.From
		}

		msg.Audio.Performer = "@" + user.UserName
	}

	if msg.Audio.Title == "" {
		msg.Audio.Title = msg.Caption
	}

	return &TelegramMessage{
		Audio:       msg.Audio,
		Description: msg.Caption,
		FileURL:     u,
	}
}

func (tg *TelegramProvider) sendResponse(msg tgbotapi.Message, text string) {
	resp := tgbotapi.NewMessage(msg.Chat.ID, text)
	resp.ReplyToMessageID = msg.MessageID

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

func (tg *TelegramMessage) AudioStreamURL(ctx context.Context) (string, error) {
	return tg.FileURL, nil
}
