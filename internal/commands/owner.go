package commands

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go-cult-2025/internal/database"
	"go-cult-2025/internal/models"

	"github.com/bwmarrin/discordgo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func init() {
	SlashCommands = append(SlashCommands, 
		&discordgo.ApplicationCommand{
			Name:        "set_winlog",
			Description: "Configure win log settings (Owner/Manager only). Leave blank to remove settings.",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionChannel,
					Name:        "channel",
					Description: "Channel to send win logs",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "clan_name",
					Description: "Exact clan tag to filter (e.g. OG)",
					Required:    false,
				},
			},
		},
		&discordgo.ApplicationCommand{
			Name:        "account_linking",
			Description: "Link territorial.io account to Discord user",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "account_name",
					Description: "Territorial.io account name (5 characters)",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "user",
					Description: "Discord user to link",
					Required:    true,
				},
			},
		},
		&discordgo.ApplicationCommand{
			Name:        "adminpoints",
			Description: "Add points from leaderboard message",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "message_id",
					Description: "Message ID of the leaderboard",
					Required:    true,
				},
			},
		},
	)
	
	CommandHandlers["set_winlog"] = setWinlogHandler
	CommandHandlers["account_linking"] = accountLinkingHandler
	CommandHandlers["adminpoints"] = adminpointsHandler
}

func setWinlogHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}

	if !IsBotOwner(usr.ID) { // You could expand this to check managers
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Bot Owner permission required!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := i.ApplicationCommandData().Options

	if len(opts) == 0 {
		// Remove setting
		_, err := database.ColWinlogSetting.DeleteMany(ctx, bson.M{"guild_id": guildID})
		msg := "✅ Win log tracking has been removed for this server."
		if err != nil {
			msg = "❌ Failed to remove win log settings."
		}
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: msg,
			},
		})
		return
	}

	if len(opts) < 2 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ You must provide both a **channel** and a **clan_name** to set up tracking, or provide NO options to remove it.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	var channelIDStr, clanName string
	for _, opt := range opts {
		if opt.Name == "channel" {
			channelIDStr = opt.ChannelValue(s).ID
		} else if opt.Name == "clan_name" {
			clanName = opt.StringValue()
		}
	}

	channelID, _ := strconv.ParseInt(channelIDStr, 10, 64)

	// Delete any existing setting for this guild to ensure clean replacement
	database.ColWinlogSetting.DeleteMany(ctx, bson.M{"guild_id": guildID})

	_, err := database.ColWinlogSetting.InsertOne(ctx, bson.M{
		"guild_id":   guildID,
		"channel_id": channelID,
		"clan_name":  clanName,
		"active":     true,
	})

	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Failed to update database.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "✅ Win Log Configured",
		Description: fmt.Sprintf("Win logs for clan `[%s]` will be sent to <#%s>", clanName, channelIDStr),
		Color:       0x00ff00,
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func accountLinkingHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}

	if !IsBotOwner(usr.ID) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ You are not authorized to use this command!", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	accountName := i.ApplicationCommandData().Options[0].StringValue()
	targetDiscordUser := i.ApplicationCommandData().Options[1].UserValue(s)

	if len(accountName) != 5 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Account name must be exactly 5 characters!", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	targetID, _ := strconv.ParseInt(targetDiscordUser.ID, 10, 64)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := options.Update().SetUpsert(true)
	_, err := database.ColAccountLinks.UpdateOne(
		ctx,
		bson.M{"user_id": targetID, "guild_id": guildID},
		bson.M{
			"$set": bson.M{
				"user_id":      targetID,
				"guild_id":     guildID,
				"account_name": accountName,
				"linked_by":    usr.ID,
				"timestamp":    time.Now().UTC(),
			},
		},
		opts,
	)

	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ An error occurred connecting database!", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	// Try to DM user
	dm, err := s.UserChannelCreate(targetDiscordUser.ID)
	if err == nil {
		s.ChannelMessageSendEmbed(dm.ID, &discordgo.MessageEmbed{
			Title:       "🔗 Account Linked",
			Description: fmt.Sprintf("Your territorial.io account `%s` is now linked with bot!\n\nIf you are a clan winner, points will be automatically added to your account.", accountName),
			Color:       0x00ff00,
		})
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{{
				Title:       "✅ Account Linked",
				Description: fmt.Sprintf("Successfully linked `%s` to <@%d>", accountName, targetID),
				Color:       0x00ff00,
			}},
		},
	})
}

func adminpointsHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}

	if !IsBotOwner(usr.ID) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ You don't have permission to use this command!", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})

	messageIDStr := i.ApplicationCommandData().Options[0].StringValue()
	_, err := strconv.ParseInt(messageIDStr, 10, 64)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: stringPtr("❌ Invalid message ID format!")})
		return
	}

	// Brute force fetch message across all channels (inefficient but matches python exactly)
	channels, err := s.GuildChannels(i.GuildID)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: stringPtr("❌ Failed to fetch channels!")})
		return
	}

	var foundMsg *discordgo.Message
	for _, ch := range channels {
		if ch.Type == discordgo.ChannelTypeGuildText {
			msg, err := s.ChannelMessage(ch.ID, messageIDStr)
			if err == nil && msg != nil {
				foundMsg = msg
				break
			}
		}
	}

	if foundMsg == nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: stringPtr("❌ Message not found in any channel!")})
		return
	}

	content := foundMsg.Content
	if len(foundMsg.Embeds) > 0 {
		if foundMsg.Embeds[0].Description != "" {
			content = foundMsg.Embeds[0].Description
		}
	}

	if content == "" {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: stringPtr("❌ Message has no content to process!")})
		return
	}

	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Multiplier check
	var mult models.Multiplier
	multiplierVal := 1.0
	err = database.ColMultipliers.FindOne(ctx, bson.M{"guild_id": guildID, "active": true}).Decode(&mult)
	if err == nil {
		multiplierVal = mult.Multiplier
	}

	lines := strings.Split(content, "\n")
	processed := 0
	failed := 0
	var successDetails []string

	generalPointsRegex := regexp.MustCompile(`<@!?(\d+)>\s*•\s*([\d.,]+)`)

	// Load members to cache (very rough caching, discordgo doesn't auto-cache 1000s deeply instantly)
	membersCache := make(map[string]*discordgo.Member)
	mems, _ := s.GuildMembers(i.GuildID, "", 1000)
	for _, m := range mems {
		membersCache[strings.ToLower(m.User.Username)] = m
		if m.Nick != "" {
			membersCache[strings.ToLower(m.Nick)] = m
		}
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Leaderboard") || strings.HasPrefix(line, "Showing") || line == "⠀" {
			continue
		}

		var targetID int64
		var points float64
		var targetUser *discordgo.User
		matched := false

		// Pattern: 1. <@ID> • 1,500
		match := generalPointsRegex.FindStringSubmatch(line)
		if len(match) == 3 {
			idStr := match[1]
			ptsStr := strings.ReplaceAll(strings.ReplaceAll(match[2], ",", ""), ".", "")
			tID, _ := strconv.ParseInt(idStr, 10, 64)
			pts, err := strconv.ParseFloat(ptsStr, 64)
			if err == nil {
				targetID = tID
				points = pts
				u, err := s.User(idStr)
				if err == nil {
					targetUser = u
					matched = true
				}
			}
		}

		if !matched && strings.Contains(line, "•") {
			parts := strings.Split(line, "•")
			if len(parts) >= 2 {
				usernamePart := strings.TrimSpace(parts[0])
				if strings.HasPrefix(usernamePart, "@") {
					usernamePart = usernamePart[1:]
				}
				// Attempt to remove prefixed numbering "1. @user"
				if partsName := strings.SplitN(usernamePart, " ", 2); len(partsName) == 2 && strings.HasSuffix(partsName[0], ".") {
					usernamePart = strings.TrimSpace(partsName[1])
				}

				ptsStr := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(parts[1]), ",", ""), ".", "")
				pts, err := strconv.ParseFloat(ptsStr, 64)
				if err == nil {
					points = pts
					// Find user by name
					if mem, ok := membersCache[strings.ToLower(usernamePart)]; ok {
						tID, _ := strconv.ParseInt(mem.User.ID, 10, 64)
						targetID = tID
						targetUser = mem.User
						matched = true
					}
				}
			}
		}

		if !matched {
			if strings.Contains(line, "•") {
				failed++
			}
			continue
		}

		finalPoints := points * multiplierVal

		// Get user cult
		var userCult models.Cult
		database.ColCults.FindOne(ctx, bson.M{"guild_id": guildID, "members": targetID, "active": true}).Decode(&userCult)
		
		cultIDStr := ""
		cultNameStr := ""
		if userCult.ID != primitive.NilObjectID {
			cultIDStr = userCult.ID.Hex()
			cultNameStr = userCult.CultName
		}

		// Insert Transaction
		trans := models.Transaction{
			UserID:         targetID,
			UserName:       targetUser.Username,
			GuildID:        guildID,
			GuildName:      "",
			Amount:         finalPoints,
			BaseAmount:     points,
			MultiplierUsed: multiplierVal,
			Type:           "adminpoints",
			Timestamp:      time.Now().UTC(),
		}
		if cultIDStr != "" {
			trans.CultID = &cultIDStr
			trans.CultName = &cultNameStr
		}

		_, err = database.ColPoints.InsertOne(ctx, trans)
		if err == nil {
			processed++
			successDetails = append(successDetails, fmt.Sprintf("%s: %.0f points", targetUser.Username, finalPoints))
		} else {
			failed++
		}
	}

	if processed > 0 {
		desc := fmt.Sprintf("Successfully processed **%d** users\nFailed: **%d** entries", processed, failed)
		embed := &discordgo.MessageEmbed{
			Title:       "✅ Admin Points Added",
			Description: desc,
			Color:       0x00ff00,
		}
		if len(successDetails) > 0 {
			max := 10
			if len(successDetails) < 10 {
				max = len(successDetails)
			}
			detailStr := ""
			for i := 0; i < max; i++ {
				detailStr += successDetails[i] + "\n"
			}
			if len(successDetails) > 10 {
				detailStr += fmt.Sprintf("...and %d more", len(successDetails)-10)
			}
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:  "Details",
				Value: detailStr,
			})
		}

		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{embed},
		})
	} else {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{{
				Title:       "❌ No Points Added",
				Description: fmt.Sprintf("Failed to process any users from the message\nFailed entries: %d", failed),
				Color:       0xff0000,
			}},
		})
	}
}
