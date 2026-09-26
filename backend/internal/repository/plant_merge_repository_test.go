package repository

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/gbplantwiki/gbplantwiki/internal/model"
)

func TestMergeAliasAppendsWithoutDuplicates(t *testing.T) {
	got := mergeAlias("蓬莱蕉、龟背芋", "龟背竹", "蓬莱蕉，龟背蕉", "Monstera")
	want := "蓬莱蕉、龟背芋、龟背竹、龟背蕉、Monstera"
	if got != want {
		t.Errorf("mergeAlias = %q, want %q", got, want)
	}
}

func TestMergeAliasEmptyExisting(t *testing.T) {
	if got := mergeAlias("", "月月红", "月季花"); got != "月月红、月季花" {
		t.Errorf("mergeAlias = %q", got)
	}
}

// TestMergeExecuteRollbackOnStuckSource verifies that when a source plant is
// already merged away, the transaction is rolled back and MergeStuckError
// identifies the blocked source plant.
func TestMergeExecuteRollbackOnStuckSource(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewPlantMergeRepository(db)

	mock.ExpectBegin()
	// SELECT ... FOR UPDATE on target
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `plant_species` WHERE `plant_species`.`id` = ? ORDER BY `plant_species`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(uint(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "merged_into_id"}).
			AddRow(1, "龟背竹", 0))
	// SELECT ... FOR UPDATE on sources
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `plant_species` WHERE id IN (?) FOR UPDATE")).
		WithArgs(uint(2)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "merged_into_id"}).
			AddRow(2, "蓬莱蕉", 1))
	mock.ExpectRollback()

	_, err := repo.Execute(10, 1, []uint{2})
	if err == nil {
		t.Fatal("expected MergeStuckError, got nil")
	}
	stuck, ok := err.(*MergeStuckError)
	if !ok {
		t.Fatalf("expected *MergeStuckError, got %T: %v", err, err)
	}
	if stuck.SourcePlantID != 2 || stuck.Step != "validate" {
		t.Errorf("unexpected stuck marker: %+v", stuck)
	}
}

// TestMergeExecuteTargetMissing verifies a missing kept plant also rolls back.
func TestMergeExecuteTargetMissing(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewPlantMergeRepository(db)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `plant_species` WHERE `plant_species`.`id` = ? ORDER BY `plant_species`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(uint(99), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	_, err := repo.Execute(10, 99, []uint{2})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	stuck, ok := err.(*MergeStuckError)
	if !ok {
		t.Fatalf("expected *MergeStuckError, got %T", err)
	}
	if stuck.Step != "load_target" {
		t.Errorf("unexpected step: %s", stuck.Step)
	}
}

// TestRecordFailureWritesFailedLogs checks failed logs are persisted (outside
// the rolled-back transaction) and the stuck source carries the reason.
func TestRecordFailureWritesFailedLogs(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewPlantMergeRepository(db)

	// enrichment lookup of the source names
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `plant_species` WHERE id IN (?,?)")).
		WithArgs(uint(2), uint(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "alias", "merged_into_id"}).
			AddRow(2, "蓬莱蕉", "龟背芋", 0).
			AddRow(3, "电线草", "", 0))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `plant_merge_logs`")).
		WillReturnResult(sqlmock.NewResult(1, 2))
	mock.ExpectCommit()

	stuck := &MergeStuckError{SourcePlantID: 3, SourceName: "电线草", Step: "garden_relink", Reason: "boom"}
	if err := repo.RecordFailure(10, 1, "龟背竹", []uint{2, 3}, stuck); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

// TestPlantMergeLogConstants guards the model statuses used by service/UI.
func TestPlantMergeLogConstants(t *testing.T) {
	if model.PlantMergeStatusSuccess != "success" || model.PlantMergeStatusFailed != "failed" {
		t.Fatal("merge log status constants changed")
	}
}
