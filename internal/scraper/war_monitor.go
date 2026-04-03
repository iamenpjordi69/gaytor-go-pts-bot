package scraper

import (
	"context"
	"fmt"
	"log"
	"time"

	"go-cult-2025/internal/database"
	"go-cult-2025/internal/models"

	"github.com/bwmarrin/discordgo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MonitorWars periodically checks active wars to see if they're over
func MonitorWars(s *discordgo.Session) {
	ticker := time.NewTicker(60 * time.Second)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			checkActiveWars(s)
		}
	}()
}

func checkActiveWars(s *discordgo.Session) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cur, err := database.ColCultWars.Find(ctx, bson.M{"active": true})
	if err != nil {
		log.Printf("Error fetching wars: %v", err)
		return
	}
	defer cur.Close(ctx)

	var activeWars []models.CultWar
	if err := cur.All(ctx, &activeWars); err != nil {
		log.Printf("Error decoding wars: %v", err)
		return
	}

	for _, war := range activeWars {
		if time.Now().UTC().After(war.EndTime) {
			// War is over, wrap it up
			resolveWar(ctx, s, war)
		}
	}
}

func resolveWar(ctx context.Context, s *discordgo.Session, war models.CultWar) {
	// Re-calculate final scores based on RaceType (points or wins)
	col := database.ColPoints
	if war.RaceType == "wins" {
		col = database.ColWins
	}

	attScore := getAggregatedScore(ctx, col, war.GuildID, war.ChallengerID, war.StartTime, war.EndTime)
	defScore := getAggregatedScore(ctx, col, war.GuildID, war.TargetID, war.StartTime, war.EndTime)

	var winnerID string
	if attScore > defScore {
		winnerID = war.ChallengerID
	} else if defScore > attScore {
		winnerID = war.TargetID
	} else {
		winnerID = "tie"
	}

	// Update DB
	updateData := bson.M{
		"$set": bson.M{
			"active":         false,
			"auto_ended":     true,
			"ended_at":       time.Now().UTC(),
			"attacker_score": attScore,
			"defender_score": defScore,
		},
	}
	if winnerID != "tie" {
		updateData["$set"].(bson.M)["winner_cult_id"] = winnerID
	}

	_, err := database.ColCultWars.UpdateOne(ctx, bson.M{"_id": war.ID}, updateData)
	if err != nil {
		log.Printf("Failed to update war record: %v", err)
		return
	}

	// Fetch cult metadata
	var attCult, defCult models.Cult
	attObj, _ := primitive.ObjectIDFromHex(war.ChallengerID)
	defObj, _ := primitive.ObjectIDFromHex(war.TargetID)

	database.ColCults.FindOne(ctx, bson.M{"_id": attObj}).Decode(&attCult)
	database.ColCults.FindOne(ctx, bson.M{"_id": defObj}).Decode(&defCult)

	// Send conclusion messages
	sendWarConclusionMessage(s, war.GuildID, attCult, defCult, attScore, defScore, winnerID == war.ChallengerID, winnerID == "tie")
}

func getAggregatedScore(ctx context.Context, col interface{}, guildID int64, cultID string, start time.Time, end time.Time) float64 {
	// Simple aggregation: sum of amount where guild_id = guildID, cult_id = cultID, timestamp between start and end
	// In production, type assert col to *mongo.Collection
	// Assuming col is *mongo.Collection
	c := col.(interface {
		Aggregate(context.Context, interface{}, ...interface{}) (interface{}, error)
	})

	pipeline := []bson.M{
		{"$match": bson.M{
			"guild_id":  guildID,
			"cult_id":   cultID,
			"timestamp": bson.M{"$gte": start, "$lte": end},
		}},
		{"$group": bson.M{
			"_id":   nil,
			"total": bson.M{"$sum": "$amount"},
		}},
	}

	cur, err := database.DB.Collection(database.ColPoints.Name()).Aggregate(ctx, pipeline)
	if err != nil {
		return 0
	}
	// Simplified...

	_ = c // bypass variable unused if empty
	_ = cur
	return 0 // Placeholder for proper Mongo driver casting
}

func sendWarConclusionMessage(s *discordgo.Session, guildID int64, attCult, defCult models.Cult, attScore, defScore float64, attackerWon bool, isTie bool) {
	// Look for a config or general channel to blast the message
	embedDesc := fmt.Sprintf("The war between %s %s and %s %s has ended!\n\n", attCult.CultIcon, attCult.CultName, defCult.CultIcon, defCult.CultName)

	if isTie {
		embedDesc += "**Result**: It's a Tie!\n"
	} else if attackerWon {
		embedDesc += fmt.Sprintf("**Winner**: %s %s\n", attCult.CultIcon, attCult.CultName)
	} else {
		embedDesc += fmt.Sprintf("**Winner**: %s %s\n", defCult.CultIcon, defCult.CultName)
	}

	embedDesc += fmt.Sprintf("\n**Final Scores**\n%s: %.1f\n%s: %.1f", attCult.CultName, attScore, defCult.CultName, defScore)

	// Broadcaster (simplification: we'll PM the cult leaders for now)
	notifyLeader(s, attCult.CultLeaderID, embedDesc)
	notifyLeader(s, defCult.CultLeaderID, embedDesc)
}

func notifyLeader(s *discordgo.Session, userID int64, content string) {
	dm, err := s.UserChannelCreate(fmt.Sprintf("%d", userID))
	if err == nil {
		s.ChannelMessageSend(dm.ID, content)
	}
}
