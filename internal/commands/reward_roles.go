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
		Name:        "rewardrole",
		Description: "Create a new reward role milestone",
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
	})

	CommandHandlers["rewardrole"] = rewardRoleHandler
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
