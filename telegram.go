package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

var ErrUserNotAllowed = errors.New("user was not whitelisted")

type TelegramProvider struct {
	api             *tgbotapi.BotAPI
	mediaServiceURL *url.URL
	allowedUsers    map[int]struct{}
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

	p := &TelegramProvider{
		api: api,
	}

	if mediaServiceURL != "" {
		u, err := url.Parse(mediaServiceURL)
		if err != nil {
			return nil, fmt.Errorf("malformed media service URL %s: %w", mediaServiceURL, err)
		}

		p.mediaServiceURL = u
	}

	return p, nil
}

func (tg *TelegramProvider) WhitelistUser(id int) {
	log.Printf("allowing user with id %d to send commands to bot", id)
	if tg.allowedUsers != nil {
		tg.allowedUsers[id] = struct{}{}
		return
	}

	tg.allowedUsers = map[int]struct{}{
		id: struct{}{},
	}
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
		tg.sendResponse(msg, "Could not add this item: Telegram sent nonsense", true)
		w.WriteHeader(http.StatusNoContent)

		return nil
	}

	w.WriteHeader(http.StatusNoContent)

	switch src, err := tg.HandleMessage(msg); err {
	case ErrUserNotAllowed, ErrNoAudio:
		return nil
	case nil:
		return src
	default:
		tg.sendResponse(msg, "Could not add this item: "+err.Error(), true)
		return nil
	}
}

func (tg *TelegramProvider) HandleMessage(msg *tgbotapi.Message) (*TelegramMessage, error) {
	if tg.allowedUsers != nil {
		if _, ok := tg.allowedUsers[msg.From.ID]; !ok {
			return nil, ErrUserNotAllowed
		}
	}

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

	var sender string

	switch {
	case msg.ForwardFromChat != nil:
		sender = msg.ForwardFromChat.Title
	case msg.ForwardFrom != nil:
		sender = msg.ForwardFrom.UserName
	default:
		sender = msg.From.UserName
	}

	if msg.Caption == "" {
		msg.Caption = fmt.Sprintf("Audio from %s submitted on %s", sender, time.Unix(int64(msg.Date), 0).Format("Jan, 02 15:04 MST"))
	}

	if msg.Audio.Performer == "" {
		msg.Audio.Performer = sender
	}

	if msg.Audio.Title == "" {
		msg.Audio.Title = msg.Caption
	}

	var linkURL string
	switch {
	case msg.ForwardFromChat != nil:
		if chatID := strconv.FormatInt(msg.ForwardFromChat.ID, 10); len(chatID) > 4 {
			linkURL = path.Join("https://t.me/c", chatID[4:], strconv.Itoa(msg.MessageID))
		} else {
			log.Println("unexpected telegram chat ID:", msg.Chat.ID)
		}
	case msg.Chat.IsChannel():
		linkURL = path.Join("https://t.me", msg.Chat.UserName, strconv.Itoa(msg.MessageID))
	case msg.Chat.IsSuperGroup():
		if chatID := strconv.FormatInt(msg.Chat.ID, 10); len(chatID) > 4 {
			linkURL = path.Join("https://t.me/c", chatID[4:], strconv.Itoa(msg.MessageID))
		} else {
			log.Println("unexpected telegram chat ID:", msg.Chat.ID)
		}
	}

	return &TelegramMessage{
		Audio:       msg.Audio,
		Description: msg.Caption,
		Link:        linkURL,
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
				case ErrUserNotAllowed:
					log.Printf("UNAUTHORIZED message from user %s (id:%d)", upd.Message.From.UserName, upd.Message.From.ID)
				case ErrNoAudio:
					log.Printf("incoming message from user %s (id:%d)", upd.Message.From.UserName, upd.Message.From.ID)
					tg.HandleCommand(upd.Message)
				case nil:
					res <- src
					tg.sendResponse(upd.Message, fmt.Sprintf(`Will add "%s" to your feed`, src.Audio.Title), true)
				default:
					log.Printf("failed to handle Telegram update: %s", err)
					tg.sendResponse(upd.Message, "Could not add this item: "+err.Error(), true)
				}
			case <-ctx.Done():
				log.Println("context cancelled, shutting down Telegram provider")
				return
			}
		}
	}()

	return res, nil
}

func (tg *TelegramProvider) HandleCommand(msg *tgbotapi.Message) {
	switch strings.ToLower(msg.Text) {
	case "/start", "/help":
		tg.sendResponse(msg, "Hello, I'm LaterCast bot! Just forward me audio files and I will add them to your feed.", false)
	case "/status":
		tg.sendResponse(msg, "Up and running!", false)
	default:
		tg.sendResponse(msg, "Unknown command, send /help", false)
	}
}

func (tg *TelegramProvider) sendResponse(msg *tgbotapi.Message, text string, quoteSrc bool) {
	resp := tgbotapi.NewMessage(msg.Chat.ID, text)
	if quoteSrc {
		resp.ReplyToMessageID = msg.MessageID
	}

	if _, err := tg.api.Send(resp); err != nil {
		log.Printf("failed to send response: %s", err)
	}
}

type TelegramMessage struct {
	Audio       *tgbotapi.Audio
	Description string
	Link        string
	FileURL     string
}

func (tg *TelegramMessage) Metadata(ctx context.Context) (Metadata, error) {
	return Metadata{
		Type:          TelegramItem,
		OriginalURL:   tg.Link,
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
