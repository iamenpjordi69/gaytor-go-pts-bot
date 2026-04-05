package commands

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"time"

	"go-cult-2025/internal/database"
	"go-cult-2025/internal/models"

	"github.com/bwmarrin/discordgo"
	chart "github.com/wcharczuk/go-chart/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func init() {
	SlashCommands = append(SlashCommands, &discordgo.ApplicationCommand{
		Name:        "profile",
		Description: "View user's Territorial profile",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "user",
				Description: "User to check (leave blank for yourself)",
				Required:    false,
			},
		},
	})

	CommandHandlers["profile"] = profileHandler
	ComponentHandlers["profile_graph_"] = profileGraphComponentHandler
}

func fetchTotal(ctx context.Context, uID, gID int64, colType string) float64 {
	col := database.ColPoints
	if colType == "wins" {
		col = database.ColWins
	}
	pipeline := []bson.M{
		{"$match": bson.M{"user_id": uID, "guild_id": gID}},
		{"$group": bson.M{"_id": nil, "total": bson.M{"$sum": "$amount"}}},
	}
	cursor, err := col.Aggregate(ctx, pipeline)
	if err != nil {
		return 0
	}
	defer cursor.Close(ctx)
	if cursor.Next(ctx) {
		var result struct {
			Total float64 `bson:"total"`
		}
		if err := cursor.Decode(&result); err == nil {
			return result.Total
		}
	}
	return 0
}

func fetchRank(ctx context.Context, uID, gID int64, colType string) string {
	col := database.ColPoints
	if colType == "wins" {
		col = database.ColWins
	}
	pipeline := []bson.M{
		{"$match": bson.M{"guild_id": gID}},
		{"$group": bson.M{"_id": "$user_id", "total": bson.M{"$sum": "$amount"}}},
		{"$sort": bson.M{"total": -1}},
	}
	cursor, err := col.Aggregate(ctx, pipeline)
	if err != nil || cursor == nil {
		return "N/A"
	}
	defer cursor.Close(ctx)

	var results []struct {
		ID    int64   `bson:"_id"`
		Total float64 `bson:"total"`
	}
	cursor.All(ctx, &results)
	for i, r := range results {
		if r.ID == uID {
			return strconv.Itoa(i + 1)
		}
	}
	return "N/A"
}

