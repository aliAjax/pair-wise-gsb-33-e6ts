package repository

import (
	"errors"

	"gorm.io/gorm"

	"github.com/gbplantwiki/gbplantwiki/internal/model"
)

// PlantSpeciesRepository handles persistence of plant species.
type PlantSpeciesRepository struct {
	db *gorm.DB
}

// NewPlantSpeciesRepository creates a PlantSpeciesRepository.
func NewPlantSpeciesRepository(db *gorm.DB) *PlantSpeciesRepository {
	return &PlantSpeciesRepository{db: db}
}

// Create inserts a plant species.
func (r *PlantSpeciesRepository) Create(p *model.PlantSpecies) error {
	if err := r.db.Create(p).Error; err != nil {
		if isDuplicate(err) {
			return ErrDuplicate
		}
		return err
	}
	return nil
}

// FindByID locates a plant species by id.
func (r *PlantSpeciesRepository) FindByID(id uint) (*model.PlantSpecies, error) {
	return r.FindByIDWith(r.db, id)
}

// FindByIDWith locates a plant species by id using the given handle (tx-aware).
func (r *PlantSpeciesRepository) FindByIDWith(q *gorm.DB, id uint) (*model.PlantSpecies, error) {
	var p model.PlantSpecies
	if err := q.First(&p, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

// Update persists changes on a plant species.
func (r *PlantSpeciesRepository) Update(p *model.PlantSpecies) error {
	return r.db.Save(p).Error
}

// UpdateTx persists changes on a plant species inside a transaction.
func (r *PlantSpeciesRepository) UpdateTx(tx *gorm.DB, p *model.PlantSpecies) error {
	return tx.Save(p).Error
}

// Delete removes a plant species by id.
func (r *PlantSpeciesRepository) Delete(id uint) error {
	res := r.db.Delete(&model.PlantSpecies{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// List filters plant species by type, family and keyword with pagination.
// Merged-away synonyms never appear in the public list.
func (r *PlantSpeciesRepository) List(speciesType, family, keyword string, page, pageSize int) ([]model.PlantSpecies, int64, error) {
	var items []model.PlantSpecies
	var total int64
	q := r.db.Model(&model.PlantSpecies{}).Where("merged_into_id = 0")
	if speciesType != "" {
		q = q.Where("type = ?", speciesType)
	}
	if family != "" {
		q = q.Where("family = ?", family)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("name LIKE ? OR alias LIKE ? OR genus LIKE ?", like, like, like)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListHot returns the most recently added plants for the home page.
func (r *PlantSpeciesRepository) ListHot(limit int) ([]model.PlantSpecies, error) {
	var items []model.PlantSpecies
	if err := r.db.Where("merged_into_id = 0").Order("id DESC").Limit(limit).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
