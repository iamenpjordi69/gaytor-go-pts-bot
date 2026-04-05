package commands

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-cult-2025/internal/database"

	"github.com/bwmarrin/discordgo"
)

var (
	winlogClaims   = make(map[string]map[string]float64) // msgID -> userID -> multiplier
	winlogClaimsMu sync.RWMutex
)

func init() {
	// Prefix match route
	ComponentHandlers["claim_winlog_"] = claimWinlogHandler
}

func claimWinlogHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	usr := i.Member.User
	if usr == nil {
		usr = i.User
	}
	
	customID := i.MessageComponentData().CustomID
	parts := strings.Split(customID, "_")
	if len(parts) < 5 {
		// Expect claim_winlog_{guildID}_{points}_{multiplier}
		return
	}

	guildID, _ := strconv.ParseInt(parts[2], 10, 64)
	points, _ := strconv.ParseFloat(parts[3], 64)
	multiplier, _ := strconv.ParseFloat(parts[4], 64)

	msgID := i.Message.ID
	userID := usr.ID
	
	winlogClaimsMu.Lock()
	if winlogClaims[msgID] == nil {
		winlogClaims[msgID] = make(map[string]float64)
		// Auto-cleanup the map entry after 5 minutes
		go func(mID string) {
			time.Sleep(5 * time.Minute)
			winlogClaimsMu.Lock()
			delete(winlogClaims, mID)
			winlogClaimsMu.Unlock()
		}(msgID)
	}

	// Check if user already claimed
	if _, exists := winlogClaims[msgID][userID]; exists {
		winlogClaimsMu.Unlock()
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ You already claimed points from this log!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Record the claim provisionally
	winlogClaims[msgID][userID] = multiplier
	winlogClaimsMu.Unlock()

	// Calculate and assign points
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userIDInt, _ := strconv.ParseInt(userID, 10, 64)
	
	success, err := database.AddWinlogPoints(ctx, userIDInt, guildID, points*multiplier)
	if !success || err != nil {
		log.Printf("Error adding winlog points: %v", err)
		// Rollback claim
		winlogClaimsMu.Lock()
		delete(winlogClaims[msgID], userID)
		winlogClaimsMu.Unlock()

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Failed to add points!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Create embed
	embed := &discordgo.MessageEmbed{
		Title:       "✅ Points Claimed!",
		Description: fmt.Sprintf("You received **%.1f points** and **1 win**!", points*multiplier),
		Color:       0x00ff00,
	}
	if multiplier > 1.0 {
		embed.Description += fmt.Sprintf("\n*Base: %.1f x %.1f = %.1f points*", points, multiplier, points*multiplier)
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})

	// Trigger Reward Manager in background (own fresh context)
	go CheckAndAssignRewards(s, guildID, userIDInt, "points")
	go CheckAndAssignRewards(s, guildID, userIDInt, "wins")

	go updateOriginalMessageEmbed(s, i.Message, msgID)
}

func updateOriginalMessageEmbed(s *discordgo.Session, msg *discordgo.Message, msgID string) {
	winlogClaimsMu.RLock()
	claimsMap := winlogClaims[msgID]
	
	claimedStr := ""
	count := 0
	for uID, mult := range claimsMap {
		claimedStr += fmt.Sprintf("<@%s> (%.1fx)\n", uID, mult)
		count++
		if count >= 10 { // Limit visually to 10 like python code
			break
		}
	}
	winlogClaimsMu.RUnlock()

	if len(msg.Embeds) > 0 && claimedStr != "" {
		emb := msg.Embeds[0]
		found := false
		for _, f := range emb.Fields {
			if f.Name == "Claimed by" {
				f.Value = claimedStr
				found = true
				break
			}
		}
		if !found {
			emb.Fields = append(emb.Fields, &discordgo.MessageEmbedField{
				Name: "Claimed by",
				Value: claimedStr,
				Inline: false,
			})
		}
		s.ChannelMessageEditEmbeds(msg.ChannelID, msg.ID, []*discordgo.MessageEmbed{emb})
	}
}
