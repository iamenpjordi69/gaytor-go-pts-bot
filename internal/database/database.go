package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

var (
	Client *mongo.Client
	DB     *mongo.Database

	// Collections
	ColGuildEvents   *mongo.Collection
	ColPoints        *mongo.Collection
	ColWins          *mongo.Collection
	ColCults         *mongo.Collection
	ColCultWars      *mongo.Collection
	ColMultipliers   *mongo.Collection
	ColRewardRoles   *mongo.Collection
	ColWinlogSetting *mongo.Collection
	ColAccountLinks  *mongo.Collection
	ColGuildSettings *mongo.Collection
)

// Connect initializes the MongoDB setup.
func Connect() error {
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		return fmt.Errorf("MONGO_URI is not set in environment")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return fmt.Errorf("failed to connect to mongodb: %w", err)
	}

	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("failed to ping mongodb: %w", err)
	}

	Client = client
	// The Python script connected to "your_mongodb_database_name" locally (or a dynamically mapped one)
	// We will use "territorial_bot_db" unless overriden by DB_NAME var
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		// Let's inspect the python one. It said "self.db = self.mongodb_client.your_mongodb_database_name"
		dbName = "your_mongodb_database_name"
	}
	DB = client.Database(dbName)

	ColGuildEvents = DB.Collection("guild_events")
	ColPoints = DB.Collection("points")
	ColWins = DB.Collection("wins")
	ColCults = DB.Collection("cults")
	ColCultWars = DB.Collection("cult_wars")
	ColMultipliers = DB.Collection("multipliers")
	ColRewardRoles = DB.Collection("reward_roles")
	ColWinlogSetting = DB.Collection("winlog_settings")
	ColAccountLinks = DB.Collection("account_links")
	ColGuildSettings = DB.Collection("guild_settings")

	log.Println("Successfully connected to MongoDB")
	return nil
}

// Disconnect cleans up the connection
func Disconnect() {
	if Client != nil {
		if err := Client.Disconnect(context.Background()); err != nil {
			log.Printf("Error disconnecting MongoDB: %v\n", err)
		} else {
			log.Println("MongoDB disconnected")
		}
	}
}
