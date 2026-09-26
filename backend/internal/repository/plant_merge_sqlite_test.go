//go:build integration_sqlite

// Integration tests for the plant merge flow against a real (SQLite) database.
// These tests exercise actual transaction commit/rollback semantics and are
// opt-in via the integration_sqlite build tag (CGO required).
//
//	go test -tags=integration_sqlite -run TestIntegration ./internal/repository/
package repository

import (
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/gbplantwiki/gbplantwiki/internal/model"
)

func newIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.PlantSpecies{}, &model.PlantMergeLog{},
		&model.DiseasePest{}, &model.CareReminder{}, &model.Favorite{}, &model.UserGarden{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Shared in-memory DB: clean between tests.
	for _, tbl := range []string{"user_gardens", "favorites", "care_reminders", "disease_pests", "plant_merge_logs", "plant_species", "users"} {
		if err := db.Exec("DELETE FROM " + tbl).Error; err != nil {
			t.Fatalf("clean %s: %v", tbl, err)
		}
	}
	return db
}

func seedMergeScenario(t *testing.T, db *gorm.DB) (target, source model.PlantSpecies, users []model.User) {
	t.Helper()
	target = model.PlantSpecies{Name: "龟背竹", Alias: "蓬莱蕉", Type: "foliage", ImageURLs: "[]"}
	source = model.PlantSpecies{Name: "电线草", Alias: "龟背芋", Type: "foliage", ImageURLs: "[]"}
	if err := db.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	users = []model.User{
		{Username: "alice", Email: "a@x", Nickname: "Alice", Role: "user"},
		{Username: "bob", Email: "b@x", Nickname: "Bob", Role: "user"},
	}
	for i := range users {
		if err := db.Create(&users[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Alice keeps BOTH plants -> collision, must collapse to one garden row.
	db.Create(&model.UserGarden{UserID: users[0].ID, PlantSpeciesID: target.ID, Nickname: "大龟", Location: "客厅", CareReminderID: 0})
	db.Create(&model.UserGarden{UserID: users[0].ID, PlantSpeciesID: source.ID, Nickname: "旧名小草", Location: "阳台", CareReminderID: 77})
	// Bob keeps only the source -> row relinked.
	db.Create(&model.UserGarden{UserID: users[1].ID, PlantSpeciesID: source.ID, Nickname: "波波的草", Location: "书房", CareReminderID: 88})
	// Favorites: Alice favorite both -> dedupe; Bob only source -> relink.
	db.Create(&model.Favorite{UserID: users[0].ID, TargetType: "plant", TargetID: target.ID})
	db.Create(&model.Favorite{UserID: users[0].ID, TargetType: "plant", TargetID: source.ID})
	db.Create(&model.Favorite{UserID: users[1].ID, TargetType: "plant", TargetID: source.ID})
	// Pest + reminder on source.
	db.Create(&model.DiseasePest{PlantSpeciesID: source.ID, Name: "叶斑病"})
	db.Create(&model.CareReminder{UserID: users[1].ID, PlantSpeciesID: source.ID, TaskTitle: "擦叶子", Status: "pending"})
	return target, source, users
}

func TestIntegrationMergeHappyPath(t *testing.T) {
	db := newIntegrationDB(t)
	repo := NewPlantMergeRepository(db)
	target, source, users := seedMergeScenario(t, db)

	res, err := repo.Execute(99, target.ID, []uint{source.ID})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.GardenDeduped != 1 || res.GardenRelinked != 1 {
		t.Errorf("garden counts: relinked=%d deduped=%d", res.GardenRelinked, res.GardenDeduped)
	}
	if res.FavoritesDeduped != 1 || res.FavoritesMoved != 1 {
		t.Errorf("favorite counts: moved=%d deduped=%d", res.FavoritesMoved, res.FavoritesDeduped)
	}
	if res.PestsMoved != 1 || res.RemindersMoved != 1 || res.AffectedUsers != 2 {
		t.Errorf("pest/reminder/users: %+v", res)
	}

	// One garden row per user, both on the kept plant.
	var gardenCount int64
	db.Model(&model.UserGarden{}).Where("user_id IN ? AND plant_species_id = ?",
		[]uint{users[0].ID, users[1].ID}, target.ID).Count(&gardenCount)
	if gardenCount != 2 {
		t.Errorf("expected 2 garden rows on target, got %d", gardenCount)
	}
	var sourceGarden int64
	db.Model(&model.UserGarden{}).Where("plant_species_id = ?", source.ID).Count(&sourceGarden)
	if sourceGarden != 0 {
		t.Errorf("source garden rows remain: %d", sourceGarden)
	}

	// Alice's kept row picked up the old reminder binding (77) she was missing.
	var aliceGarden model.UserGarden
	if err := db.Where("user_id = ? AND plant_species_id = ?", users[0].ID, target.ID).First(&aliceGarden).Error; err != nil {
		t.Fatal(err)
	}
	if aliceGarden.CareReminderID != 77 {
		t.Errorf("alice reminder carry-over = %d, want 77", aliceGarden.CareReminderID)
	}
	if aliceGarden.Nickname != "大龟" {
		t.Errorf("alice kept row nickname changed: %q", aliceGarden.Nickname)
	}

	// Favorites: exactly one plant favorite per user, on target.
	var favCount int64
	db.Model(&model.Favorite{}).Where("target_type = ? AND target_id = ?", "plant", target.ID).Count(&favCount)
	if favCount != 2 {
		t.Errorf("expected 2 favorites on target, got %d", favCount)
	}

	// Source hidden from public list; target absorbed its names as alias.
	var refreshedSource model.PlantSpecies
	db.First(&refreshedSource, source.ID)
	if refreshedSource.MergedIntoID != target.ID {
		t.Errorf("source merged_into_id = %d, want %d", refreshedSource.MergedIntoID, target.ID)
	}
	var refreshedTarget model.PlantSpecies
	db.First(&refreshedTarget, target.ID)
	if refreshedTarget.Alias == "蓬莱蕉" {
		t.Errorf("alias not absorbed: %q", refreshedTarget.Alias)
	}

	// Success merge log with garden snapshots written.
	var logs []model.PlantMergeLog
	if err := db.Where("source_plant_id = ?", source.ID).Find(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Status != model.PlantMergeStatusSuccess {
		t.Fatalf("expected 1 success log, got %+v", logs)
	}
	if logs[0].GardenCount != 2 || logs[0].GardenSnapshots == "" || logs[0].GardenSnapshots == "[]" {
		t.Errorf("log missing garden snapshots: %+v", logs[0])
	}

	// Public list hides source.
	var public []model.PlantSpecies
	if err := db.Where("merged_into_id = 0").Find(&public).Error; err != nil {
		t.Fatal(err)
	}
	for _, p := range public {
		if p.ID == source.ID {
			t.Error("source plant still visible in public list")
		}
	}
}

func TestIntegrationMergeRollbackKeepsPreMergeState(t *testing.T) {
	db := newIntegrationDB(t)
	repo := NewPlantMergeRepository(db)
	target, source, _ := seedMergeScenario(t, db)

	// Force every subsequent UPDATE/DELETE on user_gardens to fail.
	callbackName := "test_fail_garden_writes"
	db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "user_gardens" {
			tx.AddError(errors.New("forced garden write failure"))
		}
	})
	db.Callback().Delete().Before("gorm:delete").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "user_gardens" {
			tx.AddError(errors.New("forced garden delete failure"))
		}
	})
	t.Cleanup(func() {
		db.Callback().Update().Remove(callbackName)
		db.Callback().Delete().Remove(callbackName)
	})

	_, err := repo.Execute(99, target.ID, []uint{source.ID})
	if err == nil {
		t.Fatal("expected merge failure, got nil")
	}
	var stuck *MergeStuckError
	if !errors.As(err, &stuck) {
		t.Fatalf("expected MergeStuckError, got %T %v", err, err)
	}
	if stuck.SourcePlantID != source.ID {
		t.Errorf("stuck source = %d, want %d", stuck.SourcePlantID, source.ID)
	}

	// Everything stays in the pre-merge state.
	var refreshedSource model.PlantSpecies
	db.First(&refreshedSource, source.ID)
	if refreshedSource.MergedIntoID != 0 {
		t.Errorf("source marked merged despite rollback: %d", refreshedSource.MergedIntoID)
	}
	var sourceGardens int64
	db.Model(&model.UserGarden{}).Where("plant_species_id = ?", source.ID).Count(&sourceGardens)
	if sourceGardens != 2 {
		t.Errorf("source garden rows changed: %d, want 2", sourceGardens)
	}
	var targetGardens int64
	db.Model(&model.UserGarden{}).Where("plant_species_id = ?", target.ID).Count(&targetGardens)
	if targetGardens != 1 {
		t.Errorf("target garden rows changed: %d, want 1", targetGardens)
	}
	var sourceFavs int64
	db.Model(&model.Favorite{}).Where("target_id = ?", source.ID).Count(&sourceFavs)
	if sourceFavs != 2 {
		t.Errorf("source favorites changed: %d, want 2", sourceFavs)
	}
	var pestsOnSource int64
	db.Model(&model.DiseasePest{}).Where("plant_species_id = ?", source.ID).Count(&pestsOnSource)
	if pestsOnSource != 1 {
		t.Errorf("pest moved despite rollback: %d", pestsOnSource)
	}
	var successLogs int64
	db.Model(&model.PlantMergeLog{}).Where("status = ?", model.PlantMergeStatusSuccess).Count(&successLogs)
	if successLogs != 0 {
		t.Errorf("success log survived rollback: %d", successLogs)
	}

	// RecordFailure (called by the service outside the tx) marks the stuck entry.
	if err := repo.RecordFailure(99, target.ID, target.Name, []uint{source.ID}, stuck); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	var failedLog model.PlantMergeLog
	if err := db.Where("status = ? AND source_plant_id = ?", model.PlantMergeStatusFailed, source.ID).
		First(&failedLog).Error; err != nil {
		t.Fatalf("failed log not found: %v", err)
	}
	if failedLog.FailedStep == "" {
		t.Error("failed log missing failed_step marker")
	}
}

