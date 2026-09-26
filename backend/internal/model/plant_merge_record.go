package model

import "time"

// Plant merge record statuses.
const (
	MergeStatusSuccess = "success"
	MergeStatusFailed  = "failed"
)

// PlantMergeRecord audits one plant species merge (synonym consolidation).
// Detail holds a JSON snapshot of the merge course: affected users and how
// each garden item (original nickname, location, bound reminder), favorite,
// disease/pest entry and care reminder was handled.
type PlantMergeRecord struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	KeepPlantID     uint      `gorm:"index;not null" json:"keep_plant_id"`
	KeepPlantName   string    `gorm:"size:128" json:"keep_plant_name"`
	SourcePlantID   uint      `gorm:"index;not null" json:"source_plant_id"`
	SourcePlantName string    `gorm:"size:128" json:"source_plant_name"`
	OperatorID      uint      `json:"operator_id"`
	Status          string    `gorm:"size:16;index;not null" json:"status"`
	Detail          string    `gorm:"type:json" json:"detail"`
	Error           string    `gorm:"size:512" json:"error"`
	CreatedAt       time.Time `json:"created_at"`
}
