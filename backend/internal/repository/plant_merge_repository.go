package repository

import (
	"gorm.io/gorm"

	"github.com/gbplantwiki/gbplantwiki/internal/model"
)

// PlantMergeRepository handles persistence of plant merge records.
type PlantMergeRepository struct {
	db *gorm.DB
}

// NewPlantMergeRepository creates a PlantMergeRepository.
func NewPlantMergeRepository(db *gorm.DB) *PlantMergeRepository {
	return &PlantMergeRepository{db: db}
}

// Create inserts a merge record (used for failed records outside the merge transaction).
func (r *PlantMergeRepository) Create(rec *model.PlantMergeRecord) error {
	return r.db.Create(rec).Error
}

// CreateTx inserts a merge record inside a transaction.
func (r *PlantMergeRepository) CreateTx(tx *gorm.DB, rec *model.PlantMergeRecord) error {
	return tx.Create(rec).Error
}

// List returns merge records newest first with pagination.
func (r *PlantMergeRepository) List(page, pageSize int) ([]model.PlantMergeRecord, int64, error) {
	var items []model.PlantMergeRecord
	var total int64
	q := r.db.Model(&model.PlantMergeRecord{})
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
