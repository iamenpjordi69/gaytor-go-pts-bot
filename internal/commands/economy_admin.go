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
)

func init() {
	SlashCommands = append(SlashCommands, 
		&discordgo.ApplicationCommand{
			Name:        "add",
			Description: "Add points to an account",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionNumber,
					Name:        "points",
					Description: "Points to add",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "user",
					Description: "Target user (Admin only, leave blank for self)",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "wins",
					Description: "Number of wins to add alongside points (default 1 if omitted)",
					Required:    false,
				},
			},
		},
		&discordgo.ApplicationCommand{
			Name:        "remove",
			Description: "Remove points or wins from an account",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "user",
					Description: "Target user (Admin only, leave blank for self)",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionNumber,
					Name:        "points",
					Description: "Points to remove",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "wins",
					Description: "Wins to remove",
					Required:    false,
				},
			},
		},
		&discordgo.ApplicationCommand{
			Name:        "cleanup_roles",
			Description: "Remove duplicate milestone roles, keep only highest (Admin only)",
		},
	)

	CommandHandlers["add"] = addHandler
	CommandHandlers["remove"] = removeHandler
	CommandHandlers["cleanup_roles"] = cleanupRolesHandler
}

// cleanupRolesHandler removes duplicate milestone roles keeping only the highest
func cleanupRolesHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}
	
	// Admin auth
	canManage := (i.Member.Permissions & discordgo.PermissionAdministrator) == discordgo.PermissionAdministrator
	if !canManage && !IsBotOwner(usr.ID) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Administrator permissions required!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}
	
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Get all active reward roles for this server
	cur, err := database.ColRewardRoles.Find(ctx, bson.M{"guild_id": guildID, "active": true})
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: stringPtr("❌ Database error")})
		return
	}
	defer cur.Close(ctx)

	var rewards []models.RewardRole
	cur.All(ctx, &rewards)

	if len(rewards) == 0 {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: stringPtr("❌ No reward roles found!")})
		return
	}

	var pointsRewards []models.RewardRole
	var winsRewards []models.RewardRole
	for _, r := range rewards {
		if r.Type == "points" {
			pointsRewards = append(pointsRewards, r)
		} else {
			winsRewards = append(winsRewards, r)
		}
	}

	var cleanedUsers int

	// Loop over all members in the guild (paged if large, but SDK handles basic caching if initialized)
	members, err := s.GuildMembers(i.GuildID, "", 1000)
	if err == nil {
		for _, m := range members {
			needsClean := false
			
			// Points check
			if len(pointsRewards) > 1 {
				var hasReward []models.RewardRole
				for _, reward := range pointsRewards {
					roleIDStr := strconv.FormatInt(reward.RoleID, 10)
					if containsStr(m.Roles, roleIDStr) {
						hasReward = append(hasReward, reward)
					}
				}
				if len(hasReward) > 1 {
					highest := hasReward[0]
					for _, rev := range hasReward {
						if rev.Amount > highest.Amount {
							highest = rev
						}
					}
					// Remove lesser roles
					for _, rev := range hasReward {
						if rev.RoleID != highest.RoleID {
							s.GuildMemberRoleRemove(i.GuildID, m.User.ID, strconv.FormatInt(rev.RoleID, 10))
							needsClean = true
						}
					}
				}
			}

			// Wins check
			if len(winsRewards) > 1 {
				var hasReward []models.RewardRole
				for _, reward := range winsRewards {
					roleIDStr := strconv.FormatInt(reward.RoleID, 10)
					if containsStr(m.Roles, roleIDStr) {
						hasReward = append(hasReward, reward)
					}
				}
				if len(hasReward) > 1 {
					highest := hasReward[0]
					for _, rev := range hasReward {
						if rev.Amount > highest.Amount {
							highest = rev
						}
					}
					for _, rev := range hasReward {
						if rev.RoleID != highest.RoleID {
							s.GuildMemberRoleRemove(i.GuildID, m.User.ID, strconv.FormatInt(rev.RoleID, 10))
							needsClean = true
						}
					}
				}
			}

			if needsClean {
				cleanedUsers++
			}
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:       "✅ Role Cleanup Complete",
		Description: fmt.Sprintf("Cleaned up milestone roles for %d users.\nEach user now has only their highest points role and highest wins role.", cleanedUsers),
		Color:       0x00ff00,
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func containsStr(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func stringPtr(val string) *string {
	return &val
}

// addHandler handles /add — adds points and/or wins to a user's account
func addHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}

	targetUser := usr
	pointsToAdd := 0.0
	winsToAdd := 0.0           // default 0 — only added if explicitly provided
	winsExplicit := false      // track if user actually passed the wins option

	for _, opt := range i.ApplicationCommandData().Options {
		switch opt.Name {
		case "user":
			targetUser = opt.UserValue(s)
		case "points":
			pointsToAdd = opt.FloatValue()
		case "wins":
			winsToAdd = float64(opt.IntValue())
			winsExplicit = true
		}
	}

	// Validate points
	if pointsToAdd <= 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Points must be a positive number.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Only admins/owner can add to OTHER users — anyone can add to themselves
	if targetUser.ID != usr.ID {
		canManage := (i.Member.Permissions & discordgo.PermissionAdministrator) == discordgo.PermissionAdministrator
		if !canManage && !IsBotOwner(usr.ID) {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "❌ Administrator permissions required to add points to another user!",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}
	}

	// Only admins can explicitly set wins — wins are earned, not self-assigned
	if winsExplicit {
		canManage := (i.Member.Permissions & discordgo.PermissionAdministrator) == discordgo.PermissionAdministrator
		if !canManage && !IsBotOwner(usr.ID) {
			// Silently ignore the wins amount for non-admins
			winsExplicit = false
			winsToAdd = 0
		} else if winsToAdd < 0 {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "❌ Wins cannot be negative.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}
	}

	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	targetID, _ := strconv.ParseInt(targetUser.ID, 10, 64)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Multiplier
	multiplier := 1.0
	var mult models.Multiplier
	if err := database.ColMultipliers.FindOne(ctx, bson.M{"guild_id": guildID, "active": true}).Decode(&mult); err == nil {
		multiplier = mult.Multiplier
	}

	finalPoints := pointsToAdd * multiplier

	// Cult tracking
	var userCult models.Cult
	database.ColCults.FindOne(ctx, bson.M{"guild_id": guildID, "members": targetID, "active": true}).Decode(&userCult)
	cultIDStr := ""
	if !userCult.ID.IsZero() {
		cultIDStr = userCult.ID.Hex()
	}

	guildName := ""
	if guild, err := s.Guild(i.GuildID); err == nil {
		guildName = guild.Name
	}

	now := time.Now().UTC()

	// Insert points transaction
	database.ColPoints.InsertOne(ctx, models.Transaction{
		UserID:         targetID,
		UserName:       targetUser.Username,
		GuildID:        guildID,
		GuildName:      guildName,
		Amount:         finalPoints,
		BaseAmount:     pointsToAdd,
		MultiplierUsed: multiplier,
		CultID:         &cultIDStr,
		CultName:       &userCult.CultName,
		Type:           "add",
		Timestamp:      now,
	})

	// Insert wins transaction only if explicitly provided
	if winsExplicit && winsToAdd > 0 {
		database.ColWins.InsertOne(ctx, models.Transaction{
			UserID:    targetID,
			UserName:  targetUser.Username,
			GuildID:   guildID,
			GuildName: guildName,
			Amount:    winsToAdd,
			CultID:    &cultIDStr,
			CultName:  &userCult.CultName,
			Type:      "add",
			Timestamp: now,
		})
	}

	// Fetch updated totals with a fresh context so the cancelled ctx doesn't cause a stale read
	totalCtx, totalCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer totalCancel()
	newPoints := fetchTotal(totalCtx, targetID, guildID, "points")
	newWins := fetchTotal(totalCtx, targetID, guildID, "wins")

	// Build response message
	addedDesc := fmt.Sprintf("**+%.1f points**", finalPoints)
	if multiplier > 1 {
		addedDesc += fmt.Sprintf(" *(base %.1f × %.1fx multiplier)*", pointsToAdd, multiplier)
	}
	if winsExplicit && winsToAdd > 0 {
		addedDesc += fmt.Sprintf(" and **+%.0f wins**", winsToAdd)
	}

	embed := &discordgo.MessageEmbed{
		Color: 0x00ff00,
		Title: fmt.Sprintf("✅ Points Added — %s", targetUser.Username),
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Added", Value: addedDesc, Inline: false},
			{Name: "New Points", Value: fmt.Sprintf("%.1f", newPoints), Inline: true},
			{Name: "New Wins", Value: fmt.Sprintf("%.0f", newWins), Inline: true},
		},
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})

	// Reward check in background
	go CheckAndAssignRewards(s, guildID, targetID, "points")
	if winsExplicit && winsToAdd > 0 {
		go CheckAndAssignRewards(s, guildID, targetID, "wins")
	}
}

