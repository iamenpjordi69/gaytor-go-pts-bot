package commands

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"go-cult-2025/internal/database"
	"go-cult-2025/internal/models"

	"github.com/bwmarrin/discordgo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// CheckAndAssignRewards evaluates milestone qualifications and updates Discord roles.
// Always call this in a goroutine (go CheckAndAssignRewards(...)) so it never blocks
// command responses and always gets a fresh context independent of the caller.
func CheckAndAssignRewards(s *discordgo.Session, guildID int64, userID int64, rType string) {
	// Use a fresh context — never inherit from the calling handler's context
	// (the caller's context may already be cancelled by the time we run)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	guildIDStr := strconv.FormatInt(guildID, 10)
	userIDStr := strconv.FormatInt(userID, 10)

	// 1. Fetch the member first — bail early if they're not in the server
	member, err := s.GuildMember(guildIDStr, userIDStr)
	if err != nil {
		log.Printf("[RewardMgr] Cannot fetch member %s in guild %s: %v", userIDStr, guildIDStr, err)
		return
	}

	// 2. Fetch current total for this type
	total := fetchTotal(ctx, userID, guildID, rType)
	log.Printf("[RewardMgr] User %s total %s = %.2f", userIDStr, rType, total)

	// 3. Fetch ALL active reward roles of this type for this guild (sorted ascending by amount)
	findOpts := options.Find().SetSort(bson.D{{Key: "amount", Value: 1}})
	allCur, err := database.ColRewardRoles.Find(ctx, bson.M{
		"guild_id": guildID,
		"type":     rType,
		"active":   true,
	}, findOpts)
	if err != nil {
		log.Printf("[RewardMgr] DB error fetching reward roles: %v", err)
		return
	}
	var allRoles []models.RewardRole
	allCur.All(ctx, &allRoles)
	allCur.Close(ctx)

	if len(allRoles) == 0 {
		// No reward roles configured for this type — nothing to do
		return
	}

	// 4. Determine ALL roles the user qualifies for (amount <= total)
	var qualifying []models.RewardRole
	for _, r := range allRoles {
		if total >= r.Amount {
			qualifying = append(qualifying, r)
		}
	}

	// 5. Fetch guild stacking preference (default = false = non-stackable)
	var settings models.GuildSettings
	database.ColGuildSettings.FindOne(ctx, bson.M{"guild_id": guildID}).Decode(&settings)
	stackable := settings.RewardStackable

	// 6. Determine which role IDs the user SHOULD have
	shouldHaveSet := make(map[int64]bool)

	if stackable {
		// Stackable: user should have ALL qualifying roles
		for _, r := range qualifying {
			shouldHaveSet[r.RoleID] = true
		}
	} else {
		// Non-stackable: user should only have the HIGHEST qualifying role
		if len(qualifying) > 0 {
			highest := qualifying[len(qualifying)-1]
			shouldHaveSet[highest.RoleID] = true
		}
	}

	// 7. Build set of current member role IDs for O(1) lookup
	currentRoles := make(map[int64]bool)
	for _, rStr := range member.Roles {
		rid, err := strconv.ParseInt(rStr, 10, 64)
		if err == nil {
			currentRoles[rid] = true
		}
	}

	// 8. Apply additions and removals, scoped to reward roles only
	for _, r := range allRoles {
		roleIDStr := strconv.FormatInt(r.RoleID, 10)
		has := currentRoles[r.RoleID]
		want := shouldHaveSet[r.RoleID]

		if want && !has {
			// Add role
			if err := s.GuildMemberRoleAdd(guildIDStr, userIDStr, roleIDStr); err != nil {
				log.Printf("[RewardMgr] Failed to ADD role %s to user %s: %v", roleIDStr, userIDStr, err)
				// Notify the reward channel so admins can see and fix the issue
				go announceRoleFailure(s, guildID, userID, r.RoleID, rType, total)
			} else {
				log.Printf("[RewardMgr] ✅ Added role %s to user %s", roleIDStr, userIDStr)
				// Only announce congratulations when role was actually granted
				announceMilestone(s, guildID, userID, r.RoleID, rType, total)
			}
		} else if !want && has {
			// Remove role (only reward roles — not arbitrary server roles)
			if err := s.GuildMemberRoleRemove(guildIDStr, userIDStr, roleIDStr); err != nil {
				log.Printf("[RewardMgr] Failed to REMOVE role %s from user %s: %v", roleIDStr, userIDStr, err)
			} else {
				log.Printf("[RewardMgr] ➖ Removed role %s from user %s", roleIDStr, userIDStr)
			}
		}
	}
}

func announceMilestone(s *discordgo.Session, guildID int64, userID int64, roleID int64, rType string, amount float64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var reward models.RewardRole
	err := database.ColRewardRoles.FindOne(ctx, bson.M{
		"guild_id": guildID,
		"role_id":  roleID,
	}).Decode(&reward)
	if err != nil || reward.ChannelID == 0 {
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🎉 Milestone Reached!",
		Description: fmt.Sprintf("Congratulations <@%d>! You've reached **%.0f %s** and earned the <@&%d> role!", userID, amount, rType, roleID),
		Color:       0x00ff00,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	channelIDStr := strconv.FormatInt(reward.ChannelID, 10)
	if _, err := s.ChannelMessageSendEmbed(channelIDStr, embed); err != nil {
		log.Printf("[RewardMgr] Failed to announce milestone in channel %s: %v", channelIDStr, err)
	}
}

// announceRoleFailure sends a visible warning to the reward channel when the bot
// cannot assign a role due to a hierarchy or permissions issue.
func announceRoleFailure(s *discordgo.Session, guildID int64, userID int64, roleID int64, rType string, amount float64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var reward models.RewardRole
	err := database.ColRewardRoles.FindOne(ctx, bson.M{
		"guild_id": guildID,
		"role_id":  roleID,
	}).Decode(&reward)
	if err != nil || reward.ChannelID == 0 {
		return
	}

	embed := &discordgo.MessageEmbed{
		Title: "⚠️ Role Assignment Failed",
		Description: fmt.Sprintf(
			"<@%d> reached **%.0f %s** and qualifies for <@&%d>, but I couldn't assign the role.\n\n"+
				"**Reason:** Missing Permissions (role hierarchy)\n"+
				"**Fix:** In Server Settings → Roles, move the bot's role **above** <@&%d>.",
			userID, amount, rType, roleID, roleID,
		),
		Color:     0xff4444,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	channelIDStr := strconv.FormatInt(reward.ChannelID, 10)
	if _, err := s.ChannelMessageSendEmbed(channelIDStr, embed); err != nil {
		log.Printf("[RewardMgr] Failed to send role-failure notice in channel %s: %v", channelIDStr, err)
	}
}

func containsID(roles []string, roleID int64) bool {
	idStr := strconv.FormatInt(roleID, 10)
	for _, r := range roles {
		if r == idStr {
			return true
		}
	}
	return false
}
