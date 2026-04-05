package commands

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go-cult-2025/internal/database"

	"github.com/bwmarrin/discordgo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func init() {
	// Register the command definitions
	SlashCommands = append(SlashCommands, &discordgo.ApplicationCommand{
		Name:        "bot_manager",
		Description: "Configure bot manager role (Admin only)",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionRole,
				Name:        "role",
				Description: "Role to set as bot manager",
				Required:    true,
			},
		},
	})

	// Register the handler
	CommandHandlers["bot_manager"] = botManagerHandler
}

func botManagerHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}

	// Permission check: Admin or Bot Owner
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

	roleID := i.ApplicationCommandData().Options[0].RoleValue(s, i.GuildID).ID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	guildIDInt, _ := strconv.ParseInt(i.GuildID, 10, 64)

	// Use update with upsert to maintain other settings like RewardStackable
	opts := options.Update().SetUpsert(true)
	_, err := database.ColGuildSettings.UpdateOne(ctx,
		bson.M{"guild_id": guildIDInt},
		bson.M{"$set": bson.M{"manager_role_id": roleID}},
		opts,
	)

	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Failed to save bot settings!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "✅ Bot Manager Role Set",
		Description: fmt.Sprintf("Bot manager role set to <@&%s>", roleID),
		Color:       0x00ff00,
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}
