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
		Title:       "Territorial.io Cults Bot — Help",
		Description: "A comprehensive Go-powered bot for tracking Territorial.io cults, points, wins and milestone rewards.",
		Color:       0x00ff00,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "📌 General", Value: "`/help`, `/invite`", Inline: false},
			{Name: "⚙️ Admin / Config", Value: "`/bot_manager`, `/set_winlog`, `/account_linking`", Inline: false},
			{Name: "👑 Owner Commands", Value: "`/adminpoints`", Inline: false},
			{Name: "📊 Economy", Value: "`/add`, `/remove`, `/profile`, `/leaderboard`", Inline: false},
			{Name: "🎖️ Reward Roles", Value: "`/rewardrole`, `/listrewards`, `/rolelist`, `/deletereward`, `/editrewardrole`, `/set_reward_stackable`, `/force_refresh_rewards`, `/debug_rewards`, `/cleanup_roles`", Inline: false},
			{Name: "✖️ Multipliers", Value: "`/set_multiplier`, `/end_multiplier`, `/edit_multiplier`, `/multiplier_info`", Inline: false},
			{Name: "⚔️ Cults", Value: "`/cult_create`, `/cult_info`, `/cult_list`, `/join_cult`, `/promote_member`, `/cult_war`, `/end_war`, `/cult_leaderboard`", Inline: false},
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
