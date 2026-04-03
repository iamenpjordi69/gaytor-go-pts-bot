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

// addHandler handles /add which natively replaces point adding and win adding logic
func addHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}

	targetUser := usr
	pointsToAdd := 0.0
	winsToAdd := 1.0

	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "user" {
			targetUser = opt.UserValue(s)
		} else if opt.Name == "points" {
			pointsToAdd = opt.FloatValue()
		} else if opt.Name == "wins" {
			winsToAdd = float64(opt.IntValue())
		}
	}

	// Permission check if trying to add to someone else or doing massive amounts
	if targetUser.ID != usr.ID {
		canManage := (i.Member.Permissions & discordgo.PermissionAdministrator) == discordgo.PermissionAdministrator
		if !canManage && !IsBotOwner(usr.ID) {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "❌ You need Administrator permissions to add points to another user!",
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

	// Multiplier logic
	multiplier := 1.0
	var mult models.Multiplier
	if err := database.ColMultipliers.FindOne(ctx, bson.M{"guild_id": guildID, "active": true}).Decode(&mult); err == nil {
		multiplier = mult.Multiplier
	}

	finalPoints := pointsToAdd * multiplier

	// Find cult for tracking
	var userCult models.Cult
	database.ColCults.FindOne(ctx, bson.M{"guild_id": guildID, "members": targetID, "active": true}).Decode(&userCult)
	cultIDStr := ""
	if userCult.ID.Hex() != "000000000000000000000000" {
		cultIDStr = userCult.ID.Hex()
	}

	guildName := ""
	guild, err := s.Guild(i.GuildID)
	if err == nil {
		guildName = guild.Name
	}

	// Save transactions
	if finalPoints > 0 {
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
			Timestamp:      time.Now().UTC(),
		})
	}
	
	if winsToAdd > 0 {
		database.ColWins.InsertOne(ctx, models.Transaction{
			UserID:         targetID,
			UserName:       targetUser.Username,
			GuildID:        guildID,
			GuildName:      guildName,
			Amount:         winsToAdd,
			CultID:         &cultIDStr,
			CultName:       &userCult.CultName,
			Type:           "add",
			Timestamp:      time.Now().UTC(),
		})
	}

	userPoints := fetchTotal(ctx, targetID, guildID, "points")
	userWins := fetchTotal(ctx, targetID, guildID, "wins")

	embed := &discordgo.MessageEmbed{
		Color: 0x00ff00,
		Title: fmt.Sprintf("✅ Points Added to %s", targetUser.Username),
		Description: fmt.Sprintf("%.1f points and %.0f wins added to balance.\n**New Points:** %.1f\n**New Wins:** %.0f", finalPoints, winsToAdd, userPoints, userWins),
	}
	if multiplier > 1 {
		embed.Description += fmt.Sprintf("\n*Multiplier:* %vx applied", multiplier)
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

// removeHandler consolidates point subtraction
func removeHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}

	targetUser := usr
	pointsToRemove := 0.0
	winsToRemove := 0.0

	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "user" {
			targetUser = opt.UserValue(s)
		} else if opt.Name == "points" {
			pointsToRemove = opt.FloatValue()
		} else if opt.Name == "wins" {
			winsToRemove = float64(opt.IntValue())
		}
	}

	// Only admin or owner can modify others
	if targetUser.ID != usr.ID {
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
	}

	if pointsToRemove <= 0 && winsToRemove <= 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Please provide a positive number of points or wins to remove.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	targetID, _ := strconv.ParseInt(targetUser.ID, 10, 64)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	guildName := "Server"
	guild, err := s.Guild(i.GuildID)
	if err == nil {
		guildName = guild.Name
	}

	if pointsToRemove > 0 {
		database.ColPoints.InsertOne(ctx, models.Transaction{
			UserID:         targetID,
			UserName:       targetUser.Username,
			GuildID:        guildID,
			GuildName:      guildName,
			Amount:         -pointsToRemove, // negative logic
			Type:           "remove",
			Timestamp:      time.Now().UTC(),
		})
	}
	
	if winsToRemove > 0 {
		database.ColWins.InsertOne(ctx, models.Transaction{
			UserID:         targetID,
			UserName:       targetUser.Username,
			GuildID:        guildID,
			GuildName:      guildName,
			Amount:         -winsToRemove,
			Type:           "remove",
			Timestamp:      time.Now().UTC(),
		})
	}

	userPoints := fetchTotal(ctx, targetID, guildID, "points")
	userWins := fetchTotal(ctx, targetID, guildID, "wins")

	embed := &discordgo.MessageEmbed{
		Color: 0xff0000,
		Title: fmt.Sprintf("➖ Subtracted from %s", targetUser.Username),
		Description: fmt.Sprintf("%.1f points and %.0f wins were removed.\n**New Points:** %.1f\n**New Wins:** %.0f", pointsToRemove, winsToRemove, userPoints, userWins),
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}