func generateGraph(ctx context.Context, uID, gID int64, colType, title string) ([]byte, error) {
	col := database.ColPoints
	if colType == "wins" {
		col = database.ColWins
	}

	opts := options.Find().SetSort(bson.M{"timestamp": 1})
	cur, err := col.Find(ctx, bson.M{"user_id": uID, "guild_id": gID}, opts)
	if err != nil {
		return nil, fmt.Errorf("finding points: %w", err)
	}
	var txs []models.Transaction
	if err := cur.All(ctx, &txs); err != nil {
		return nil, fmt.Errorf("decoding points: %w", err)
	}
	if len(txs) == 0 {
		return nil, fmt.Errorf("no data found")
	}

	var xValues []time.Time
	var yValues []float64
	cumulative := 0.0

	for _, tx := range txs {
		cumulative += tx.Amount
		xValues = append(xValues, tx.Timestamp)
		yValues = append(yValues, cumulative)
	}

	if len(xValues) == 0 {
		return nil, fmt.Errorf("no points collected yet")
	}

	// Fix go-chart rendering crash for single data points by padding with a prior point of 0
	if len(xValues) == 1 {
		xValues = append([]time.Time{xValues[0].AddDate(0, 0, -1)}, xValues...)
		yValues = append([]float64{0}, yValues...)
	}

	graph := chart.Chart{
		Width:  512,
		Height: 269, // explicitly in the middle of 256 and 282
		Background: chart.Style{
			Padding: chart.Box{
				Top:    25, // Halved from 50
				Bottom: 8, // Halved from 20
				Left:   8, // Halved from 20
				Right:  8, // Halved from 20
			},
		},
		Title:  title,
		XAxis: chart.XAxis{
			Name:           "Date",
			ValueFormatter: chart.TimeValueFormatterWithFormat("2006-01-02"),
			GridMajorStyle: chart.Style{
				StrokeColor: chart.ColorLightGray,
				StrokeWidth: 1.0,
			},
		},
		YAxis: chart.YAxis{
			Name:  "Amount",
			GridMajorStyle: chart.Style{
				StrokeColor: chart.ColorLightGray,
				StrokeWidth: 1.0,
			},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				XValues: xValues,
				YValues: yValues,
				Style: chart.Style{
					StrokeWidth: 2.0,
					DotWidth:    3.0,
				},
			},
		},
	}

	var buffer bytes.Buffer
	if err := graph.Render(chart.PNG, &buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func profileHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	var targetUser *discordgo.User
	if len(i.ApplicationCommandData().Options) > 0 {
		targetUser = i.ApplicationCommandData().Options[0].UserValue(s)
	} else {
		targetUser = i.Member.User
		if targetUser == nil {
			targetUser = i.User
		}
	}
	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	targetID, _ := strconv.ParseInt(targetUser.ID, 10, 64)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	points := fetchTotal(ctx, targetID, guildID, "points")
	wins := fetchTotal(ctx, targetID, guildID, "wins")
	pointsRank := fetchRank(ctx, targetID, guildID, "points")
	winsRank := fetchRank(ctx, targetID, guildID, "wins")

	var userCult models.Cult
	database.ColCults.FindOne(ctx, bson.M{"guild_id": guildID, "members": targetID, "active": true}).Decode(&userCult)
	cultStr := "None"
	if userCult.CultName != "" {
		cultStr = fmt.Sprintf("%s %s", userCult.CultIcon, userCult.CultName)
	}

	opts := options.FindOne().SetSort(bson.M{"amount": 1})
	var nextReward models.RewardRole
	progressText := ""
	if err := database.ColRewardRoles.FindOne(ctx, bson.M{"guild_id": guildID, "type": "points", "amount": bson.M{"$gt": points}, "active": true}, opts).Decode(&nextReward); err == nil {
		progress := points / nextReward.Amount
		filled := int(progress * 10)
		bar := ""
		for idx := 0; idx < 10; idx++ {
			if idx < filled {
				bar += "█"
			} else {
				bar += "░"
			}
		}
		progressText = fmt.Sprintf("\n\n**Next Reward:** <@&%d>\n%s %.0f/%.0f (%.1f%%)", nextReward.RoleID, bar, points, nextReward.Amount, progress*100)
	}

	embed := &discordgo.MessageEmbed{
		Title: fmt.Sprintf("📊 %s's Profile", targetUser.Username),
		Color: 0x00aaff,
	}
	if targetUser.Avatar != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: targetUser.AvatarURL("")}
	}

	embed.Fields = []*discordgo.MessageEmbedField{
		{
			Name:   "📊 Stats",
			Value:  fmt.Sprintf("**Points:** %.0f (#%s)\n**Wins:** %.0f (#%s)%s", points, pointsRank, wins, winsRank, progressText),
			Inline: true,
		},
		{
			Name:   "⚔️ Cult",
			Value:  cultStr,
			Inline: true,
		},
	}

	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Points Graph",
					Style:    discordgo.PrimaryButton,
					CustomID: fmt.Sprintf("profile_graph_points_%d", targetID),
					Emoji:    &discordgo.ComponentEmoji{Name: "📊"},
				},
				discordgo.Button{
					Label:    "Wins Graph",
					Style:    discordgo.SecondaryButton,
					CustomID: fmt.Sprintf("profile_graph_wins_%d", targetID),
					Emoji:    &discordgo.ComponentEmoji{Name: "🏆"},
				},
			},
		},
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	})
}

func profileGraphComponentHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	// Extract graphType and targetID from `profile_graph_points_123456`
	var graphType string
	var targetID int64
	fmt.Sscanf(customID, "profile_graph_%s", &graphType) // gets "points_123456"

	// Split the remaining string securely
	var realType string
	if len(graphType) > 6 && graphType[:6] == "points" {
		realType = "points"
		fmt.Sscanf(customID, "profile_graph_points_%d", &targetID)
	} else if len(graphType) > 4 && graphType[:4] == "wins" {
		realType = "wins"
		fmt.Sscanf(customID, "profile_graph_wins_%d", &targetID)
	}

	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Fetch the actual username from Discord instead of relying on the database caching IDs
	title := "User"
	if userObj, err := s.User(strconv.FormatInt(targetID, 10)); err == nil && userObj != nil {
		title = userObj.Username
	}
	title += "'s " + realType

	graphData, err := generateGraph(ctx, targetID, guildID, realType, title)

	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ Graph error: %v", err),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Defer the message update so we can edit the webhook and clear attachments
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})

	fileName := realType + "_graph.png"
	
	// Preserve the original embed but update its image
	var embed *discordgo.MessageEmbed
	if len(i.Message.Embeds) > 0 {
		embed = i.Message.Embeds[0]
	} else {
		embed = &discordgo.MessageEmbed{}
	}
	
	embed.Image = &discordgo.MessageEmbedImage{
		URL: "attachment://" + fileName,
	}

	// Edit the webhook to replace the file and clear old attachments
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &i.Message.Components,
		Files: []*discordgo.File{
			{
				Name:        fileName,
				ContentType: "image/png",
				Reader:      bytes.NewReader(graphData),
			},
		},
		Attachments: &[]*discordgo.MessageAttachment{}, // This clears previous attachments (avoiding the 2 graphs issue)
	})
}
