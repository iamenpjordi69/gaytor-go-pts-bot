package commands

import (
	"log"
	"os"

	"github.com/bwmarrin/discordgo"
)

var (
	// All application commands available
	SlashCommands = []*discordgo.ApplicationCommand{}
	// Map of command name to handler function
	CommandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){}
	// Map of component customID to handler function
	ComponentHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){}
)

// RegisterHandlers maps the discord interaction events to our handler maps
func RegisterHandlers(s *discordgo.Session) {
	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			if h, ok := CommandHandlers[i.ApplicationCommandData().Name]; ok {
				h(s, i)
			}
		case discordgo.InteractionMessageComponent:
			// For components, we might use a prefix, like "claim_winlog:user_id"
			// So we need to match prefixes or exact IDs.
			customID := i.MessageComponentData().CustomID
			
			// Exact match
			if h, ok := ComponentHandlers[customID]; ok {
				h(s, i)
				return
			}
			
			// Prefix matching (Iterate and find prefix)
			for key, h := range ComponentHandlers {
				if len(customID) >= len(key) && customID[:len(key)] == key {
					h(s, i)
					return
				}
			}
		case discordgo.InteractionApplicationCommandAutocomplete:
			if h, ok := CommandHandlers[i.ApplicationCommandData().Name+"_autocomplete"]; ok {
				h(s, i)
			}
		}
	})
}

// SyncCommands registers all defined slash commands with Discord
func SyncCommands(s *discordgo.Session, force bool) {
	// Let's only sync if a specific dev command is called, or if force is true
	if !force {
		log.Println("Command sync skipped. Use the force flag or /sync command to update Discord API.")
		return
	}

	log.Println("Syncing commands to Discord API...")
	
	// We are syncing globally
	for _, v := range SlashCommands {
		_, err := s.ApplicationCommandCreate(s.State.User.ID, "", v)
		if err != nil {
			log.Printf("Cannot create '%v' command: %v", v.Name, err)
		} else {
			log.Printf("Created command '%v'", v.Name)
		}
	}
	
	log.Println("Command sync complete.")
}

// IsBotOwner checks if the interaction user is the owner defined in .env
func IsBotOwner(userID string) bool {
	return userID == os.Getenv("BOT_OWNER")
}
