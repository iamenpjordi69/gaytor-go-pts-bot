package database

import (
	"context"
	"fmt"
	"time"

	"go-cult-2025/internal/models"
	
	"go.mongodb.org/mongo-driver/bson"
)

// AddWinlogPoints directly translates the python AddWinlogPoints logic
func AddWinlogPoints(ctx context.Context, userID int64, guildID int64, points float64) (bool, error) {
	// Get multiplier
	multiplier := 1.0
	var mult models.Multiplier
	if err := ColMultipliers.FindOne(ctx, bson.M{"guild_id": guildID, "active": true}).Decode(&mult); err == nil {
		multiplier = mult.Multiplier
	}

	finalPoints := points * multiplier

	// Get user's active cult
	var userCult models.Cult
	errCult := ColCults.FindOne(ctx, bson.M{
		"guild_id": guildID,
		"members":  userID,
		"active":   true,
	}).Decode(&userCult)

	var cultIDPtr *string
	var cultNamePtr *string

	if errCult == nil {
		cid := userCult.ID.Hex()
		cultIDPtr = &cid
		cname := userCult.CultName
		cultNamePtr = &cname
	}

	now := time.Now().UTC()

	// Points transaction
	ptTx := models.Transaction{
		UserID:         userID,
		UserName:       fmt.Sprintf("%d", userID), // We don't fetch from cache explicitly here
		GuildID:        guildID,
		Amount:         finalPoints,
		BaseAmount:     points,
		MultiplierUsed: multiplier,
		CultID:         cultIDPtr,
		CultName:       cultNamePtr,
		Type:           "winlog_auto",
		Timestamp:      now,
	}

	pRes, err := ColPoints.InsertOne(ctx, ptTx)
	if err != nil {
		return false, fmt.Errorf("failed to insert points: %w", err)
	}

	// Wins transaction
	wtTx := models.Transaction{
		UserID:         userID,
		UserName:       fmt.Sprintf("%d", userID),
		GuildID:        guildID,
		Amount:         1,
		CultID:         cultIDPtr,
		CultName:       cultNamePtr,
		Type:           "winlog_auto",
		Timestamp:      now,
	}

	_, err = ColWins.InsertOne(ctx, wtTx)
	if err != nil {
		// Rollback logic
		ColPoints.DeleteOne(ctx, bson.M{"_id": pRes.InsertedID})
		return false, fmt.Errorf("failed to insert win, points rolled back: %w", err)
	}

	return true, nil
}
