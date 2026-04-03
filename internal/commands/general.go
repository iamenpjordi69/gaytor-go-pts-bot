package commands

import (
	"github.com/bwmarrin/discordgo"
)

func init() {
	SlashCommands = append(SlashCommands, &discordgo.ApplicationCommand{
		Name:        "help",
		Description: "Shows the bot's help menu",
	})
	
	SlashCommands = append(SlashCommands, &discordgo.ApplicationCommand{
		Name:        "invite",
		Description: "Get the bot's invite link",
	})

	CommandHandlers["help"] = helpHandler
	CommandHandlers["invite"] = inviteHandler
}

func helpHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "Territorial.io Cults Bot - Help",
		Description: "A comprehensive rewrite in Go for matching Territorial.io win logs, tracking cults, and offering a points/wins economy.",
		Color:       0x00ff00,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "General Commands", Value: "`/help`, `/invite`", Inline: false},
			{Name: "Admin Commands", Value: "`/bot_manager`", Inline: false},
			{Name: "Owner Commands", Value: "`/set_winlog`, `/adminpoints`, `/adminwins`, `/account_linking`", Inline: false},
			{Name: "Reward Roles", Value: "`/rewardrole`, `/listrewards`, `/deletereward`, `/editrewardrole`, `/force_refresh_rewards`", Inline: false},
			{Name: "Cults & Economy", Value: "`/cult_create`, `/cult_info`, `/cult_stats`, `/profile`, `/leaderboard`, among others", Inline: false},
		},
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func inviteHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	inviteLink := "https://discord.com/api/oauth2/authorize?client_id=" + s.State.User.ID + "&permissions=8&scope=bot%20applications.commands"
	
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Invite the bot to your server using this link:\n" + inviteLink,
		},
	})
}
