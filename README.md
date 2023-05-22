YouCast — this video could've been a podcast
============================================

YouCast is a self-hosted podcast service that serves audio files adding by the user as a feed, compatible with most if not all modern podcast apps.

YouCast supports following sources of media files:
* YouTube — add video URL and YouCast will download and extract the audio from it.
* [Telegram](#telegram-bot) — send a message with an audio file attached to the Telegram bot, and it will be added to your feed.
* Upload — upload audio file to add it to the podcast feed.

Installation
------------

Using go1.16+:
```bash
go install github.com/andrewslotin/youcast@latest
```

Using Docker:
```bash
docker pull ghcr.io/andrewslotin/youcast@latest
```

Usage
-----
YouCast is designed to be a low-maintenance service. Once launched, it can run permanently in your local network or a [cloud instance](#running-youcast-outside-of-your-local-network), serving podcast to all your devices. Here is the minimal command to run an instance of this service installed with `go install`:

```bash
$GOPATH/bin/youcast -l :8080 -storage-dir ./downloads
```

This will launch a podcast server available at https://localhost:8080.

See the [Configuration](#configuration) section for more detailed instructions.

#### Running YouCast outside of your local network
The common use case for YouCast is to run it inside of your home network that is not externally accessible. Since YouCast allows users to upload files, it is a **really bad idea** to run it on a publicly available server, such as AWS instance or a DigitalOcean droplet, without any authentication. Consider using a reverse-proxy, or any other solution of your choice.

Configuration
-------------

YouCast requires few configuration options to run. These options can be provided both as command-line arguments and environment variables. If a configuration option is provided via both env vars and args, the latter takes precedence.

| Command-line flag | Environment variable | Description                                           | Required | Default value | 
|-------------------|----------------------|-------------------------------------------------------|----------|---------------|
| `-l`              | `LISTEN_ADDR`        | Server address `[host]:port`                          | **Yes**  |               |
| `-storage-dir`    | `STORAGE_PATH`       | Path to the directory where to store downloaded files | **Yes**  |               |
| `-title`          | `PODCAST_TITLE`      | Feed title, displayed as a podcast name               | No       | `YouCast`     |
| `-db`             | `DB_PATH`            | Path to the database file                             | No       | `./feed.db`   |

### Telegram bot
YouCast comes with a Telegram bot included. To activate the bot you need an API token, that can be obtained via [@BotFather](https://t.me/botfather). Please consult [Telegram's Bot API Guide](https://core.telegram.org/bots#how-do-i-create-a-bot) for details.

#### Large file downloads
Telegram Bot API [limits](https://core.telegram.org/bots/faq#how-do-i-download-files) downloads to 20 MB per file. While this should be enough for most of use cases, some audio files may exceed it. A way to work around this limitation is to run a [self-hosted Bot API server](https://core.telegram.org/bots/api#using-a-local-bot-api-server).

#### Whitelisting users
By default your bot will accept files from any Telegram user. It is strictly recommended to provide a list of user IDs that are allowed to send messages to the bot. You can find out your own user ID using [@IDBot](https://t.me/username_to_id_bot).

#### Configuration options
Here are the configuration options for the YouCast Telegram bot that you can provide via environment variables. Note that until `TELEGRAM_API_TOKEN` is provided, the bot remains inactive.

| Environment variable     | Description                                                                                                                     | Required | Default value              |
|--------------------------|---------------------------------------------------------------------------------------------------------------------------------|----------|----------------------------|
| `TELEGRAM_API_TOKEN`     | The token for Telegram Bot API                                                                                                  | **Yes**  |                            |
| `TELEGRAM_API_ENDPOINT`  | Telegram bot API endpoint URL. You need to set it if using a self-hosted Bot API server ([details](#downloading-large-files)) | No       | `https://api.telegram.org` |
| `TELEGRAM_FILE_SERVER`   | The file server URL of a self-hosted API server, if used ([details](#downloading-large-files))                                | No       |                            |
| `TELEGRAM_ALLOWED_USERS` | Optional list of Telegram user IDs that are allowed to add items to the feed, comma-separated ([details](#whitelisting-users))     | No       |                            |

License
-------
The creators of YouCast bear no responsibility for any illegal dissemination of copyrighted material via YouCast. This software has been conceptualized and developed for individual utilization, mandating that users secure legal authorization to access content prior to uploading it to YouCast, and abide by the specific user agreement under which it is distributed.

YouCast source code is distributed under the [MIT License](LICENSE.md).