// removeHandler handles /remove — subtracts points and/or wins from a user's account
func removeHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}

	targetUser := usr
	pointsToRemove := 0.0
	winsToRemove := 0.0

	for _, opt := range i.ApplicationCommandData().Options {
		switch opt.Name {
		case "user":
			targetUser = opt.UserValue(s)
		case "points":
			pointsToRemove = opt.FloatValue()
		case "wins":
			winsToRemove = float64(opt.IntValue())
		}
	}

	// Must provide at least one of points or wins
	if pointsToRemove <= 0 && winsToRemove <= 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Please provide a positive number of points and/or wins to remove.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Only admin/owner can modify other users
	if targetUser.ID != usr.ID {
		canManage := (i.Member.Permissions & discordgo.PermissionAdministrator) == discordgo.PermissionAdministrator
		if !canManage && !IsBotOwner(usr.ID) {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "❌ Administrator permissions required to remove from another user!",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}
	}

	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	targetID, _ := strconv.ParseInt(targetUser.ID, 10, 64)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	guildName := ""
	if guild, err := s.Guild(i.GuildID); err == nil {
		guildName = guild.Name
	}

	now := time.Now().UTC()

	if pointsToRemove > 0 {
		database.ColPoints.InsertOne(ctx, models.Transaction{
			UserID:    targetID,
			UserName:  targetUser.Username,
			GuildID:   guildID,
			GuildName: guildName,
			Amount:    -pointsToRemove, // negative to subtract from aggregate sum
			Type:      "remove",
			Timestamp: now,
		})
	}

	if winsToRemove > 0 {
		database.ColWins.InsertOne(ctx, models.Transaction{
			UserID:    targetID,
			UserName:  targetUser.Username,
			GuildID:   guildID,
			GuildName: guildName,
			Amount:    -winsToRemove,
			Type:      "remove",
			Timestamp: now,
		})
	}

	// Fresh context for reading totals
	totalCtx, totalCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer totalCancel()
	newPoints := fetchTotal(totalCtx, targetID, guildID, "points")
	newWins := fetchTotal(totalCtx, targetID, guildID, "wins")

	// Build contextual removed line — only mention what was actually removed
	var removedParts []string
	if pointsToRemove > 0 {
		removedParts = append(removedParts, fmt.Sprintf("**-%.1f points**", pointsToRemove))
	}
	if winsToRemove > 0 {
		removedParts = append(removedParts, fmt.Sprintf("**-%.0f wins**", winsToRemove))
	}
	removedStr := ""
	for idx, p := range removedParts {
		if idx > 0 {
			removedStr += " and "
		}
		removedStr += p
	}

	embed := &discordgo.MessageEmbed{
		Color: 0xff4444,
		Title: fmt.Sprintf("➖ Points Removed — %s", targetUser.Username),
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Removed", Value: removedStr, Inline: false},
			{Name: "New Points", Value: fmt.Sprintf("%.1f", newPoints), Inline: true},
			{Name: "New Wins", Value: fmt.Sprintf("%.0f", newWins), Inline: true},
		},
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})

	// Re-evaluate rewards in background
	go CheckAndAssignRewards(s, guildID, targetID, "points")
	go CheckAndAssignRewards(s, guildID, targetID, "wins")
}

