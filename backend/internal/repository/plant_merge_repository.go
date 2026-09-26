package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/gbplantwiki/gbplantwiki/internal/model"
)

// MergeGardenRow is one affected user's garden situation for a merge preview.
type MergeGardenRow struct {
	SourceGardenID   uint   `gorm:"column:source_garden_id"`
	TargetGardenID   uint   `gorm:"column:target_garden_id"`
	UserID           uint   `gorm:"column:user_id"`
	Username         string `gorm:"column:username"`
	UserNickname     string `gorm:"column:user_nickname"`
	SourceNickname   string `gorm:"column:source_nickname"`
	SourceLocation   string `gorm:"column:source_location"`
	SourceReminderID uint   `gorm:"column:source_reminder_id"`
	Collides         bool   `gorm:"column:collides"`
}

// MergeStuckError marks the exact source plant a merge transaction failed on.
// On this error the whole transaction is rolled back, so gardens, favorites,
// pests, reminders and plant records all stay in their pre-merge state.
type MergeStuckError struct {
	SourcePlantID uint   `json:"source_plant_id"`
	SourceName    string `json:"source_name"`
	Step          string `json:"step"`
	Reason        string `json:"reason"`
}

func (e *MergeStuckError) Error() string {
	return fmt.Sprintf("merge stuck at source_plant_id=%d step=%s: %s", e.SourcePlantID, e.Step, e.Reason)
}

// MergeResult summarizes one executed merge request.
type MergeResult struct {
	TargetID         uint   `json:"target_id"`
	MergedSources    []uint `json:"merged_source_ids"`
	GardenRelinked   int    `json:"garden_relinked"`
	GardenDeduped    int    `json:"garden_deduped"`
	FavoritesMoved   int    `json:"favorites_moved"`
	FavoritesDeduped int    `json:"favorites_deduped"`
	PestsMoved       int    `json:"pests_moved"`
	RemindersMoved   int    `json:"reminders_moved"`
	AffectedUsers    int    `json:"affected_users"`
}

// mergeState tracks rows already moved onto the kept plant within the current
// transaction so multiple duplicate sources never collide on a unique index
// (e.g. a user keeping three synonym plants must end with one garden row).
type mergeState struct {
	targetGardenByUser map[uint]model.UserGarden
	favUsers           map[uint]bool
}

// PlantMergeRepository handles merge previews, transactional execution and logs.
type PlantMergeRepository struct {
	db *gorm.DB
}

// NewPlantMergeRepository creates a PlantMergeRepository.
func NewPlantMergeRepository(db *gorm.DB) *PlantMergeRepository {
	return &PlantMergeRepository{db: db}
}

// PreviewPlants loads the target and candidate source plants for selection.
func (r *PlantMergeRepository) PreviewPlants(ids []uint) (map[uint]model.PlantSpecies, error) {
	plants := make(map[uint]model.PlantSpecies)
	if len(ids) == 0 {
		return plants, nil
	}
	var rows []model.PlantSpecies
	if err := r.db.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, p := range rows {
		plants[p.ID] = p
	}
	return plants, nil
}

// PreviewGardenUserCounts returns user_id -> number of selected source garden
// rows the user owns, used to detect collisions between the duplicates too.
func (r *PlantMergeRepository) PreviewGardenUserCounts(sourceIDs []uint) (map[uint]int, error) {
	counts := map[uint]int{}
	if len(sourceIDs) == 0 {
		return counts, nil
	}
	type row struct {
		UserID uint
		N      int
	}
	var rows []row
	if err := r.db.Model(&model.UserGarden{}).
		Select("user_id, COUNT(*) AS n").
		Where("plant_species_id IN ?", sourceIDs).
		Group("user_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, rw := range rows {
		counts[rw.UserID] = rw.N
	}
	return counts, nil
}

