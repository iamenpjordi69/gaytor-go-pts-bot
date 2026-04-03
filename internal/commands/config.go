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
    "go.mongodb.org/mongo-driver/mongo/options"
)

func init() {
	SlashCommands = append(SlashCommands, 
		&discordgo.ApplicationCommand{
			Name:        "delete_winlog",
			Description: "Remove the win log configuration from this server",
		},
		&discordgo.ApplicationCommand{
			Name:        "set_multiplier",
			Description: "Set a server-wide multiplier for points (Admin only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionNumber,
					Name:        "multiplier",
					Description: "Number like 1.5, 2.0, 0.5",
					Required:    true,
				},
			},
		},
	)

	CommandHandlers["delete_winlog"] = deleteWinlogHandler
	CommandHandlers["set_multiplier"] = setMultiplierHandler
}

func deleteWinlogHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	canManage := (i.Member.Permissions & discordgo.PermissionAdministrator) == discordgo.PermissionAdministrator
	if !canManage {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Administrator permissions required!", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := database.ColWinlogSetting.DeleteMany(ctx, bson.M{"guild_id": guildID})
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Delete failed!", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "🗑️ Winlog configuraton for this server has been deleted.",
		},
	})
}

func setMultiplierHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	canManage := (i.Member.Permissions & discordgo.PermissionAdministrator) == discordgo.PermissionAdministrator
	if !canManage {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Administrator permissions required!", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	val := i.ApplicationCommandData().Options[0].FloatValue()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := options.Update().SetUpsert(true)
	
	newConf := models.Multiplier{
		GuildID:    guildID,
		Multiplier: val,
		Active:     true,
	}

	_, err := database.ColMultipliers.UpdateOne(ctx, 
		bson.M{"guild_id": guildID}, 
		bson.M{"$set": newConf}, 
		opts,
	)

	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Configuration saving failed!", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("✅ Base multiplier updated to **%.2fx** across this server.", val),
		},
	})
}
