package commands

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go-cult-2025/internal/database"

	"github.com/bwmarrin/discordgo"
	"go.mongodb.org/mongo-driver/bson"
)

func init() {
	SlashCommands = append(SlashCommands, 
		&discordgo.ApplicationCommand{
			Name:        "leaderboard",
			Description: "Show server leaderboard",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "days",
					Description: "Days to look back (0=24h, 1=48h, etc. Leave empty for all time)",
					Required:    false,
				},
			},
		},
		&discordgo.ApplicationCommand{
			Name:        "leaderboard_week",
			Description: "Show server leaderboard for last 7 days",
		},
	)

	CommandHandlers["leaderboard"] = leaderboardHandler
	CommandHandlers["leaderboard_week"] = leaderboardWeekHandler

	// Handle component clicks for leaderboard
	ComponentHandlers["leaderboard_"] = leaderboardComponentHandler
}

// Emulate a week lookup
func leaderboardWeekHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	sendLeaderboard(s, i, 7)
}

func leaderboardHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	days := -1
	if len(i.ApplicationCommandData().Options) > 0 {
		days = int(i.ApplicationCommandData().Options[0].IntValue())
	}
	sendLeaderboard(s, i, days)
}

func sendLeaderboard(s *discordgo.Session, i *discordgo.InteractionCreate, days int) {
	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	
	page := 0
	mode := "points"

	embed, components := generateLeaderboardPayload(guildID, i.GuildID, days, page, mode)

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func generateLeaderboardPayload(guildIDInt int64, guildIDStr string, days int, page int, mode string) (*discordgo.MessageEmbed, []discordgo.MessageComponent) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	col := database.ColPoints
	if mode == "wins" {
		col = database.ColWins
	}

	matchQuery := bson.M{"guild_id": guildIDInt}
	titleSuffix := "All Time"

	if days >= 0 {
		start := time.Now().UTC().Truncate(24 * time.Hour).AddDate(0, 0, -days)
		var end time.Time
		if days == 0 {
			titleSuffix = "Today (GMT)"
			end = start.Add(24 * time.Hour)
			matchQuery["timestamp"] = bson.M{"$gte": start, "$lt": end}
		} else {
			titleSuffix = fmt.Sprintf("Last %d days (GMT)", days+1)
			matchQuery["timestamp"] = bson.M{"$gte": start}
		}
	}

	pipeline := []bson.M{
		{"$match": matchQuery},
		{"$group": bson.M{
			"_id":       "$user_id",
			"total":     bson.M{"$sum": "$amount"},
		}},
		{"$sort": bson.M{"total": -1}},
		{"$skip": page * 10},
		{"$limit": 10},
	}

	cur, err := col.Aggregate(ctx, pipeline)
	var results []struct {
		ID    int64   `bson:"_id"`
		Total float64 `bson:"total"`
	}
	if err == nil {
		cur.All(ctx, &results)
	}

	title := "Points Leaderboard"
	if mode == "wins" {
		title = "Wins Leaderboard"
	}
	title = fmt.Sprintf("%s - %s", title, titleSuffix)

	desc := ""
	if len(results) == 0 {
		desc = "No data found for this period."
	} else {
		for idx, row := range results {
			desc += fmt.Sprintf("%d. <@%d> - %.0f\n", (page*10)+idx+1, row.ID, row.Total)
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: desc,
		Color:       0x00ff00,
	}

	// Prev: leaderboard_{guildID}_{days}_{page-1}_{mode}
	prevDisabled := page == 0
	
	btnModeLabel := "Wins"
	nextMode := "wins"
	if mode == "wins" {
		btnModeLabel = "Points"
		nextMode = "points"
	}

	// Payload formatting: leaderboard_GUILDID_DAYS_PAGE_MODE
	baseID := fmt.Sprintf("leaderboard_%d_%d", guildIDInt, days)

	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "◀",
					Style:    discordgo.SecondaryButton,
					CustomID: fmt.Sprintf("%s_%d_%s", baseID, page-1, mode),
					Disabled: prevDisabled,
				},
				discordgo.Button{
					Label:    btnModeLabel,
					Style:    discordgo.PrimaryButton,
					CustomID: fmt.Sprintf("%s_%d_%s", baseID, 0, nextMode),
				},
				discordgo.Button{
					Label:    "▶",
					Style:    discordgo.SecondaryButton,
					CustomID: fmt.Sprintf("%s_%d_%s", baseID, page+1, mode),
				},
			},
		},
	}

	return embed, components
}

func leaderboardComponentHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	parts := strings.Split(customID, "_")
	if len(parts) < 5 {
		return
	}

	guildIDInt, _ := strconv.ParseInt(parts[1], 10, 64)
	days, _ := strconv.Atoi(parts[2])
	page, _ := strconv.Atoi(parts[3])
	mode := parts[4]

	embed, components := generateLeaderboardPayload(guildIDInt, i.GuildID, days, page, mode)

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}
