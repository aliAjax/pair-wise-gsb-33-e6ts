package service

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/gbplantwiki/gbplantwiki/internal/constants"
	"github.com/gbplantwiki/gbplantwiki/internal/dto"
	"github.com/gbplantwiki/gbplantwiki/internal/model"
	"github.com/gbplantwiki/gbplantwiki/internal/repository"
	"github.com/gbplantwiki/gbplantwiki/internal/util"
)

// PlantMergeService implements the admin merge console: preview then atomic execute.
type PlantMergeService struct {
	repo   *repository.PlantMergeRepository
	logger *slog.Logger
}

// NewPlantMergeService creates a PlantMergeService.
func NewPlantMergeService(repo *repository.PlantMergeRepository, logger *slog.Logger) *PlantMergeService {
	return &PlantMergeService{repo: repo, logger: logger}
}

// Preview collects every user and association that would be affected.
func (s *PlantMergeService) Preview(targetID uint, sourceIDs []uint) (*dto.MergePreview, error) {
	ids := append(append([]uint{}, sourceIDs...), targetID)
	plants, err := s.repo.PreviewPlants(ids)
	if err != nil {
		return nil, fmt.Errorf("merge preview load plants: %w", err)
	}
	target, ok := plants[targetID]
	if !ok {
		return nil, util.NewAppError(404, constants.CodeNotFound,
			fmt.Sprintf("PlantSpecies[id=%d] merge preview failed: kept plant not found", targetID))
	}
	if target.MergedIntoID != 0 {
		return nil, util.NewAppError(422, constants.CodeValidationError,
			fmt.Sprintf("PlantSpecies[id=%d] merge preview failed: kept plant is already merged away", targetID))
	}

	uniqSources := dedupeIDs(sourceIDs)
	var sources []model.PlantSpecies
	var missing []uint
	for _, sid := range uniqSources {
		if sid == targetID {
			return nil, util.NewAppError(422, constants.CodeValidationError,
				"merge preview failed: kept plant cannot also be a duplicate")
		}
		p, ok := plants[sid]
		if !ok {
			missing = append(missing, sid)
			continue
		}
		if p.MergedIntoID != 0 {
			return nil, util.NewAppError(422, constants.CodeValidationError,
				fmt.Sprintf("PlantSpecies[id=%d] merge preview failed: plant already merged into #%d", sid, p.MergedIntoID))
		}
		sources = append(sources, p)
	}
	if len(missing) > 0 {
		return nil, util.NewAppError(404, constants.CodeNotFound,
			fmt.Sprintf("merge preview failed: duplicate plant(s) not found: %v", missing))
	}

	gardenRows, err := s.repo.PreviewGarden(targetID, uniqSources)
	if err != nil {
		return nil, fmt.Errorf("merge preview garden: %w", err)
	}
	gardenUserCounts, err := s.repo.PreviewGardenUserCounts(uniqSources)
	if err != nil {
		return nil, fmt.Errorf("merge preview garden counts: %w", err)
	}
	favorites, err := s.repo.PreviewFavorites(uniqSources)
	if err != nil {
		return nil, fmt.Errorf("merge preview favorites: %w", err)
	}
	pests, err := s.repo.PreviewPests(uniqSources)
	if err != nil {
		return nil, fmt.Errorf("merge preview pests: %w", err)
	}
	reminders, err := s.repo.PreviewReminders(uniqSources)
	if err != nil {
		return nil, fmt.Errorf("merge preview reminders: %w", err)
	}

	preview := &dto.MergePreview{
		TargetPlant:  target,
		SourcePlants: sources,
		Favorites:    favorites,
		Pests:        pests,
		Reminders:    reminders,
		GardenRows:   make([]dto.MergePreviewGardenRow, 0, len(gardenRows)),
	}

	userAgg := map[uint]*dto.MergePreviewUser{}
	ensureUser := func(id uint, uname, nick string) *dto.MergePreviewUser {
		u := userAgg[id]
		if u == nil {
			u = &dto.MergePreviewUser{UserID: id, Username: uname, Nickname: nick}
			userAgg[id] = u
		}
		return u
	}

	collisions := 0
	for _, g := range gardenRows {
		// Collision when the user keeps the target plant OR keeps more than one
		// of the selected duplicates — either way only one row can survive.
		collides := g.Collides || gardenUserCounts[g.UserID] > 1
		action := "relink"
		if collides {
			action = "dedupe"
			collisions++
		}
		preview.GardenRows = append(preview.GardenRows, dto.MergePreviewGardenRow{
			SourceGardenID:   g.SourceGardenID,
			TargetGardenID:   g.TargetGardenID,
			UserID:           g.UserID,
			Username:         g.Username,
			UserNickname:     g.UserNickname,
			SourceNickname:   g.SourceNickname,
			SourceLocation:   g.SourceLocation,
			SourceReminderID: g.SourceReminderID,
			Collides:         collides,
			Action:           action,
		})
		u := ensureUser(g.UserID, g.Username, g.UserNickname)
		u.GardenRows++
		if collides {
			u.Collides = true
		}
	}
	for _, f := range favorites {
		if u := userAgg[f.UserID]; u != nil {
			u.FavoriteRows++
		} else {
			userAgg[f.UserID] = &dto.MergePreviewUser{UserID: f.UserID, FavoriteRows: 1}
		}
	}
	for _, r := range reminders {
		if u := userAgg[r.UserID]; u != nil {
			u.ReminderRows++
		} else {
			userAgg[r.UserID] = &dto.MergePreviewUser{UserID: r.UserID, ReminderRows: 1}
		}
	}

	// Enrich users that only appear through favorites/reminders.
	var missingUserIDs []uint
	for id, u := range userAgg {
		if u.Username == "" {
			missingUserIDs = append(missingUserIDs, id)
		}
	}
	if len(missingUserIDs) > 0 {
		users, err := s.repo.UsersByID(missingUserIDs)
		if err != nil {
			return nil, fmt.Errorf("merge preview users: %w", err)
		}
		for id, m := range users {
			if u := userAgg[id]; u != nil {
				u.Username = m.Username
				u.Nickname = m.Nickname
			}
		}
	}

	for _, u := range userAgg {
		preview.AffectedUsers = append(preview.AffectedUsers, *u)
	}
	preview.Summary = dto.MergePreviewSummary{
		GardenRows:       len(gardenRows),
		GardenCollisions: collisions,
		FavoriteRows:     len(favorites),
		PestRows:         len(pests),
		ReminderRows:     len(reminders),
		AffectedUsers:    len(userAgg),
	}
	return preview, nil
}

