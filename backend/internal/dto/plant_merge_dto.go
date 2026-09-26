package dto

// Merge actions applied to garden items and favorites during a plant merge.
const (
	MergeActionMove            = "move"             // transferred to the kept plant
	MergeActionRemoveDuplicate = "remove_duplicate" // dropped because the user already has the kept plant
)

// PlantMergeRequest selects the kept plant and the plant merged into it.
type PlantMergeRequest struct {
	KeepPlantID   uint `json:"keep_plant_id" binding:"required"`
	SourcePlantID uint `json:"source_plant_id" binding:"required"`
}

// MergeAffectedUser summarizes one user touched by a merge.
type MergeAffectedUser struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
}

// MergeGardenImpact describes how one garden item of the source plant is handled.
type MergeGardenImpact struct {
	GardenID       uint   `json:"garden_id"`
	UserID         uint   `json:"user_id"`
	Username       string `json:"username"`
	Nickname       string `json:"nickname"`
	Location       string `json:"location"`
	CareReminderID uint   `json:"care_reminder_id"`
	Action         string `json:"action"`
}

// MergeFavoriteImpact describes how one plant favorite of the source plant is handled.
type MergeFavoriteImpact struct {
	FavoriteID uint   `json:"favorite_id"`
	UserID     uint   `json:"user_id"`
	Username   string `json:"username"`
	Action     string `json:"action"`
}

// MergeReminderImpact describes one care reminder moved to the kept plant.
type MergeReminderImpact struct {
	ReminderID uint   `json:"reminder_id"`
	UserID     uint   `json:"user_id"`
	Username   string `json:"username"`
	TaskTitle  string `json:"task_title"`
	RemindDate string `json:"remind_date"`
}

// MergePestImpact describes one disease/pest entry moved to the kept plant.
type MergePestImpact struct {
	PestID uint   `json:"pest_id"`
	Name   string `json:"name"`
}

// PlantMergePreview is the dry-run impact report of a plant merge. The same
// structure is stored as the merge record detail after execution.
type PlantMergePreview struct {
	KeepPlantID     uint                  `json:"keep_plant_id"`
	KeepPlantName   string                `json:"keep_plant_name"`
	SourcePlantID   uint                  `json:"source_plant_id"`
	SourcePlantName string                `json:"source_plant_name"`
	AffectedUsers   []MergeAffectedUser   `json:"affected_users"`
	Gardens         []MergeGardenImpact   `json:"gardens"`
	Favorites       []MergeFavoriteImpact `json:"favorites"`
	Reminders       []MergeReminderImpact `json:"reminders"`
	Pests           []MergePestImpact     `json:"pests"`
}
