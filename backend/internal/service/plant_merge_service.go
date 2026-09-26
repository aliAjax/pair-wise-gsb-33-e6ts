package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"gorm.io/gorm"

	"github.com/gbplantwiki/gbplantwiki/internal/constants"
	"github.com/gbplantwiki/gbplantwiki/internal/dto"
	"github.com/gbplantwiki/gbplantwiki/internal/model"
	"github.com/gbplantwiki/gbplantwiki/internal/repository"
	"github.com/gbplantwiki/gbplantwiki/internal/util"
)

// mergePlan is the computed merge course used both for preview display and
// for executing the write transaction.
type mergePlan struct {
	keep           *model.PlantSpecies
	source         *model.PlantSpecies
	sourceGardens  []model.UserGarden
	sourceFavorite []model.Favorite
	gardenAction   map[uint]string
	favoriteAction map[uint]string
	preview        dto.PlantMergePreview
}

// PlantMergeService consolidates synonym plant species: it moves garden
// items, favorites, disease/pest entries and care reminders onto the kept
// species, hides the source species and records the whole merge course.
type PlantMergeService struct {
	db           *gorm.DB
	plantRepo    *repository.PlantSpeciesRepository
	gardenRepo   *repository.UserGardenRepository
	favoriteRepo *repository.FavoriteRepository
	reminderRepo *repository.CareReminderRepository
	pestRepo     *repository.DiseasePestRepository
	userRepo     *repository.UserRepository
	mergeRepo    *repository.PlantMergeRepository
	logger       *slog.Logger
}

// NewPlantMergeService creates a PlantMergeService.
func NewPlantMergeService(
	db *gorm.DB,
	plantRepo *repository.PlantSpeciesRepository,
	gardenRepo *repository.UserGardenRepository,
	favoriteRepo *repository.FavoriteRepository,
	reminderRepo *repository.CareReminderRepository,
	pestRepo *repository.DiseasePestRepository,
	userRepo *repository.UserRepository,
	mergeRepo *repository.PlantMergeRepository,
	logger *slog.Logger,
) *PlantMergeService {
	return &PlantMergeService{
		db: db, plantRepo: plantRepo, gardenRepo: gardenRepo, favoriteRepo: favoriteRepo,
		reminderRepo: reminderRepo, pestRepo: pestRepo, userRepo: userRepo,
		mergeRepo: mergeRepo, logger: logger,
	}
}

// Preview runs the merge dry-run and reports affected users and associations.
func (s *PlantMergeService) Preview(keepID, sourceID uint) (*dto.PlantMergePreview, error) {
	plan, err := s.loadMergePlan(s.db, keepID, sourceID)
	if err != nil {
		return nil, err
	}
	s.logger.Info(fmt.Sprintf(constants.LogPlantMergePreview, keepID, sourceID))
	return &plan.preview, nil
}

