package commands

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
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
			Name:        "rewardrole",
			Description: "Create a new reward role milestone (Admin only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "role",
					Description: "The role to give",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "amount",
					Description: "Amount of points/wins needed",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "type",
					Description: "points or wins",
					Required:    true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "Points", Value: "points"},
						{Name: "Wins", Value: "wins"},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionChannel,
					Name:        "channel",
					Description: "Channel to announce milestone",
					Required:    true,
				},
			},
		},
		&discordgo.ApplicationCommand{
			Name:        "set_reward_stackable",
			Description: "Toggle whether reward milestone roles stack or only highest is kept (Admin only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "stackable",
					Description: "Should roles stack?",
					Required:    true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "Yes (Stackable)", Value: "yes"},
						{Name: "No (Replace Previous)", Value: "no"},
					},
				},
			},
		},
		&discordgo.ApplicationCommand{
			Name:        "listrewards",
			Description: "List all configured reward roles (Admin only)",
		},
		&discordgo.ApplicationCommand{
			Name:        "rolelist",
			Description: "Show all reward role milestones (public)",
		},
		&discordgo.ApplicationCommand{
			Name:        "deletereward",
			Description: "Delete a reward role milestone (Admin only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "role",
					Description: "The reward role to remove",
					Required:    true,
				},
			},
		},
		&discordgo.ApplicationCommand{
			Name:        "editrewardrole",
			Description: "Edit an existing reward role's amount or channel (Admin only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "role",
					Description: "The reward role to edit",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "new_amount",
					Description: "New amount threshold required",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionChannel,
					Name:        "new_channel",
					Description: "New announcement channel (optional)",
					Required:    false,
				},
			},
		},
		&discordgo.ApplicationCommand{
			Name:        "force_refresh_rewards",
			Description: "Retroactively assign reward roles to all eligible members (Admin only)",
		},
	)

	CommandHandlers["rewardrole"] = rewardRoleHandler
	CommandHandlers["set_reward_stackable"] = setRewardStackableHandler
	CommandHandlers["listrewards"] = listRewardsHandler
	CommandHandlers["rolelist"] = roleListHandler
	CommandHandlers["deletereward"] = deleteRewardHandler
	CommandHandlers["editrewardrole"] = editRewardRoleHandler
	CommandHandlers["force_refresh_rewards"] = forceRefreshRewardsHandler
}

func rewardRoleHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}
	
	isAdmin := false
	if i.Member.Permissions&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator {
		isAdmin = true
	}

	if !isAdmin && !IsBotOwner(usr.ID) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Administrator permission required!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	roleIDStr := i.ApplicationCommandData().Options[0].RoleValue(s, i.GuildID).ID
	amount := i.ApplicationCommandData().Options[1].IntValue()
	rType := i.ApplicationCommandData().Options[2].StringValue()
	channelIDStr := i.ApplicationCommandData().Options[3].ChannelValue(s).ID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	roleID, _ := strconv.ParseInt(roleIDStr, 10, 64)
	channelID, _ := strconv.ParseInt(channelIDStr, 10, 64)

	// Check if already exists
	count, _ := database.ColRewardRoles.CountDocuments(ctx, bson.M{
		"guild_id": guildID,
		"role_id":  roleID,
		"active":   true,
	})

	if count > 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ This role is already set as a reward!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	_, err := database.ColRewardRoles.InsertOne(ctx, bson.M{
		"guild_id":   guildID,
		"role_id":    roleID,
		"channel_id": channelID,
		"amount":     float64(amount),
		"type":       rType,
		"active":     true,
	})

	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Error connecting to database.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "✅ Reward Role Configured",
		Description: fmt.Sprintf("Users will receive <@&%s> when reaching **%d** %s.\nAnnouncements will be sent in <#%s>.", roleIDStr, amount, rType, channelIDStr),
		Color:       0x00ff00,
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func setRewardStackableHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}
	
	isAdmin := (i.Member.Permissions & discordgo.PermissionAdministrator) == discordgo.PermissionAdministrator
	if !isAdmin && !IsBotOwner(usr.ID) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Administrator permission required!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	stackableStr := i.ApplicationCommandData().Options[0].StringValue()
	stackable := stackableStr == "yes"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)

	opts := options.Update().SetUpsert(true)
	_, err := database.ColGuildSettings.UpdateOne(ctx,
		bson.M{"guild_id": guildID},
		bson.M{"$set": bson.M{"reward_stackable": stackable}},
		opts,
	)

	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Failed to update settings!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	status := "stackable (all milestones kept)"
	if !stackable {
		status = "non-stackable (only highest milestone kept)"
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("✅ Reward roles are now **%s**.", status),
		},
	})
}

// ─── /listrewards ─────────────────────────────────────────────────────────────

func listRewardsHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}
	isAdmin := (i.Member.Permissions & discordgo.PermissionAdministrator) == discordgo.PermissionAdministrator
	if !isAdmin && !IsBotOwner(usr.ID) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Administrator permission required!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cur, err := database.ColRewardRoles.Find(ctx, bson.M{"guild_id": guildID, "active": true})
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Database error.", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}
	var rewards []models.RewardRole
	cur.All(ctx, &rewards)

	if len(rewards) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ No reward roles configured for this server.", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	// Sort: points first (by amount), then wins (by amount)
	sort.Slice(rewards, func(a, b int) bool {
		if rewards[a].Type != rewards[b].Type {
			return rewards[a].Type < rewards[b].Type // "points" < "wins" alphabetically
		}
		return rewards[a].Amount < rewards[b].Amount
	})

	var fields []*discordgo.MessageEmbedField
	for idx, r := range rewards {
		label := fmt.Sprintf("%d. %.0f %s", idx+1, r.Amount, strings.Title(r.Type))
		val := fmt.Sprintf("Role: <@&%d>\nChannel: <#%d>", r.RoleID, r.ChannelID)
		fields = append(fields, &discordgo.MessageEmbedField{Name: label, Value: val, Inline: true})
		if len(fields) >= 24 { // Discord embed field limit safety
			break
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:  "📋 Configured Reward Roles",
		Color:  0x00aaff,
		Fields: fields,
		Footer: &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Total: %d reward roles", len(rewards))},
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
}

// ─── /rolelist ────────────────────────────────────────────────────────────────

func roleListHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cur, err := database.ColRewardRoles.Find(ctx, bson.M{"guild_id": guildID, "active": true})
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Database error.", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}
	var rewards []models.RewardRole
	cur.All(ctx, &rewards)

	if len(rewards) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "No reward roles have been configured for this server.", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	sort.Slice(rewards, func(a, b int) bool {
		if rewards[a].Type != rewards[b].Type {
			return rewards[a].Type < rewards[b].Type
		}
		return rewards[a].Amount < rewards[b].Amount
	})

	var ptsLines, winLines []string
	for _, r := range rewards {
		line := fmt.Sprintf("<@&%d> — **%.0f** %s", r.RoleID, r.Amount, r.Type)
		if r.Type == "points" {
			ptsLines = append(ptsLines, line)
		} else {
			winLines = append(winLines, line)
		}
	}

	var fields []*discordgo.MessageEmbedField
	if len(ptsLines) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "📊 Points Milestones",
			Value:  strings.Join(ptsLines, "\n"),
			Inline: false,
		})
	}
	if len(winLines) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "🏆 Wins Milestones",
			Value:  strings.Join(winLines, "\n"),
			Inline: false,
		})
	}

	embed := &discordgo.MessageEmbed{
		Title:  "🎖️ Reward Role Milestones",
		Color:  0xffa500,
		Fields: fields,
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

// ─── /deletereward ────────────────────────────────────────────────────────────

func deleteRewardHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
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

	roleObj := i.ApplicationCommandData().Options[0].RoleValue(s, i.GuildID)
	roleIDStr := roleObj.ID
	roleID, _ := strconv.ParseInt(roleIDStr, 10, 64)
	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := database.ColRewardRoles.DeleteOne(ctx, bson.M{
		"guild_id": guildID,
		"role_id":  roleID,
		"active":   true,
	})

	if err != nil || res.DeletedCount == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ No active reward found for <@&%s>.", roleIDStr),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{{
				Description: fmt.Sprintf("✅ Removed reward milestone for <@&%s>.", roleIDStr),
				Color:       0xff4444,
			}},
		},
	})
}

// ─── /editrewardrole ──────────────────────────────────────────────────────────

func editRewardRoleHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
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

	opts := i.ApplicationCommandData().Options
	roleObj := opts[0].RoleValue(s, i.GuildID)
	roleIDStr := roleObj.ID
	roleID, _ := strconv.ParseInt(roleIDStr, 10, 64)
	newAmount := opts[1].FloatValue()
	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)

	if newAmount <= 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Amount must be greater than 0.", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	updateSet := bson.M{"amount": newAmount}
	// Optional new channel
	for _, opt := range opts[2:] {
		if opt.Name == "new_channel" {
			chIDStr := opt.ChannelValue(s).ID
			chID, _ := strconv.ParseInt(chIDStr, 10, 64)
			updateSet["channel_id"] = chID
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := database.ColRewardRoles.UpdateOne(ctx,
		bson.M{"guild_id": guildID, "role_id": roleID, "active": true},
		bson.M{"$set": updateSet},
	)

	if err != nil || res.MatchedCount == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ No active reward found for <@&%s>.", roleIDStr),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	desc := fmt.Sprintf("✅ Updated reward for <@&%s> — new threshold: **%.0f**.", roleIDStr, newAmount)
	if chID, ok := updateSet["channel_id"]; ok {
		desc += fmt.Sprintf("\nAnnouncements will now go to <#%d>.", chID)
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{{Description: desc, Color: 0x00ff00}},
		},
	})
}

// ─── /force_refresh_rewards ───────────────────────────────────────────────────

func forceRefreshRewardsHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
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

	// Defer — this can take a while
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fetch all active rewards for this guild
	cur, err := database.ColRewardRoles.Find(ctx, bson.M{"guild_id": guildID, "active": true})
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: stringPtr("❌ Database error fetching rewards.")})
		return
	}
	var allRewards []models.RewardRole
	cur.All(ctx, &allRewards)
	if len(allRewards) == 0 {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: stringPtr("❌ No reward roles configured for this server.")})
		return
	}

	// For each type, aggregate user totals
	processedCount := 0
	for _, rType := range []string{"points", "wins"} {
		col := database.ColPoints
		if rType == "wins" {
			col = database.ColWins
		}
		pipeline := []bson.M{
			{"$match": bson.M{"guild_id": guildID}},
			{"$group": bson.M{"_id": "$user_id", "total": bson.M{"$sum": "$amount"}}},
		}
		aggCur, err := col.Aggregate(ctx, pipeline)
		if err != nil {
			continue
		}
		var userTotals []struct {
			UserID int64   `bson:"_id"`
			Total  float64 `bson:"total"`
		}
		aggCur.All(ctx, &userTotals)

		for _, ut := range userTotals {
			// Fire the standard reward check per user — runs in background goroutine-safe
			go CheckAndAssignRewards(s, guildID, ut.UserID, rType)
			processedCount++
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:       "✅ Reward Refresh Queued",
		Description: fmt.Sprintf("Queued reward evaluations for **%d** user-type combinations.\nRole assignments will be applied shortly.", processedCount),
		Color:       0x00ff00,
	}
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
}
