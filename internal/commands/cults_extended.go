package commands

import (
	"context"
	"fmt"
	"log"
	"regexp"
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
			Name:        "join_cult",
			Description: "Join a cult",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "name",
					Description: "Name of the cult to join",
					Required:    true,
				},
			},
		},
		&discordgo.ApplicationCommand{
			Name:        "cult_info",
			Description: "Show detailed information about a cult",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "name",
					Description: "Name of the cult",
					Required:    true,
				},
			},
		},
		&discordgo.ApplicationCommand{
			Name:        "cult_list",
			Description: "List all cults in this server",
		},
		&discordgo.ApplicationCommand{
			Name:        "promote_member",
			Description: "Promote a cult member to officer (Leaders only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "user",
					Description: "User to promote",
					Required:    true,
				},
			},
		},
	)

	CommandHandlers["join_cult"] = joinCultHandler
	CommandHandlers["cult_info"] = cultInfoHandler
	CommandHandlers["cult_list"] = cultListHandler
	CommandHandlers["promote_member"] = promoteMemberHandler
}

func promoteMemberHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}
	userID, _ := strconv.ParseInt(usr.ID, 10, 64)
	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	targetUser := i.ApplicationCommandData().Options[0].UserValue(s)
	targetID, _ := strconv.ParseInt(targetUser.ID, 10, 64)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var userCult models.Cult
	err := database.ColCults.FindOne(ctx, bson.M{"guild_id": guildID, "cult_leader_id": userID, "active": true}).Decode(&userCult)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Only Cult Leaders can promote officers!", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	isMember := false
	for _, m := range userCult.Members {
		if m == targetID {
			isMember = true
			break
		}
	}
	if !isMember {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ User is not in your cult!", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	isOfficer := false
	for _, m := range userCult.Officers {
		if m == targetID {
			isOfficer = true
			break
		}
	}
	if isOfficer {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ User is already an officer!", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	database.ColCults.UpdateOne(ctx, bson.M{"_id": userCult.ID}, bson.M{"$push": bson.M{"officers": targetID}})

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("✅ **%s** has been promoted to Cult Officer!", targetUser.Username),
		},
	})
}

func joinCultHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}
	userID, _ := strconv.ParseInt(usr.ID, 10, 64)
	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)

	cultName := i.ApplicationCommandData().Options[0].StringValue()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if already in a cult
	var existing models.Cult
	err := database.ColCults.FindOne(ctx, bson.M{"guild_id": guildID, "members": userID, "active": true}).Decode(&existing)
	if err == nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ You are already in the cult **%s**!", existing.CultName),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	var target models.Cult
	// Case-insensitive lookup (requires exact matching or regex, for ease we do pure regex but Go string matching is strict)
	err = database.ColCults.FindOne(ctx, bson.M{"guild_id": guildID, "cult_name": bson.M{"$regex": "(?i)^" + regexp.QuoteMeta(cultName) + "$"}, "active": true}).Decode(&target)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Cult not found!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Wait, if it has a max members (like limit 50), enforce it. But python code just allowed anyone.
	_, err = database.ColCults.UpdateOne(ctx, bson.M{"_id": target.ID}, bson.M{"$push": bson.M{"members": userID}})

	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Failed to join cult database error.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Try to assign the cult role
	if target.RoleID != 0 {
		roleIDStr := strconv.FormatInt(target.RoleID, 10)
		if err := s.GuildMemberRoleAdd(i.GuildID, usr.ID, roleIDStr); err != nil {
			log.Printf("[JoinCult] Failed to assign cult role %s to user %s: %v", roleIDStr, usr.ID, err)
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🤝 Joined Cult",
		Description: fmt.Sprintf("You have successfully joined **%s %s**!", target.CultIcon, target.CultName),
		Color:       0x00ff00,
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func cultInfoHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	cultName := i.ApplicationCommandData().Options[0].StringValue()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var target models.Cult
	err := database.ColCults.FindOne(ctx, bson.M{"guild_id": guildID, "cult_name": bson.M{"$regex": "(?i)^" + regexp.QuoteMeta(cultName) + "$"}, "active": true}).Decode(&target)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Cult not found!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s %s", target.CultIcon, target.CultName),
		Description: fmt.Sprintf("Leader: <@%d>", target.CultLeaderID),
		Color:       target.Color,
	}
	
	if len(target.Officers) > 0 {
		offStr := ""
		for _, o := range target.Officers {
			offStr += fmt.Sprintf("<@%d> ", o)
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Officers",
			Value:  offStr,
			Inline: false,
		})
	}
	
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   "Members",
		Value:  fmt.Sprintf("%d members", len(target.Members)),
		Inline: true,
	})

	if target.ClanTag != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Linked Clan",
			Value:  target.ClanTag,
			Inline: true,
		})
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func cultListHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	guildID, _ := strconv.ParseInt(i.GuildID, 10, 64)
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cur, err := database.ColCults.Find(ctx, bson.M{"guild_id": guildID, "active": true}, options.Find().SetSort(bson.M{"cult_name": 1}))
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "❌ Database error!"},
		})
		return
	}
	defer cur.Close(ctx)

	var cults []models.Cult
	cur.All(ctx, &cults)

	if len(cults) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "No cults exist in this server!"},
		})
		return
	}

	desc := ""
	for idx, c := range cults {
		desc += fmt.Sprintf("**%d.** %s %s - `%d members`\n", idx+1, c.CultIcon, c.CultName, len(c.Members))
	}

	embed := &discordgo.MessageEmbed{
		Title: "Server Cults",
		Description: desc,
		Color: 0x00aaff,
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}
