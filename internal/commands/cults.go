package commands

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go-cult-2025/internal/database"

	"github.com/bwmarrin/discordgo"
	"go.mongodb.org/mongo-driver/bson"
)

func init() {
	SlashCommands = append(SlashCommands, &discordgo.ApplicationCommand{
		Name:        "cult_create",
		Description: "Create a new Territorial cult",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "name",
				Description: "Name of your cult",
				Required:    true,
			},
		},
	})
	
	CommandHandlers["cult_create"] = cultCreateHandler
}

func cultCreateHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}
	
	cultName := i.ApplicationCommandData().Options[0].StringValue()
	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	userID, _ := strconv.ParseInt(usr.ID, 10, 64)
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if already in a cult
	count, _ := database.ColCults.CountDocuments(ctx, bson.M{
		"guild_id": guildID,
		"members":  userID,
		"active":   true,
	})

	if count > 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ You are already in a cult!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Insert Cult
	_, err := database.ColCults.InsertOne(ctx, bson.M{
		"guild_id":         guildID,
		"cult_name":        cultName,
		"cult_description": "A new cult.",
		"cult_icon":        "🏆",
		"cult_leader_id":   userID,
		"active":           true,
		"members":          []int64{userID},
		"created_at":       time.Now().UTC(),
	})

	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Database error while creating cult.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🎉 Cult Created",
		Description: fmt.Sprintf("You successfully created the cult **%s**!", cultName),
		Color:       0x00ff00,
	}
	
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}