// Execute performs the merge in a single transaction. Any mid-processing
// failure rolls gardens, favorites, disease/pest entries, reminders and
// plant rows back to their pre-merge state, while a failed merge record is
// written afterwards so the console can flag the stuck entry.
func (s *PlantMergeService) Execute(operatorID, keepID, sourceID uint) (*model.PlantMergeRecord, error) {
	// Pre-check on the outer connection: validate the request and snapshot the
	// expected merge course before opening the write transaction.
	prePlan, err := s.loadMergePlan(s.db, keepID, sourceID)
	if err != nil {
		return nil, err
	}
	detailJSON, err := json.Marshal(prePlan.preview)
	if err != nil {
		return nil, fmt.Errorf("plant merge detail marshal: %w", err)
	}

	var created *model.PlantMergeRecord
	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		// Reload inside the transaction so the writes act on a consistent,
		// fresh snapshot (also rejects a source merged away concurrently).
		plan, err := s.loadMergePlan(tx, keepID, sourceID)
		if err != nil {
			return err
		}
		for i := range plan.sourceGardens {
			g := &plan.sourceGardens[i]
			if plan.gardenAction[g.ID] == dto.MergeActionRemoveDuplicate {
				// The user already gardens the kept plant: only one garden row
				// may remain, so the source-side row (snapshot recorded in the
				// merge record) is dropped.
				if err := s.gardenRepo.DeleteTx(tx, g.ID); err != nil {
					return fmt.Errorf("plant merge garden dedupe: %w", err)
				}
			} else {
				g.PlantSpeciesID = keepID
				if err := s.gardenRepo.UpdateTx(tx, g); err != nil {
					return fmt.Errorf("plant merge garden move: %w", err)
				}
			}
		}
		for i := range plan.sourceFavorite {
			f := &plan.sourceFavorite[i]
			if plan.favoriteAction[f.ID] == dto.MergeActionRemoveDuplicate {
				if err := s.favoriteRepo.DeleteTx(tx, f.ID); err != nil {
					return fmt.Errorf("plant merge favorite dedupe: %w", err)
				}
			} else {
				f.TargetID = keepID
				if err := s.favoriteRepo.UpdateTx(tx, f); err != nil {
					return fmt.Errorf("plant merge favorite move: %w", err)
				}
			}
		}
		if _, err := s.reminderRepo.ReassignPlantTx(tx, sourceID, keepID); err != nil {
			return fmt.Errorf("plant merge reminder move: %w", err)
		}
		if _, err := s.pestRepo.ReassignPlantTx(tx, sourceID, keepID); err != nil {
			return fmt.Errorf("plant merge pest move: %w", err)
		}
		plan.source.MergedIntoID = keepID
		if err := s.plantRepo.UpdateTx(tx, plan.source); err != nil {
			return fmt.Errorf("plant merge hide source: %w", err)
		}
		rec := &model.PlantMergeRecord{
			KeepPlantID:     keepID,
			KeepPlantName:   plan.keep.Name,
			SourcePlantID:   sourceID,
			SourcePlantName: plan.source.Name,
			OperatorID:      operatorID,
			Status:          model.MergeStatusSuccess,
			Detail:          string(detailJSON),
		}
		if err := s.mergeRepo.CreateTx(tx, rec); err != nil {
			return fmt.Errorf("plant merge record create: %w", err)
		}
		created = rec
		return nil
	})
	if txErr != nil {
		// A business validation error raised before any write (e.g. the source
		// was merged away by another admin) is reported as-is and is not a
		// "stuck" merge. Infrastructure failures mid-processing have rolled the
		// transaction back already; record them as failed for the console.
		var appErr *util.AppError
		if errors.As(txErr, &appErr) {
			return nil, txErr
		}
		failedRec := &model.PlantMergeRecord{
			KeepPlantID:     keepID,
			KeepPlantName:   prePlan.keep.Name,
			SourcePlantID:   sourceID,
			SourcePlantName: prePlan.source.Name,
			OperatorID:      operatorID,
			Status:          model.MergeStatusFailed,
			Detail:          string(detailJSON),
			Error:           truncate(txErr.Error(), 500),
		}
		if recErr := s.mergeRepo.Create(failedRec); recErr != nil {
			s.logger.Error("failed merge record create failed", "error", recErr,
				"keep_plant_id", keepID, "source_plant_id", sourceID)
		}
		s.logger.Error(fmt.Sprintf(constants.LogPlantMergeFailed, keepID, sourceID), "error", txErr)
		return nil, util.NewAppError(500, constants.CodeInternalError,
			fmt.Sprintf("PlantMerge[keep_id=%d source_id=%d] failed mid-processing, all records rolled back: %s",
				keepID, sourceID, failedRec.Error))
	}
	s.logger.Info(fmt.Sprintf(constants.LogPlantMergeSuccess, keepID, sourceID),
		"record_id", created.ID, "operator_id", operatorID)
	return created, nil
}

// ListRecords returns merge records (both successful and failed/stuck).
func (s *PlantMergeService) ListRecords(page, pageSize int) ([]model.PlantMergeRecord, int64, error) {
	items, total, err := s.mergeRepo.List(page, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("plant merge record list: %w", err)
	}
	return items, total, nil
}

