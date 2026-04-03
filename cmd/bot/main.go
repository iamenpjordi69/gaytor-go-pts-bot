package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"go-cult-2025/internal/commands"
	"go-cult-2025/internal/database"
	"go-cult-2025/internal/scraper"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

func main() {
	// 1. Load Environment Variables
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found. Reading directly from system environment variables.")
	}

	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		log.Fatal("DISCORD_TOKEN is not defined in environment variables!")
	}

	// 2. Initialise Database Connection
	log.Println("Connecting to MongoDB...")
	if err := database.Connect(); err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer database.Disconnect()

	// 3. Initialise Discord Session
	log.Println("Starting Discord Session...")
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("Invalid Bot parameters: %v", err)
	}

	// Required intents exactly like original main.py
	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildMembers | discordgo.IntentMessageContent

	// Hook events
	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Bot logged in as %v (ID: %v)", s.State.User.String(), s.State.User.ID)
		
		// Set bot status
		s.UpdateGameStatus(0, "Territorial.io Cults | Natively ported")

		// Start background monitoring loops natively
		scraper.StartMonitoring(s)
	})

	// Hook standard interaction callbacks dynamically mapping to everything inside /internal/commands
	commands.RegisterHandlers(session)

	// 4. Open Websocket Connection
	err = session.Open()
	if err != nil {
		log.Fatalf("Cannot open the session: %v", err)
	}
	defer session.Close()

	// 5. Register Slash Commands (Sync)
	// Passing true synchronizes application commands automatically against Discord's API on launch
	commands.SyncCommands(session, true)

	// 6. Setup HTTP Health Check Server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Status: OK - TerritorialBot Go is Running"))
		})
		log.Printf("Health server started on port %s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Fatalf("Failed to start HTTP server: %v", err)
		}
	}()

	// 7. Wait for OS Signal to cleanly exit
	log.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	log.Println("Shutting down the bot gracefully...")
}