// PreviewGarden returns, per source plant, the garden rows pointing at it with
// an indication of whether the same user already keeps the target plant.
func (r *PlantMergeRepository) PreviewGarden(targetID uint, sourceIDs []uint) ([]MergeGardenRow, error) {
	var rows []MergeGardenRow
	if len(sourceIDs) == 0 {
		return rows, nil
	}
	// For every source garden row, look up a same-user garden row on the target.
	err := r.db.Table("user_gardens AS sg").
		Select("sg.id AS source_garden_id, tg.id AS target_garden_id, sg.user_id AS user_id, "+
			"u.username AS username, u.nickname AS user_nickname, sg.nickname AS source_nickname, "+
			"sg.location AS source_location, sg.care_reminder_id AS source_reminder_id, "+
			"CASE WHEN tg.id IS NOT NULL THEN TRUE ELSE FALSE END AS collides").
		Joins("JOIN users AS u ON u.id = sg.user_id").
		Joins("LEFT JOIN user_gardens AS tg ON tg.user_id = sg.user_id AND tg.plant_species_id = ?", targetID).
		Where("sg.plant_species_id IN ?", sourceIDs).
		Order("sg.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// PreviewFavorites returns plant favorites pointing at any source plant.
func (r *PlantMergeRepository) PreviewFavorites(sourceIDs []uint) ([]model.Favorite, error) {
	var rows []model.Favorite
	if len(sourceIDs) == 0 {
		return rows, nil
	}
	if err := r.db.Where("target_type = ? AND target_id IN ?", "plant", sourceIDs).
		Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// PreviewPests returns disease/pest entries pointing at any source plant.
func (r *PlantMergeRepository) PreviewPests(sourceIDs []uint) ([]model.DiseasePest, error) {
	var rows []model.DiseasePest
	if len(sourceIDs) == 0 {
		return rows, nil
	}
	if err := r.db.Where("plant_species_id IN ?", sourceIDs).
		Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// PreviewReminders returns care reminders pointing at any source plant.
func (r *PlantMergeRepository) PreviewReminders(sourceIDs []uint) ([]model.CareReminder, error) {
	var rows []model.CareReminder
	if len(sourceIDs) == 0 {
		return rows, nil
	}
	if err := r.db.Where("plant_species_id IN ?", sourceIDs).
		Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// UsersByID returns a map of user id -> user for snapshot enrichment.
func (r *PlantMergeRepository) UsersByID(ids []uint) (map[uint]model.User, error) {
	users := make(map[uint]model.User)
	if len(ids) == 0 {
		return users, nil
	}
	var rows []model.User
	if err := r.db.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, u := range rows {
		users[u.ID] = u
	}
	return users, nil
}

// Execute runs the whole merge inside one transaction. Any error rolls back
// every garden/favorite/pest/reminder/plant change to the pre-merge state and
// returns a MergeStuckError describing the source plant that blocked progress.
func (r *PlantMergeRepository) Execute(adminID, targetID uint, sourceIDs []uint) (*MergeResult, error) {
	result := &MergeResult{TargetID: targetID}
	affectedUsers := map[uint]struct{}{}

	err := r.db.Transaction(func(tx *gorm.DB) error {
		// Lock the target and the sources for the duration of the merge.
		var target model.PlantSpecies
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&target, targetID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &MergeStuckError{SourcePlantID: 0, Step: "load_target", Reason: "保留项不存在"}
			}
			return err
		}
		if target.MergedIntoID != 0 {
			return &MergeStuckError{SourcePlantID: 0, Step: "load_target",
				Reason: fmt.Sprintf("保留项「%s」已被归并，不能再作为保留项", target.Name)}
		}

		var sources []model.PlantSpecies
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ?", sourceIDs).Find(&sources).Error; err != nil {
			return err
		}
		found := map[uint]bool{}
		for _, s := range sources {
			found[s.ID] = true
		}
		for _, sid := range sourceIDs {
			if !found[sid] {
				return &MergeStuckError{SourcePlantID: sid, Step: "load_source", Reason: "待并入项不存在"}
			}
		}
		for _, source := range sources {
			if source.MergedIntoID != 0 {
				return &MergeStuckError{SourcePlantID: source.ID, SourceName: source.Name,
					Step: "validate", Reason: "该品种已被归并，不能重复归并"}
			}
		}

		state := &mergeState{
			targetGardenByUser: map[uint]model.UserGarden{},
			favUsers:           map[uint]bool{},
		}
		var existingTargetGardens []model.UserGarden
		if err := tx.Where("plant_species_id = ?", target.ID).Find(&existingTargetGardens).Error; err != nil {
			return err
		}
		for _, tg := range existingTargetGardens {
			state.targetGardenByUser[tg.UserID] = tg
		}
		var existingTargetFavUsers []uint
		if err := tx.Model(&model.Favorite{}).
			Where("target_type = ? AND target_id = ?", "plant", target.ID).
			Pluck("user_id", &existingTargetFavUsers).Error; err != nil {
			return err
		}
		for _, uid := range existingTargetFavUsers {
			state.favUsers[uid] = true
		}

		for _, source := range sources {
			if err := mergeOneSource(tx, adminID, target, source, result, affectedUsers, state); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	result.AffectedUsers = len(affectedUsers)
	result.MergedSources = sourceIDs
	return result, nil
}

// mergeOneSource merges a single source plant into the target. Runs inside tx.
func mergeOneSource(tx *gorm.DB, adminID uint, target model.PlantSpecies, source model.PlantSpecies,
	result *MergeResult, affectedUsers map[uint]struct{}, state *mergeState) error {
	stuck := func(step, reason string) error {
		return &MergeStuckError{SourcePlantID: source.ID, SourceName: source.Name, Step: step, Reason: reason}
	}

	// ---- 1. Garden: one row per (user, kept plant) ---------------------
	var sourceGardens []model.UserGarden
	if err := tx.Where("plant_species_id = ?", source.ID).Find(&sourceGardens).Error; err != nil {
		return err
	}

	userIDs := map[uint]struct{}{}
	for _, g := range sourceGardens {
		userIDs[g.UserID] = struct{}{}
	}
	userMap, err := repoLoadUsers(tx, keysOf(userIDs))
	if err != nil {
		return err
	}

	snapshots := []model.GardenMergeSnapshot{}
	gardenDeduped := 0
	gardenRelinked := 0
	for _, sg := range sourceGardens {
		affectedUsers[sg.UserID] = struct{}{}
		u := userMap[sg.UserID]
		snap := model.GardenMergeSnapshot{
			UserID:         sg.UserID,
			Username:       u.Username,
			Nickname:       sg.Nickname,
			Location:       sg.Location,
			CareReminderID: sg.CareReminderID,
		}
		if tg, ok := state.targetGardenByUser[sg.UserID]; ok {
			// User already has a row on the kept plant (from target or an earlier
			// source in this merge): drop the duplicate row, keep one. If the kept
			// row has no reminder binding yet, carry the old one over.
			if tg.CareReminderID == 0 && sg.CareReminderID != 0 {
				if err := tx.Model(&model.UserGarden{}).Where("id = ?", tg.ID).
					Update("care_reminder_id", sg.CareReminderID).Error; err != nil {
					return stuck("garden_carry_reminder", err.Error())
				}
				tg.CareReminderID = sg.CareReminderID
				state.targetGardenByUser[sg.UserID] = tg
			}
			if err := tx.Delete(&model.UserGarden{}, sg.ID).Error; err != nil {
				return stuck("garden_dedupe", err.Error())
			}
			snap.Action = "dedupe"
			gardenDeduped++
		} else {
			// Only source row: repoint it at the kept plant.
			if err := tx.Model(&model.UserGarden{}).Where("id = ?", sg.ID).
				Update("plant_species_id", target.ID).Error; err != nil {
				if isDuplicate(err) {
					return stuck("garden_relink", "归并后与保留项花园记录冲突")
				}
				return stuck("garden_relink", err.Error())
			}
			sg.PlantSpeciesID = target.ID
			state.targetGardenByUser[sg.UserID] = sg
			snap.Action = "relink"
			gardenRelinked++
		}
		snapshots = append(snapshots, snap)
	}

	// ---- 2. Favorites: unique (user, plant) ----------------------------
	var sourceFavs []model.Favorite
	if err := tx.Where("target_type = ? AND target_id = ?", "plant", source.ID).
		Find(&sourceFavs).Error; err != nil {
		return err
	}
	favoritesMoved := 0
	favoritesDeduped := 0
	for _, f := range sourceFavs {
		affectedUsers[f.UserID] = struct{}{}
		if state.favUsers[f.UserID] {
			if err := tx.Delete(&model.Favorite{}, f.ID).Error; err != nil {
				return stuck("favorite_dedupe", err.Error())
			}
			favoritesDeduped++
			continue
		}
		if err := tx.Model(&model.Favorite{}).Where("id = ?", f.ID).
			Update("target_id", target.ID).Error; err != nil {
			if isDuplicate(err) {
				return stuck("favorite_relink", "归并后与保留项收藏冲突")
			}
			return stuck("favorite_relink", err.Error())
		}
		state.favUsers[f.UserID] = true
		favoritesMoved++
	}

	// ---- 3. Disease / pest entries -------------------------------------
	pestRes := tx.Model(&model.DiseasePest{}).
		Where("plant_species_id = ?", source.ID).
		Update("plant_species_id", target.ID)
	if pestRes.Error != nil {
		return stuck("pest_relink", pestRes.Error.Error())
	}
	pestsMoved := int(pestRes.RowsAffected)

	// ---- 4. Care reminders ---------------------------------------------
	var reminderRows []model.CareReminder
	if err := tx.Where("plant_species_id = ?", source.ID).Find(&reminderRows).Error; err != nil {
		return err
	}
	reminderUserIDs := map[uint]struct{}{}
	for _, rm := range reminderRows {
		affectedUsers[rm.UserID] = struct{}{}
		reminderUserIDs[rm.UserID] = struct{}{}
	}
	remRes := tx.Model(&model.CareReminder{}).
		Where("plant_species_id = ?", source.ID).
		Update("plant_species_id", target.ID)
	if remRes.Error != nil {
		return stuck("reminder_relink", remRes.Error.Error())
	}
	remindersMoved := int(remRes.RowsAffected)

	// ---- 5. Absorb source name/alias into the kept plant ---------------
	mergedAlias := mergeAlias(target.Alias, source.Name, source.Alias)

	// ---- 6. Hide the source from the public list -----------------------
	if err := tx.Model(&model.PlantSpecies{}).Where("id = ?", source.ID).
		Updates(map[string]interface{}{"merged_into_id": target.ID}).Error; err != nil {
		return stuck("mark_merged", err.Error())
	}

	// Persist alias absorption (best-effort; a zero-length change is skipped by GORM).
	if mergedAlias != target.Alias {
		if err := tx.Model(&model.PlantSpecies{}).Where("id = ?", target.ID).
			Update("alias", mergedAlias).Error; err != nil {
			return stuck("alias_absorb", err.Error())
		}
	}

	// ---- 7. Write the merge log ----------------------------------------
	snapJSON, _ := json.Marshal(snapshots)
	affectedForSource := map[uint]struct{}{}
	for _, s := range snapshots {
		affectedForSource[s.UserID] = struct{}{}
	}
	for _, f := range sourceFavs {
		affectedForSource[f.UserID] = struct{}{}
	}
	for uid := range reminderUserIDs {
		affectedForSource[uid] = struct{}{}
	}
	log := model.PlantMergeLog{
		PlantSpeciesID:    target.ID,
		TargetName:        target.Name,
		SourcePlantID:     source.ID,
		SourceName:        source.Name,
		SourceAlias:       source.Alias,
		AdminID:           adminID,
		Status:            model.PlantMergeStatusSuccess,
		GardenCount:       gardenRelinked + gardenDeduped,
		FavoriteCount:     favoritesMoved + favoritesDeduped,
		PestCount:         pestsMoved,
		ReminderCount:     remindersMoved,
		AffectedUserCount: len(affectedForSource),
		GardenSnapshots:   string(snapJSON),
	}
	if err := tx.Create(&log).Error; err != nil {
		return stuck("write_log", err.Error())
	}

	result.GardenRelinked += gardenRelinked
	result.GardenDeduped += gardenDeduped
	result.FavoritesMoved += favoritesMoved
	result.FavoritesDeduped += favoritesDeduped
	result.PestsMoved += pestsMoved
	result.RemindersMoved += remindersMoved
	return nil
}

// RecordFailure persists a failed merge attempt outside the rolled-back
// transaction so the stuck source plant stays marked in the merge history.
func (r *PlantMergeRepository) RecordFailure(adminID, targetID uint, targetName string,
	sourceIDs []uint, stuck *MergeStuckError) error {
	logs := []model.PlantMergeLog{}
	// Lock-free reads just for enriching names; tolerate stale/missing rows.
	var plants []model.PlantSpecies
	_ = r.db.Where("id IN ?", sourceIDs).Find(&plants).Error
	nameByID := map[uint]model.PlantSpecies{}
	for _, p := range plants {
		nameByID[p.ID] = p
	}
	reason := stuck.Reason
	if len(reason) > 500 {
		reason = reason[:500]
	}
	// A merge is all-or-nothing: mark every selected source as blocked, with the
	// actually stuck one carrying the step and reason.
	for _, sid := range sourceIDs {
		entry := model.PlantMergeLog{
			AdminID:         adminID,
			Status:          model.PlantMergeStatusFailed,
			GardenSnapshots: "[]",
		}
		if targetID != 0 {
			entry.PlantSpeciesID = targetID
			entry.TargetName = targetName
		}
		if p, ok := nameByID[sid]; ok {
			entry.SourcePlantID = p.ID
			entry.SourceName = p.Name
			entry.SourceAlias = p.Alias
		} else {
			entry.SourcePlantID = sid
			entry.SourceName = fmt.Sprintf("品种#%d", sid)
		}
		if sid == stuck.SourcePlantID || stuck.SourcePlantID == 0 {
			entry.FailedStep = stuck.Step
			entry.ErrorMessage = reason
		} else {
			entry.FailedStep = "aborted"
			entry.ErrorMessage = fmt.Sprintf("因待并入项 %d 归并失败，本次归并整体回滚未执行", stuck.SourcePlantID)
		}
		logs = append(logs, entry)
	}
	if len(logs) == 0 {
		logs = append(logs, model.PlantMergeLog{
			AdminID:         adminID,
			PlantSpeciesID:  targetID,
			TargetName:      targetName,
			SourcePlantID:   stuck.SourcePlantID,
			SourceName:      stuck.SourceName,
			Status:          model.PlantMergeStatusFailed,
			FailedStep:      stuck.Step,
			ErrorMessage:    reason,
			GardenSnapshots: "[]",
		})
	}
	return r.db.Create(&logs).Error
}

// ListLogs paginates merge logs, newest first, optionally filtered by status.
func (r *PlantMergeRepository) ListLogs(status string, page, pageSize int) ([]model.PlantMergeLog, int64, error) {
	var rows []model.PlantMergeLog
	var total int64
	q := r.db.Model(&model.PlantMergeLog{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ListMergedPlants returns plants already hidden by a merge.
func (r *PlantMergeRepository) ListMergedPlants(keyword string, page, pageSize int) ([]model.PlantSpecies, int64, error) {
	var rows []model.PlantSpecies
	var total int64
	q := r.db.Model(&model.PlantSpecies{}).Where("merged_into_id > 0")
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("name LIKE ? OR alias LIKE ?", like, like)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// mergeAlias appends the source name and alias tokens to the kept plant's alias
// so old names remain searchable, without duplicating existing tokens.
func mergeAlias(existing string, tokens ...string) string {
	seen := map[string]bool{}
	var out []string
	appendToken := func(raw string) {
		for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '，' || r == '、' || r == '/' }) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if !seen[part] {
				seen[part] = true
				out = append(out, part)
			}
		}
	}
	appendToken(existing)
	for _, t := range tokens {
		appendToken(t)
	}
	return strings.Join(out, "、")
}

// repoLoadUsers loads users by id inside a transaction.
func repoLoadUsers(tx *gorm.DB, ids []uint) (map[uint]model.User, error) {
	users := map[uint]model.User{}
	if len(ids) == 0 {
		return users, nil
	}
	var rows []model.User
	if err := tx.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, u := range rows {
		users[u.ID] = u
	}
	return users, nil
}

func keysOf(m map[uint]struct{}) []uint {
	out := make([]uint, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