// loadMergePlan validates the request and computes the full merge course.
// It is tx-aware: Preview passes the pooled connection, Execute passes its
// open transaction so reads and writes share one snapshot.
func (s *PlantMergeService) loadMergePlan(q *gorm.DB, keepID, sourceID uint) (*mergePlan, error) {
	if keepID == 0 || sourceID == 0 {
		return nil, util.NewAppError(422, constants.CodeValidationError, "plant merge failed: keep_plant_id and source_plant_id are required")
	}
	if keepID == sourceID {
		return nil, util.NewAppError(422, constants.CodeValidationError,
			fmt.Sprintf("PlantMerge[keep_id=%d source_id=%d] failed: keep and source must differ", keepID, sourceID))
	}
	keep, err := s.plantRepo.FindByIDWith(q, keepID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(404, constants.CodeNotFound, fmt.Sprintf("PlantSpecies[id=%d] not found", keepID))
		}
		return nil, fmt.Errorf("plant merge keep find: %w", err)
	}
	source, err := s.plantRepo.FindByIDWith(q, sourceID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(404, constants.CodeNotFound, fmt.Sprintf("PlantSpecies[id=%d] not found", sourceID))
		}
		return nil, fmt.Errorf("plant merge source find: %w", err)
	}
	if keep.MergedIntoID != 0 {
		return nil, util.NewAppError(409, constants.CodeConflict,
			fmt.Sprintf("PlantSpecies[id=%d] merge failed: kept plant is itself merged into %d", keepID, keep.MergedIntoID))
	}
	if source.MergedIntoID != 0 {
		return nil, util.NewAppError(409, constants.CodeConflict,
			fmt.Sprintf("PlantSpecies[id=%d] merge failed: already merged into %d", sourceID, source.MergedIntoID))
	}

	sourceGardens, err := s.gardenRepo.ListByPlant(q, sourceID)
	if err != nil {
		return nil, fmt.Errorf("plant merge garden list: %w", err)
	}
	keepGardens, err := s.gardenRepo.ListByPlant(q, keepID)
	if err != nil {
		return nil, fmt.Errorf("plant merge keep garden list: %w", err)
	}
	sourceFavorites, err := s.favoriteRepo.ListByTarget(q, constants.FavoriteTargetPlant, sourceID)
	if err != nil {
		return nil, fmt.Errorf("plant merge favorite list: %w", err)
	}
	keepFavorites, err := s.favoriteRepo.ListByTarget(q, constants.FavoriteTargetPlant, keepID)
	if err != nil {
		return nil, fmt.Errorf("plant merge keep favorite list: %w", err)
	}
	reminders, err := s.reminderRepo.ListByPlant(q, sourceID)
	if err != nil {
		return nil, fmt.Errorf("plant merge reminder list: %w", err)
	}
	pests, err := s.pestRepo.ListByPlantWith(q, sourceID)
	if err != nil {
		return nil, fmt.Errorf("plant merge pest list: %w", err)
	}

	gardenActions := make(map[uint]string, len(sourceGardens))
	gardenImpacts := make([]dto.MergeGardenImpact, 0, len(sourceGardens))
	gardenConflictUsers := make(map[uint]bool, len(keepGardens))
	for _, g := range keepGardens {
		gardenConflictUsers[g.UserID] = true
	}
	for _, g := range sourceGardens {
		action := dto.MergeActionMove
		if gardenConflictUsers[g.UserID] {
			action = dto.MergeActionRemoveDuplicate
		}
		gardenActions[g.ID] = action
		gardenImpacts = append(gardenImpacts, dto.MergeGardenImpact{
			GardenID: g.ID, UserID: g.UserID, Nickname: g.Nickname,
			Location: g.Location, CareReminderID: g.CareReminderID, Action: action,
		})
	}

	favoriteActions := make(map[uint]string, len(sourceFavorites))
	favoriteImpacts := make([]dto.MergeFavoriteImpact, 0, len(sourceFavorites))
	favoriteConflictUsers := make(map[uint]bool, len(keepFavorites))
	for _, f := range keepFavorites {
		favoriteConflictUsers[f.UserID] = true
	}
	for _, f := range sourceFavorites {
		action := dto.MergeActionMove
		if favoriteConflictUsers[f.UserID] {
			action = dto.MergeActionRemoveDuplicate
		}
		favoriteActions[f.ID] = action
		favoriteImpacts = append(favoriteImpacts, dto.MergeFavoriteImpact{
			FavoriteID: f.ID, UserID: f.UserID, Action: action,
		})
	}

	reminderImpacts := make([]dto.MergeReminderImpact, 0, len(reminders))
	pestImpacts := make([]dto.MergePestImpact, 0, len(pests))

	userIDs := make(map[uint]bool)
	for _, g := range sourceGardens {
		userIDs[g.UserID] = true
	}
	for _, f := range sourceFavorites {
		userIDs[f.UserID] = true
	}
	for _, m := range reminders {
		userIDs[m.UserID] = true
	}
	idList := make([]uint, 0, len(userIDs))
	for id := range userIDs {
		idList = append(idList, id)
	}
	sort.Slice(idList, func(i, j int) bool { return idList[i] < idList[j] })
	users, err := s.userRepo.ListByIDs(q, idList)
	if err != nil {
		return nil, fmt.Errorf("plant merge user list: %w", err)
	}
	userMap := make(map[uint]*model.User, len(users))
	for i := range users {
		userMap[users[i].ID] = &users[i]
	}
	userLabel := func(id uint) (username, nickname string) {
		if u, ok := userMap[id]; ok {
			return u.Username, u.Nickname
		}
		return "", ""
	}
	affected := make([]dto.MergeAffectedUser, 0, len(idList))
	for _, id := range idList {
		username, nickname := userLabel(id)
		affected = append(affected, dto.MergeAffectedUser{UserID: id, Username: username, Nickname: nickname})
	}
	for i := range gardenImpacts {
		gardenImpacts[i].Username, _ = userLabel(gardenImpacts[i].UserID)
	}
	for i := range favoriteImpacts {
		favoriteImpacts[i].Username, _ = userLabel(favoriteImpacts[i].UserID)
	}
	for _, m := range reminders {
		username, _ := userLabel(m.UserID)
		reminderImpacts = append(reminderImpacts, dto.MergeReminderImpact{
			ReminderID: m.ID, UserID: m.UserID, Username: username,
			TaskTitle: m.TaskTitle, RemindDate: m.RemindDate.Format(time.DateOnly),
		})
	}
	for _, p := range pests {
		pestImpacts = append(pestImpacts, dto.MergePestImpact{PestID: p.ID, Name: p.Name})
	}

	plan := &mergePlan{
		keep: keep, source: source,
		sourceGardens: sourceGardens, sourceFavorite: sourceFavorites,
		gardenAction: gardenActions, favoriteAction: favoriteActions,
		preview: dto.PlantMergePreview{
			KeepPlantID: keepID, KeepPlantName: keep.Name,
			SourcePlantID: sourceID, SourcePlantName: source.Name,
			AffectedUsers: affected, Gardens: gardenImpacts, Favorites: favoriteImpacts,
			Reminders: reminderImpacts, Pests: pestImpacts,
		},
	}
	return plan, nil
}

// truncate shortens s to at most n runes.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