func TestIntegrationRejectsMergingAlreadyMergedSource(t *testing.T) {
	db := newIntegrationDB(t)
	repo := NewPlantMergeRepository(db)
	target, source, _ := seedMergeScenario(t, db)

	if _, err := repo.Execute(99, target.ID, []uint{source.ID}); err != nil {
		t.Fatalf("first merge: %v", err)
	}
	// Second merge reusing the now-hidden source must be rejected and roll back.
	_, err := repo.Execute(99, target.ID, []uint{source.ID})
	var stuck *MergeStuckError
	if !errors.As(err, &stuck) || stuck.Step != "validate" {
		t.Fatalf("expected validate MergeStuckError, got %v", err)
	}
}

// TestIntegrationMultipleSourcesCrossCollision merges two synonyms where the
// same user keeps neither of the target but BOTH sources: the row must end up
// relinked once and deduped once, leaving exactly one row on the kept plant.
func TestIntegrationMultipleSourcesCrossCollision(t *testing.T) {
	db := newIntegrationDB(t)
	repo := NewPlantMergeRepository(db)

	target := model.PlantSpecies{Name: "龟背竹", Type: "foliage", ImageURLs: "[]"}
	s1 := model.PlantSpecies{Name: "电线草", Type: "foliage", ImageURLs: "[]"}
	s2 := model.PlantSpecies{Name: "龟背芋", Type: "foliage", ImageURLs: "[]"}
	db.Create(&target)
	db.Create(&s1)
	db.Create(&s2)
	user := model.User{Username: "carol", Email: "c@x", Nickname: "Carol", Role: "user"}
	db.Create(&user)
	// User keeps both synonyms but not the kept plant.
	db.Create(&model.UserGarden{UserID: user.ID, PlantSpeciesID: s1.ID, Nickname: "草", Location: "阳台", CareReminderID: 1})
	db.Create(&model.UserGarden{UserID: user.ID, PlantSpeciesID: s2.ID, Nickname: "芋", Location: "书房", CareReminderID: 2})
	db.Create(&model.Favorite{UserID: user.ID, TargetType: "plant", TargetID: s1.ID})
	db.Create(&model.Favorite{UserID: user.ID, TargetType: "plant", TargetID: s2.ID})

	res, err := repo.Execute(99, target.ID, []uint{s1.ID, s2.ID})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.GardenRelinked != 1 || res.GardenDeduped != 1 {
		t.Errorf("garden: relinked=%d deduped=%d", res.GardenRelinked, res.GardenDeduped)
	}
	if res.FavoritesMoved != 1 || res.FavoritesDeduped != 1 {
		t.Errorf("favorites: moved=%d deduped=%d", res.FavoritesMoved, res.FavoritesDeduped)
	}

	var gardenCount int64
	db.Model(&model.UserGarden{}).Where("user_id = ? AND plant_species_id = ?", user.ID, target.ID).
		Count(&gardenCount)
	if gardenCount != 1 {
		t.Errorf("expected exactly 1 garden row on target, got %d", gardenCount)
	}
	var favCount int64
	db.Model(&model.Favorite{}).Where("user_id = ? AND target_type = ? AND target_id = ?",
		user.ID, "plant", target.ID).Count(&favCount)
	if favCount != 1 {
		t.Errorf("expected exactly 1 favorite on target, got %d", favCount)
	}
	var hiddenCount int64
	db.Model(&model.PlantSpecies{}).Where("merged_into_id = ?", target.ID).Count(&hiddenCount)
	if hiddenCount != 2 {
		t.Errorf("expected 2 hidden sources, got %d", hiddenCount)
	}
	var successLogs int64
	db.Model(&model.PlantMergeLog{}).Where("status = ?", model.PlantMergeStatusSuccess).Count(&successLogs)
	if successLogs != 2 {
		t.Errorf("expected 2 success logs (one per source), got %d", successLogs)
	}
}
