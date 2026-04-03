package commands

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go-cult-2025/internal/database"
	"go-cult-2025/internal/models"

	"github.com/bwmarrin/discordgo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func init() {
	SlashCommands = append(SlashCommands, 
		&discordgo.ApplicationCommand{
			Name:        "cult_war",
			Description: "Declare war on another cult (Leaders/Officers only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "target_cult",
					Description: "Name of the cult to declare war on",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "duration",
					Description: "War duration in days (default 7)",
					Required:    false,
				},
			},
		},
		&discordgo.ApplicationCommand{
			Name:        "end_war",
			Description: "End your cult's ongoing war (Leaders/Officers only)",
		},
		&discordgo.ApplicationCommand{
			Name:        "cult_leaderboard",
			Description: "Show cult leaderboard",
		},
	)

	CommandHandlers["cult_war"] = cultWarHandler
	CommandHandlers["end_war"] = endWarHandler
	CommandHandlers["cult_leaderboard"] = cultLeaderboardHandler
}

// ... handlers skipped to preserve code... //

func cultWarHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}
	userID, _ := strconv.ParseInt(usr.ID, 10, 64)
	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)

	targetCultName := i.ApplicationCommandData().Options[0].StringValue()
	durationDays := 7
	if len(i.ApplicationCommandData().Options) > 1 {
		durationDays = int(i.ApplicationCommandData().Options[1].IntValue())
	}
	if durationDays < 1 {
		durationDays = 7
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find the user's cult
	var userCult models.Cult
	err := database.ColCults.FindOne(ctx, bson.M{"guild_id": guildID, "members": userID, "active": true}).Decode(&userCult)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: stringPtr("❌ You are not in a cult!")})
		return
	}

	isLeader := userCult.CultLeaderID == userID
	isOfficer := false
	for _, o := range userCult.Officers {
		if o == userID {
			isOfficer = true
		}
	}

	if !isLeader && !isOfficer {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: stringPtr("❌ Only Cult Leaders or Officers can declare war!")})
		return
	}

	// Format names for case-insensitive comparison
	if stringsToLowerClean(userCult.CultName) == stringsToLowerClean(targetCultName) {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: stringPtr("❌ You cannot declare war on your own cult!")})
		return
	}

	// Find target cult
	var targetCult models.Cult
	err = database.ColCults.FindOne(ctx, bson.M{"guild_id": guildID, "cult_name": bson.M{"$regex": "(i?)^" + targetCultName + "$"}, "active": true}).Decode(&targetCult)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: stringPtr("❌ Cult not found!")})
		return
	}

	// Check for active wars
	var activeWar models.CultWar
	warQuery := bson.M{
		"guild_id": guildID,
		"$or": []bson.M{
			{"challenger_id": userCult.ID.Hex()},
			{"target_id": userCult.ID.Hex()},
			{"challenger_id": targetCult.ID.Hex()},
			{"target_id": targetCult.ID.Hex()},
		},
		"status": "active",
	}
	err = database.ColCultWars.FindOne(ctx, warQuery).Decode(&activeWar)
	if err == nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: stringPtr("❌ One of the cults is already in an active war!")})
		return
	}

	// Declare war
	newWar := models.CultWar{
		GuildID:         guildID,
		ChallengerID:    userCult.ID.Hex(),
		TargetID:        targetCult.ID.Hex(),
		ChallengerName:  userCult.CultName,
		TargetName:      targetCult.CultName,
		StartTime:       time.Now().UTC(),
		EndTime:         time.Now().UTC().AddDate(0, 0, durationDays),
		Status:          "active",
		ChallengerStart: 0, // In full Python version, this snaps the current total points
		TargetStart:     0,
	}

	// Base points logic (for differential war tracking)
	aggCur, _ := database.ColPoints.Aggregate(ctx, []bson.M{
		{"$match": bson.M{"guild_id": guildID, "cult_id": userCult.ID.Hex()}},
		{"$group": bson.M{"_id": nil, "total": bson.M{"$sum": "$amount"}}},
	})
	if aggCur != nil {
		var res struct{ Total float64 `bson:"total"` }
		if aggCur.Next(ctx) {
			aggCur.Decode(&res)
			newWar.ChallengerStart = res.Total
		}
	}

	aggCurT, _ := database.ColPoints.Aggregate(ctx, []bson.M{
		{"$match": bson.M{"guild_id": guildID, "cult_id": targetCult.ID.Hex()}},
		{"$group": bson.M{"_id": nil, "total": bson.M{"$sum": "$amount"}}},
	})
	if aggCurT != nil {
		var res struct{ Total float64 `bson:"total"` }
		if aggCurT.Next(ctx) {
			aggCurT.Decode(&res)
			newWar.TargetStart = res.Total
		}
	}

	database.ColCultWars.InsertOne(ctx, newWar)

	embed := &discordgo.MessageEmbed{
		Title:       "⚔️ War Declared! ⚔️",
		Description: fmt.Sprintf("**%s** has declared war on **%s**!", userCult.CultName, targetCult.CultName),
		Color:       0xff0000,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Duration", Value: fmt.Sprintf("%d days", durationDays), Inline: true},
			{Name: "Ends", Value: fmt.Sprintf("<t:%d:R>", newWar.EndTime.Unix()), Inline: true},
		},
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func endWarHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}
	userID, _ := strconv.ParseInt(usr.ID, 10, 64)
	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var userCult models.Cult
	err := database.ColCults.FindOne(ctx, bson.M{"guild_id": guildID, "members": userID, "active": true}).Decode(&userCult)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ You are not in a cult!", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	isLeader := userCult.CultLeaderID == userID
	isOfficer := false
	for _, o := range userCult.Officers {
		if o == userID {
			isOfficer = true
		}
	}
	canManage := (i.Member.Permissions & discordgo.PermissionAdministrator) == discordgo.PermissionAdministrator

	if !isLeader && !isOfficer && !canManage {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Only Leaders, Officers, or Admins can surrender/end a war prematurely!", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	var activeWar models.CultWar
	err = database.ColCultWars.FindOne(ctx, bson.M{
		"guild_id": guildID,
		"$or": []bson.M{
			{"challenger_id": userCult.ID.Hex()},
			{"target_id": userCult.ID.Hex()},
		},
		"status": "active",
	}).Decode(&activeWar)

	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Your cult is not currently in a war!", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	// Update to ended
	database.ColCultWars.UpdateOne(ctx, bson.M{"_id": activeWar.ID}, bson.M{"$set": bson.M{"status": "ended", "end_time": time.Now().UTC()}})

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🏳️ The war between **%s** and **%s** has been concluded prematurely!", activeWar.ChallengerName, activeWar.TargetName),
		},
	})
}

func cultLeaderboardHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Aggregate points per cult ID 
	pipeline := []bson.M{
		{"$match": bson.M{"guild_id": guildID}},
		{"$group": bson.M{"_id": "$cult_id", "total": bson.M{"$sum": "$amount"}}},
		{"$sort": bson.M{"total": -1}},
		{"$limit": 10},
	}

	cur, err := database.ColPoints.Aggregate(ctx, pipeline)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Error reading scores!"},
		})
		return
	}
	defer cur.Close(ctx)

	var results []struct {
		CultID string  `bson:"_id"`
		Total  float64 `bson:"total"`
	}
	cur.All(ctx, &results)

	desc := ""
	for idx, res := range results {
		if res.CultID == "" || res.CultID == "<nil>" {
			continue // No-cult users
		}
		var c models.Cult
		// cult_id is stored as hex string
		oid, _ := primitive.ObjectIDFromHex(res.CultID)
		database.ColCults.FindOne(ctx, bson.M{"_id": oid}).Decode(&c)
		desc += fmt.Sprintf("**%d.** %s %s - **%.0f points**\n", idx+1, c.CultIcon, c.CultName, res.Total)
	}

	if desc == "" {
		desc = "No cult points tracked yet!"
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🏆 Cult Leaderboard",
		Description: desc,
		Color:       0xffaa00,
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

// Ensure string manipulation works safely without nil crashing
func stringsToLowerClean(str string) string {
	return strings.ToLower(strings.TrimSpace(str))
}