// Execute runs the merge atomically. On failure all data stays in the
// pre-merge state and a failed log marks the stuck source plant.
func (s *PlantMergeService) Execute(adminID uint, targetID uint, sourceIDs []uint) (*repository.MergeResult, *repository.MergeStuckError, error) {
	target, err := s.repo.PreviewPlants([]uint{targetID})
	if err != nil {
		return nil, nil, fmt.Errorf("merge execute load target: %w", err)
	}
	t, ok := target[targetID]
	targetName := ""
	if ok {
		targetName = t.Name
	}

	result, execErr := s.repo.Execute(adminID, targetID, dedupeIDs(sourceIDs))
	if execErr == nil {
		s.logger.Info("plant species merge success",
			"target_id", targetID, "source_ids", sourceIDs, "admin_id", adminID,
			"garden_relinked", result.GardenRelinked, "garden_deduped", result.GardenDeduped,
			"favorites_moved", result.FavoritesMoved, "favorites_deduped", result.FavoritesDeduped,
			"pests_moved", result.PestsMoved, "reminders_moved", result.RemindersMoved)
		return result, nil, nil
	}

	var stuck *repository.MergeStuckError
	if errors.As(execErr, &stuck) {
		s.logger.Error("plant species merge stuck, rolled back",
			"target_id", targetID, "source_plant_id", stuck.SourcePlantID,
			"step", stuck.Step, "reason", stuck.Reason, "admin_id", adminID)
	} else {
		s.logger.Error("plant species merge failed, rolled back",
			"target_id", targetID, "error", execErr, "admin_id", adminID)
		stuck = &repository.MergeStuckError{Step: "unknown", Reason: execErr.Error()}
	}
	// Persist the failure history outside the rolled-back transaction.
	if logErr := s.repo.RecordFailure(adminID, targetID, targetName, dedupeIDs(sourceIDs), stuck); logErr != nil {
		s.logger.Error("failed to persist merge failure log", "error", logErr)
	}
	return nil, stuck, execErr
}

// ListLogs returns merge history (success and failure).
func (s *PlantMergeService) ListLogs(status string, page, pageSize int) ([]model.PlantMergeLog, int64, error) {
	logs, total, err := s.repo.ListLogs(status, page, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("merge log list: %w", err)
	}
	return logs, total, nil
}

// ListMergedPlants returns plants already hidden by merges.
func (s *PlantMergeService) ListMergedPlants(keyword string, page, pageSize int) ([]model.PlantSpecies, int64, error) {
	plants, total, err := s.repo.ListMergedPlants(keyword, page, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("merged plant list: %w", err)
	}
	return plants, total, nil
}

func dedupeIDs(ids []uint) []uint {
	seen := map[uint]bool{}
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
