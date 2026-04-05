package commands

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go-cult-2025/internal/database"
	"go-cult-2025/internal/models"

	"github.com/bwmarrin/discordgo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func init() {
	SlashCommands = append(SlashCommands,
		&discordgo.ApplicationCommand{
			Name:        "set_multiplier",
			Description: "Set a server-wide points multiplier (Admin only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionNumber,
					Name:        "multiplier",
					Description: "Multiplier value e.g. 1.5, 2.0",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "description",
					Description: "Why is this multiplier active? (e.g. Weekend bonus)",
					Required:    false,
				},
			},
		},
		&discordgo.ApplicationCommand{
			Name:        "end_multiplier",
			Description: "Deactivate the current server multiplier (Admin only)",
		},
		&discordgo.ApplicationCommand{
			Name:        "multiplier_info",
			Description: "Show information about the current server multiplier",
		},
		&discordgo.ApplicationCommand{
			Name:        "edit_multiplier",
			Description: "Edit the active multiplier value or description (Admin only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionNumber,
					Name:        "multiplier",
					Description: "New multiplier value (1–20)",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "description",
					Description: "New description for this multiplier event",
					Required:    false,
				},
			},
		},
		&discordgo.ApplicationCommand{
			Name:        "debug_rewards",
			Description: "Debug your reward role eligibility (Admin only)",
		},
	)

	CommandHandlers["set_multiplier"] = setMultiplierHandler
	CommandHandlers["end_multiplier"] = endMultiplierHandler
	CommandHandlers["multiplier_info"] = multiplierInfoHandler
	CommandHandlers["edit_multiplier"] = editMultiplierHandler
	CommandHandlers["debug_rewards"] = debugRewardsHandler
}


func setMultiplierHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	canManage := (i.Member.Permissions & discordgo.PermissionAdministrator) == discordgo.PermissionAdministrator
	if !canManage {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Administrator permissions required!", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	val := i.ApplicationCommandData().Options[0].FloatValue()
	description := ""
	for _, opt := range i.ApplicationCommandData().Options[1:] {
		if opt.Name == "description" {
			description = opt.StringValue()
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	upsertOpts := options.Update().SetUpsert(true)
	setFields := bson.M{
		"guild_id":   guildID,
		"multiplier": val,
		"active":     true,
		"set_at":     time.Now().UTC(),
	}
	if description != "" {
		setFields["description"] = description
	}

	_, err := database.ColMultipliers.UpdateOne(ctx,
		bson.M{"guild_id": guildID},
		bson.M{"$set": setFields},
		upsertOpts,
	)

	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Configuration saving failed!", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	desc := fmt.Sprintf("✅ Multiplier set to **%.2fx** for this server.", val)
	if description != "" {
		desc += fmt.Sprintf("\n📝 **Reason:** %s", description)
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: desc},
	})
}

// ─── /end_multiplier ──────────────────────────────────────────────────────────

func endMultiplierHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	canManage := (i.Member.Permissions & discordgo.PermissionAdministrator) == discordgo.PermissionAdministrator
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}
	if !canManage && !IsBotOwner(usr.ID) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Administrator permissions required!", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Fetch current multiplier first to show the old value
	var existing models.Multiplier
	err := database.ColMultipliers.FindOne(ctx, bson.M{"guild_id": guildID, "active": true}).Decode(&existing)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ No active multiplier found for this server.", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	_, err = database.ColMultipliers.UpdateOne(ctx,
		bson.M{"guild_id": guildID, "active": true},
		bson.M{"$set": bson.M{"active": false, "ended_at": time.Now().UTC()}},
	)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Failed to deactivate multiplier.", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🛑 Multiplier Ended",
		Description: fmt.Sprintf("The **%.2fx** multiplier has been deactivated.\nPoints will now be added at the normal rate (1x).", existing.Multiplier),
		Color:       0xff4444,
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}},
	})
}

// ─── /multiplier_info ─────────────────────────────────────────────────────────

func multiplierInfoHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var mult models.Multiplier
	err := database.ColMultipliers.FindOne(ctx, bson.M{"guild_id": guildID, "active": true}).Decode(&mult)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{{
					Title:       "❌ No Active Multiplier",
					Description: "No multiplier is currently active in this server.\nUse `/set_multiplier` to enable one.",
					Color:       0x555555,
				}},
			},
		})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:  fmt.Sprintf("✅ Active Multiplier: %.2fx", mult.Multiplier),
		Color:  0xffa500,
		Fields: []*discordgo.MessageEmbedField{},
	}
	if mult.Description != "" {
		embed.Description = mult.Description
	}
	if !mult.SetAt.IsZero() {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Started",
			Value:  fmt.Sprintf("<t:%d:R>", mult.SetAt.Unix()),
			Inline: true,
		})
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}},
	})
}

// ─── /edit_multiplier ─────────────────────────────────────────────────────────

func editMultiplierHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	canManage := (i.Member.Permissions & discordgo.PermissionAdministrator) == discordgo.PermissionAdministrator
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}
	if !canManage && !IsBotOwner(usr.ID) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Administrator permissions required!", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	newVal := i.ApplicationCommandData().Options[0].FloatValue()
	newDesc := ""
	for _, opt := range i.ApplicationCommandData().Options[1:] {
		if opt.Name == "description" {
			newDesc = opt.StringValue()
		}
	}

	if newVal < 1 || newVal > 20 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Multiplier must be between 1 and 20.", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check an active one exists
	var existing models.Multiplier
	if err := database.ColMultipliers.FindOne(ctx, bson.M{"guild_id": guildID, "active": true}).Decode(&existing); err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ No active multiplier found. Use `/set_multiplier` first.", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	setFields := bson.M{"multiplier": newVal, "edited_at": time.Now().UTC()}
	if newDesc != "" {
		setFields["description"] = newDesc
	}

	_, err := database.ColMultipliers.UpdateOne(ctx,
		bson.M{"guild_id": guildID, "active": true},
		bson.M{"$set": setFields},
	)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Failed to update multiplier.", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title: "✏️ Multiplier Updated",
		Color: 0xffa500,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Old Multiplier", Value: fmt.Sprintf("%.2fx", existing.Multiplier), Inline: true},
			{Name: "New Multiplier", Value: fmt.Sprintf("%.2fx", newVal), Inline: true},
		},
	}
	if newDesc != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Description", Value: newDesc, Inline: false})
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}},
	})
}

// ─── /debug_rewards ───────────────────────────────────────────────────────────

func debugRewardsHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}
	isAdmin := (i.Member.Permissions & discordgo.PermissionAdministrator) == discordgo.PermissionAdministrator
	if !isAdmin && !IsBotOwner(usr.ID) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Administrator permission required!", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})

	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	userID, _ := strconv.ParseInt(usr.ID, 10, 64)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cur, _ := database.ColRewardRoles.Find(ctx, bson.M{"guild_id": guildID, "active": true})
	var rewards []models.RewardRole
	cur.All(ctx, &rewards)

	if len(rewards) == 0 {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: stringPtr("❌ No reward roles configured for this server.")})
		return
	}

	totalPoints := fetchTotal(ctx, userID, guildID, "points")
	totalWins := fetchTotal(ctx, userID, guildID, "wins")

	var lines []string
	lines = append(lines, fmt.Sprintf("**Your Totals:** %.0f points | %.0f wins", totalPoints, totalWins))
	lines = append(lines, "")

	for _, r := range rewards {
		userTotal := totalPoints
		if r.Type == "wins" {
			userTotal = totalWins
		}
		eligible := userTotal >= r.Amount
		egStr := "❌ Not eligible"
		if eligible {
			egStr = "✅ Eligible"
		}
		lines = append(lines, fmt.Sprintf("**<@&%d>** (%.0f %s) — %s (%.0f/%.0f)",
			r.RoleID, r.Amount, r.Type, egStr, userTotal, r.Amount))
	}

	desc := ""
	for _, l := range lines {
		desc += l + "\n"
	}
	if len(desc) > 4000 {
		desc = desc[:4000] + "..."
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🔍 Reward Debug Info",
		Description: desc,
		Color:       0x00aaff,
	}
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
}

