package model

import "time"

// Plant merge log statuses.
const (
	PlantMergeStatusSuccess = "success"
	PlantMergeStatusFailed  = "failed"
)

// PlantMergeLog records a single merge of one duplicate plant species (source)
// into a kept plant species (target). The log survives the source plant being
// hidden so the merge history stays traceable.
type PlantMergeLog struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	PlantSpeciesID    uint      `gorm:"index:idx_merge_target;not null" json:"plant_species_id"`
	TargetName        string    `gorm:"size:128;not null" json:"target_name"`
	SourcePlantID     uint      `gorm:"index:idx_merge_source;not null" json:"source_plant_id"`
	SourceName        string    `gorm:"size:128;not null" json:"source_name"`
	SourceAlias       string    `gorm:"size:128" json:"source_alias"`
	AdminID           uint      `gorm:"index;not null" json:"admin_id"`
	Status            string    `gorm:"size:16;index;not null;default:success" json:"status"`
	FailedStep        string    `gorm:"size:64" json:"failed_step"`
	ErrorMessage      string    `gorm:"size:512" json:"error_message"`
	GardenCount       int       `json:"garden_count"`
	FavoriteCount     int       `json:"favorite_count"`
	PestCount         int       `json:"pest_count"`
	ReminderCount     int       `json:"reminder_count"`
	AffectedUserCount int       `json:"affected_user_count"`
	GardenSnapshots   string    `gorm:"type:json" json:"garden_snapshots"`
	CreatedAt         time.Time `json:"created_at"`
}

// GardenMergeSnapshot stores the original nickname/location/reminder binding of
// a garden record of the source plant before it was merged or deduplicated.
type GardenMergeSnapshot struct {
	UserID         uint   `json:"user_id"`
	Username       string `json:"username"`
	Nickname       string `json:"nickname"`
	Location       string `json:"location"`
	CareReminderID uint   `json:"care_reminder_id"`
	Action         string `json:"action"` // "relink" (moved onto target) or "dedupe" (duplicate row removed)
}
