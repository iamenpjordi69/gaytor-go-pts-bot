package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// GuildEvent tracks bot join/leave events
type GuildEvent struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	Event       string             `bson:"event"`
	GuildID     int64              `bson:"guild_id"`
	GuildName   string             `bson:"guild_name"`
	MemberCount int                `bson:"member_count"`
	InviterID   *int64             `bson:"inviter_id,omitempty"`
	InviterName *string            `bson:"inviter_name,omitempty"`
	Timestamp   time.Time          `bson:"timestamp"`
}

// Transaction represents points or wins awarded to a user
type Transaction struct {
	ID             primitive.ObjectID `bson:"_id,omitempty"`
	UserID         int64              `bson:"user_id"`
	UserName       string             `bson:"user_name"`
	GuildID        int64              `bson:"guild_id"`
	GuildName      string             `bson:"guild_name"`
	Amount         float64            `bson:"amount"`
	BaseAmount     float64            `bson:"base_amount,omitempty"`
	MultiplierUsed float64            `bson:"multiplier_used,omitempty"`
	CultID         *string            `bson:"cult_id,omitempty"`   // Stored as string matching Python's str(id)
	CultName       *string            `bson:"cult_name,omitempty"`
	Type           string             `bson:"type"`
	Timestamp      time.Time          `bson:"timestamp"`
}

// Cult represents a group of users
type Cult struct {
	ID              primitive.ObjectID `bson:"_id,omitempty"`
	GuildID         int64              `bson:"guild_id"`
	CultName        string             `bson:"cult_name"`
	CultDescription string             `bson:"cult_description"`
	CultIcon        string             `bson:"cult_icon"`
	CultLeaderID    int64              `bson:"cult_leader_id"`
	Active          bool               `bson:"active"`
	Members         []int64            `bson:"members"`
	Officers        []int64            `bson:"officers,omitempty"`
	Color           int                `bson:"color,omitempty"`
	RoleID          int64              `bson:"role_id,omitempty"`
	ClanTag         string             `bson:"clan_tag,omitempty"`
	CreatedAt       time.Time          `bson:"created_at,omitempty"`
}

// CultWar represents a war between two cults
type CultWar struct {
	ID              primitive.ObjectID `bson:"_id,omitempty"`
	GuildID         int64              `bson:"guild_id"`
	ChallengerID    string             `bson:"challenger_id"`
	TargetID        string             `bson:"target_id"`
	ChallengerName  string             `bson:"challenger_name"`
	TargetName      string             `bson:"target_name"`
	Active          bool               `bson:"active"`
	Status          string             `bson:"status"`
	RaceType        string             `bson:"race_type"` // points, wins, both
	StartTime       time.Time          `bson:"start_time"`
	EndTime         time.Time          `bson:"end_time"`
	ChallengerStart float64            `bson:"challenger_start,omitempty"`
	TargetStart     float64            `bson:"target_start,omitempty"`
	AttackerScore   float64            `bson:"attacker_score,omitempty"`
	DefenderScore   float64            `bson:"defender_score,omitempty"`
	WinnerCultID    *string            `bson:"winner_cult_id,omitempty"`
	AutoEnded       bool               `bson:"auto_ended,omitempty"`
	EndedAt         *time.Time         `bson:"ended_at,omitempty"`
}

// Multiplier represents a server-wide points multiplier
type Multiplier struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	GuildID     int64              `bson:"guild_id"`
	Multiplier  float64            `bson:"multiplier"`
	Description string             `bson:"description,omitempty"`
	Active      bool               `bson:"active"`
	SetAt       time.Time          `bson:"set_at,omitempty"`
	EditedAt    time.Time          `bson:"edited_at,omitempty"`
}

// RewardRole represents a role granted at a certain milestone
type RewardRole struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	GuildID   int64              `bson:"guild_id"`
	RoleID    int64              `bson:"role_id"`
	ChannelID int64              `bson:"channel_id"`
	Amount    float64            `bson:"amount"`
	Type      string             `bson:"type"` // points, wins
	Active    bool               `bson:"active"`
}

// WinlogSetting represents configurations for auto-scraping territorial logs
type WinlogSetting struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	GuildID   int64              `bson:"guild_id"`
	ClanName  string             `bson:"clan_name"`
	ChannelID int64              `bson:"channel_id"`
	Active    bool               `bson:"active"`
}

// AccountLink links a Discord User ID to a Territorial.io 5-char code
type AccountLink struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	GuildID     int64              `bson:"guild_id"`
	UserID      int64              `bson:"user_id"`
	AccountName string             `bson:"account_name"`
}

// GuildSettings represents guild-wide configurations
type GuildSettings struct {
	ID              primitive.ObjectID `bson:"_id,omitempty"`
	GuildID         int64              `bson:"guild_id"`
	ManagerRoleID   string             `bson:"manager_role_id,omitempty"`
	RewardStackable bool               `bson:"reward_stackable"`
}
