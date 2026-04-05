# 🏰 Territorial Cults Go (2026)

A high-performance, natively ported Discord bot for managing competitive **Territorial.io** gaming communities. This is the 2026 Go evolution of the original Python bot, optimized for speed, concurrency, and reliability.

Credits Viktor's Point Bot (Python)[https://github.com/viktorexe/cults-2025]

## 🚀 Key Improvements in Go Version

- **Blazing Fast Scraping**: Polling interval reduced to **2 seconds** for near-instant winlog reporting.
- **Concurrent Processing**: Leverages Go's goroutines to handle multiple match results and interactions simultaneously without blocking.
- **Robustness**: Implements strict `context` timeouts for all database operations to prevent hangs.
- **Type Safety**: Full BSON/Struct mapping for MongoDB to ensure data integrity.
- **Health Monitoring**: Built-in HTTP health-check server (Port 8000) for cloud liveness probes.

## 🎮 Features

### ⚔️ Cult & War System
- **Cult Management**: Create, join, and manage gaming "cults" (clans).
- **War Engine**: Declare wars between cults with automatic score tracking.
- **Leaderboards**: Dedicated cult leaderboards and status tracking.

### 📊 Advanced Economy
- **Automated Points**: Real-time scraping of `territorial.io/clan-results`.
- **Claim System**: Interactive buttons (1x, 1.3x Duo, 1.5x Solo) with 5-minute expiry.
- **Auto-Credit**: Linked accounts receive points automatically without needing to click buttons.
- **Multipliers**: Global server-wide multipliers for special events.

### 🎖️ Reward Roles
- **Milestones**: Automatic role assignment based on point/win thresholds.
- **Stacking Logic**: Configurable "Stackable" or "Highest-Only" role management.
- **Retroactive Refresh**: Batch process entire servers to assign missing roles.

### 🔗 Account Linking
- Map Discord IDs to 5-character Territorial.io account codes.
- Admin tools to manage and verify player identities.

---

## 🛠️ Tech Stack

- **Language**: Go 1.21+
- **Library**: [Discordgo](https://github.com/bwmarrin/discordgo)
- **Database**: MongoDB (v6.0+)
- **Environment**: Dotenv for configuration.

---

## ⚙️ Setup & Installation

### 1. Prerequisites
- [Go](https://go.dev/doc/install) installed on your system.
- A running **MongoDB** instance (Local or Atlas).
- A Discord Bot Token with `Server Members` and `Message Content` intents enabled.

### 2. Clone & Install
```bash
git clone <your-repo-url>
cd go-cult-2025
go mod download
```

### 3. Configuration
Copy the example environment file and fill in your details:
```bash
cp .env.example .env
```
**Required Fields:**
- `DISCORD_TOKEN`: Your bot token.
- `MONGODB_URI`: Your Mongo connection string.
- `DB_NAME`: Database name to use.
- `BOT_OWNER`: Your Discord User ID (for bypass permissions).

### 4. Running
**Development:**
```bash
go run cmd/bot/main.go
```
**Production Build:**
```bash
go build -o bot cmd/bot/main.go
./bot
```

---

## 📜 Commands Reference

### 👤 User Commands (Public)
| Command | Usage | Options |
| :--- | :--- | :--- |
| `/profile` | View your or someone else's stats | `user` (Optional) |
| `/leaderboard` | View rankings (Points/Wins) | `days` (Optional: filter by 24h, 48h, etc.) |
| `/rolelist` | Show required milestones for Reward Roles | None |
| `/multiplier_info` | Check if a server-wide event is active | None |
| `/help` | Display the help menu | None |
| `/invite` | Get the bot's invite link | None |

### ⚔️ Cult Management
| Command | Usage | Permission |
| :--- | :--- | :--- |
| `/cult_create` | Start a new cult with clan tag | Any User |
| `/cult_info` | View stats and members of a cult | Any User |
| `/cult_list` | List all active cults in the server | Any User |
| `/join_cult` | Join a specific cult | Any User |
| `/promote_member` | Promote member to Officer status | Cult Leader |
| `/cult_war` | Challenge another cult to war | Cult Leader / Officer |

### 🛠️ Admin Commands (Administrator Permission)
| Command | Usage | Options |
| :--- | :--- | :--- |
| `/add` | Add points (and wins) to a user | `points`, `user` (opt), `wins` (opt) |
| `/remove` | Deduct points/wins from a user | `user`, `points`, `wins` |
| `/rewardrole` | Configure a new milestone reward | `role`, `amount`, `type`, `channel` |
| `/listrewards` | View all internal reward configurations | None |
| `/deletereward` | Remove a milestone reward | `role` |
| `/editrewardrole` | Modify thresholds for a role | `role`, `new_amount`, `new_channel` |
| `/set_reward_stackable` | Enable/Disable role stacking | `stackable` (Yes/No) |
| `/force_refresh_rewards` | Retroactively process all members | None |
| `/cleanup_roles` | Deduplicate roles (keep only highest tier) | None |
| `/set_multiplier` | Enable server-wide point bonus | `multiplier`, `description` (opt) |
| `/edit_multiplier` | Tweak active server-wide event | `multiplier`, `description` |
| `/end_multiplier` | Deactivate the current server bonus | None |
| `/set_winlog` | Set up automated scraping channel | `channel`, `clan_name`, `remove` |
| `/account_linking` | Link a Discord ID to T.io code | `account_name`, `user` |
| `/bot_manager` | Set the "Bot Manager" helper role | `role` |
| `/debug_rewards` | Check internal eligibility logic | None |

### 👑 Owner Commands (Bot Owner Only)
| Command | Usage | Options |
| :--- | :--- | :--- |
| `/adminpoints` | Manual scrape points from a message | `message_id` |

---

## 📁 Project Structure

- `cmd/bot/`: Entry point and main initialization logic.
- `internal/commands/`: All Discord slash command handlers and routing.
- `internal/database/`: MongoDB connection and collection-specific helpers.
- `internal/models/`: Shared BSON/Go structs for data integrity.
- `internal/scraper/`: The engine that polls and parses Territorial.io web data.

---

## 🐳 Deployment

### Docker
The project includes a `Dockerfile` for containerized environments.
```bash
docker build -t go-cult-bot .
docker run --env-file .env go-cult-bot
```

---

## 📜 License
Free to use and modify for your community. 

Made with 🐹 by the Territorial.io Cults Team.
