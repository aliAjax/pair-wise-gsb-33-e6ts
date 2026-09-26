package model

import "time"

// PlantSpecies is a plant variety with care knowledge.
type PlantSpecies struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	Family          string    `gorm:"size:64;index" json:"family"`
	Genus           string    `gorm:"size:64;index" json:"genus"`
	Name            string    `gorm:"size:128;uniqueIndex;not null" json:"name"`
	Alias           string    `gorm:"size:128" json:"alias"`
	Type            string    `gorm:"size:32;index;not null" json:"type"`
	Origin          string    `gorm:"size:128" json:"origin"`
	TempMin         float64   `json:"temp_min"`
	TempMax         float64   `json:"temp_max"`
	LightRequirement string   `gorm:"size:255" json:"light_requirement"`
	WaterFrequency  string    `gorm:"size:255" json:"water_frequency"`
	Description     string    `gorm:"type:text" json:"description"`
	ImageURLs       string    `gorm:"type:json" json:"image_urls"`
	// MergedIntoID is 0 for active entries. When set, the entry is a synonym
	// merged into the referenced species and is hidden from public listings.
	MergedIntoID    uint      `gorm:"index;not null;default:0" json:"merged_into_id"`
	CreatedAt       time.Time `json:"created_at"`
}
