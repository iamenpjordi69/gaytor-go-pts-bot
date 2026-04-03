package scraper

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"go-cult-2025/internal/database"
	"go-cult-2025/internal/models"

	"github.com/bwmarrin/discordgo"
	"go.mongodb.org/mongo-driver/bson"
)

var lastWinlogTime string

// StartMonitoring initiates the background routines
func StartMonitoring(s *discordgo.Session) {
	log.Println("Starting scraper monitoring processes...")

	// Winlog ticker
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			wl, err := ScrapeMatchLogs()
			if err != nil {
				continue // Likely no new match or parse error, silent continue
			}

			if wl.Time == lastWinlogTime {
				continue
			}
			if lastWinlogTime == "" {
				// Initialize the first run so we don't retro-process old entries
				lastWinlogTime = wl.Time
				continue
			}

			log.Printf("Detected new winlog from territorial.io at %s", wl.Time)
			lastWinlogTime = wl.Time

			processWinLogForGuilds(context.Background(), s, wl)
		}
	}()

	// War monitor
	MonitorWars(s)
}

func processWinLogForGuilds(ctx context.Context, s *discordgo.Session, wl *WinLog) {
	// Fetch all winlog settings
	cursor, err := database.ColWinlogSetting.Find(ctx, bson.M{"active": true})
	if err != nil {
		log.Printf("Error fetching winlog settings: %v", err)
		return
	}
	defer cursor.Close(ctx)

	var settings []models.WinlogSetting
	if err := cursor.All(ctx, &settings); err != nil {
		log.Printf("Error reading winlog settings: %v", err)
		return
	}

	for _, setting := range settings {
		clanMatches := false
		if setting.ClanName != "" {
			if stringsToLowerClean(setting.ClanName) == stringsToLowerClean(wl.WinningClan) {
				clanMatches = true
			}
		}

		if !clanMatches {
			continue
		}

		// Proceed to process for this guild
		processGuildAutoCredits(ctx, s, wl, setting)
	}
}

func stringsToLowerClean(str string) string {
	// simple lowercasing, ignoring spaces (could be better)
	return strings.ToLower(strings.TrimSpace(str))
}

func processGuildAutoCredits(ctx context.Context, s *discordgo.Session, wl *WinLog, setting models.WinlogSetting) {
	// Find linked accounts
	var links []models.AccountLink
	linkCur, err := database.ColAccountLinks.Find(ctx, bson.M{"guild_id": setting.GuildID})
	if err == nil {
		linkCur.All(ctx, &links)
	}

	var autoCredited []string

	for _, payoutAcc := range wl.PayoutAccounts {
		var linkedUser *models.AccountLink

		// 1. Exact match
		for _, l := range links {
			if l.AccountName == payoutAcc {
				linkedUser = &l
				break
			}
		}

		// 2. Case insensitive
		if linkedUser == nil {
			for _, l := range links {
				if strings.ToLower(l.AccountName) == strings.ToLower(payoutAcc) {
					linkedUser = &l
					break
				}
			}
		}

		if linkedUser != nil {
			// user found, award points
			// We skip the complex db logic here and just invoke a handler later
			success, err := database.AddWinlogPoints(ctx, linkedUser.UserID, setting.GuildID, wl.FinalPoints)
			if err != nil {
				log.Printf("Failed adding auto-credit to %d: %v", linkedUser.UserID, err)
			} else if success {
				autoCredited = append(autoCredited, fmt.Sprintf("<@%d>", linkedUser.UserID))

				// Try to DM the user
				mult := 1.0 // fallback
				multData := models.Multiplier{}
				if err := database.ColMultipliers.FindOne(ctx, bson.M{"guild_id": setting.GuildID, "active": true}).Decode(&multData); err == nil {
					mult = multData.Multiplier
				}

				embedDesc := fmt.Sprintf("You received **%.1f points** and **1 win** from [%s] win on %s!\n\nAccount: `%s`", wl.FinalPoints*mult, wl.WinningClan, wl.MapName, payoutAcc)
				if wl.IsContest {
					embedDesc += "\n*Contest game - double points!*"
				}
				
				dmChan, err := s.UserChannelCreate(fmt.Sprintf("%d", linkedUser.UserID))
				if err == nil {
					s.ChannelMessageSendEmbed(dmChan.ID, &discordgo.MessageEmbed{
						Title:       "🎉 Auto-Credited Points!",
						Description: embedDesc,
						Color:       0x00ff00,
					})
				}
			}
		}
	}

	// Send Claim Button
	desc := fmt.Sprintf("[%s] won on %s\n", wl.WinningClan, wl.MapName)
	if wl.IsContest {
		desc = fmt.Sprintf("[%s] won on %s (Contest)\n%d players x2 = %.0f points available to claim!\n[%s → %s]", wl.WinningClan, wl.MapName, wl.PlayerCount, wl.FinalPoints, wl.PrevPoints, wl.CurrPoints)
	} else {
		desc = fmt.Sprintf("[%s] won on %s\n%.0f points available to claim!\n[%s → %s]", wl.WinningClan, wl.MapName, wl.FinalPoints, wl.PrevPoints, wl.CurrPoints)
	}

	if len(autoCredited) > 0 {
		desc += fmt.Sprintf("\n\n**Auto-credited:** ")
		for i, m := range autoCredited {
			if i > 0 {
				desc += ", "
			}
			desc += m
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🏆 Win Log",
		Description: desc,
		Color:       0x00ff00,
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Click to claim points • Expires in 5 minutes",
		},
	}

	// Dynamic interaction ID construction
	baseID := fmt.Sprintf("claim_winlog_%d_%.0f_", setting.GuildID, wl.FinalPoints)

	btn1 := discordgo.Button{
		Label:    "Claim (1x)",
		Style:    discordgo.SecondaryButton,
		Emoji:    &discordgo.ComponentEmoji{Name: "🎯"},
		CustomID: baseID + "1.0",
	}
	btn2 := discordgo.Button{
		Label:    "DUO win (x1.3)",
		Style:    discordgo.PrimaryButton,
		Emoji:    &discordgo.ComponentEmoji{Name: "🤝"},
		CustomID: baseID + "1.3",
	}
	btn3 := discordgo.Button{
		Label:    "SOLO win (x1.5)",
		Style:    discordgo.SuccessButton,
		Emoji:    &discordgo.ComponentEmoji{Name: "👑"},
		CustomID: baseID + "1.5",
	}

	view := discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{btn1, btn2, btn3},
	}

	msg, err := s.ChannelMessageSendComplex(strconv.FormatInt(setting.ChannelID, 10), &discordgo.MessageSend{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: []discordgo.MessageComponent{view},
	})

	if err == nil {
		// Register timeout
		go func(msgID string, chID string) {
			time.Sleep(5 * time.Minute)
			// Remove buttons after 5 mins
			embed.Footer.Text = "This win log has expired"
			embed.Color = 0x808080
			
			s.ChannelMessageEditComplex(&discordgo.MessageEdit{
				ID: msgID,
				Channel: chID,
				Embeds: &[]*discordgo.MessageEmbed{embed},
				Components: &[]discordgo.MessageComponent{}, // Empty 
			})
		}(msg.ID, msg.ChannelID)
	} else {
		log.Printf("Failed to send winlog to guild %d: %v", setting.GuildID, err)
	}
}
