package dto

import "github.com/gbplantwiki/gbplantwiki/internal/model"

// PlantMergeRequest selects the kept plant and the duplicates to merge in.
type PlantMergeRequest struct {
	TargetPlantID  uint   `json:"target_plant_id" binding:"required"`
	SourcePlantIDs []uint `json:"source_plant_ids" binding:"required,min=1,dive,required"`
}

// MergePreviewGardenRow is one garden record touched by a pending merge.
type MergePreviewGardenRow struct {
	SourceGardenID   uint   `json:"source_garden_id"`
	TargetGardenID   uint   `json:"target_garden_id"`
	UserID           uint   `json:"user_id"`
	Username         string `json:"username"`
	UserNickname     string `json:"user_nickname"`
	SourceNickname   string `json:"source_nickname"`
	SourceLocation   string `json:"source_location"`
	SourceReminderID uint   `json:"source_reminder_id"`
	Collides         bool   `json:"collides"`
	Action           string `json:"action"` // "dedupe": user keeps both, source row removed; "relink": row moved to target
}

// MergePreviewUser is one user affected by a pending merge.
type MergePreviewUser struct {
	UserID       uint   `json:"user_id"`
	Username     string `json:"username"`
	Nickname     string `json:"nickname"`
	GardenRows   int    `json:"garden_rows"`
	FavoriteRows int    `json:"favorite_rows"`
	ReminderRows int    `json:"reminder_rows"`
	Collides     bool   `json:"collides"`
}

// MergePreview is returned before executing a merge so the admin can review
// every affected user and association.
type MergePreview struct {
	TargetPlant   model.PlantSpecies      `json:"target_plant"`
	SourcePlants  []model.PlantSpecies    `json:"source_plants"`
	GardenRows    []MergePreviewGardenRow `json:"garden_rows"`
	Favorites     []model.Favorite        `json:"favorites"`
	Pests         []model.DiseasePest     `json:"pests"`
	Reminders     []model.CareReminder    `json:"reminders"`
	AffectedUsers []MergePreviewUser      `json:"affected_users"`
	Summary       MergePreviewSummary     `json:"summary"`
}

// MergePreviewSummary counts the associations a merge will touch.
type MergePreviewSummary struct {
	GardenRows       int `json:"garden_rows"`
	GardenCollisions int `json:"garden_collisions"`
	FavoriteRows     int `json:"favorite_rows"`
	PestRows         int `json:"pest_rows"`
	ReminderRows     int `json:"reminder_rows"`
	AffectedUsers    int `json:"affected_users"`
}
