package service

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/gbplantwiki/gbplantwiki/internal/repository"
	"github.com/gbplantwiki/gbplantwiki/internal/util"
)

func newMergeService(t *testing.T) (*PlantMergeService, sqlmock.Sqlmock) {
	t.Helper()
	db, mock := newServiceDB(t)
	svc := NewPlantMergeService(
		db,
		repository.NewPlantSpeciesRepository(db),
		repository.NewUserGardenRepository(db),
		repository.NewFavoriteRepository(db),
		repository.NewCareReminderRepository(db),
		repository.NewDiseasePestRepository(db),
		repository.NewUserRepository(db),
		repository.NewPlantMergeRepository(db),
		newTestLogger(),
	)
	return svc, mock
}

// expectMergePlanQueries queues the read queries loadMergePlan performs, in order.
// Plant keep id=1 (月季) and source id=2 (月月红); user 10 gardens/favorites both
// plants (duplicates to drop), user 11 gardens only the source, user 12 only
// favorites the source.
func expectMergePlanQueries(mock sqlmock.Sqlmock) {
	plantCols := []string{"id", "name", "merged_into_id"}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `plant_species` WHERE `plant_species`.`id`")).
		WillReturnRows(sqlmock.NewRows(plantCols).AddRow(1, "月季", 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `plant_species` WHERE `plant_species`.`id`")).
		WillReturnRows(sqlmock.NewRows(plantCols).AddRow(2, "月月红", 0))

	gardenCols := []string{"id", "user_id", "plant_species_id", "nickname", "owned_since", "location", "care_reminder_id", "created_at"}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_gardens` WHERE plant_species_id = ? ORDER BY id ASC")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows(gardenCols).
			AddRow(100, 10, 2, "阳台月季", time.Now(), "阳台", 55, time.Now()).
			AddRow(102, 11, 2, "小月", time.Now(), "客厅", 0, time.Now()))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_gardens` WHERE plant_species_id = ? ORDER BY id ASC")).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows(gardenCols).
			AddRow(101, 10, 1, "大月季", time.Now(), "院子", 0, time.Now()))

	favCols := []string{"id", "user_id", "target_type", "target_id", "created_at"}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `favorites` WHERE target_type = ? AND target_id = ? ORDER BY id ASC")).
		WithArgs("plant", 2).
		WillReturnRows(sqlmock.NewRows(favCols).
			AddRow(200, 10, "plant", 2, time.Now()).
			AddRow(202, 12, "plant", 2, time.Now()))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `favorites` WHERE target_type = ? AND target_id = ? ORDER BY id ASC")).
		WithArgs("plant", 1).
		WillReturnRows(sqlmock.NewRows(favCols).AddRow(201, 10, "plant", 1, time.Now()))

	remCols := []string{"id", "user_id", "plant_species_id", "task_title", "remind_date", "frequency", "status", "created_at"}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `care_reminders` WHERE plant_species_id = ? ORDER BY id ASC")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows(remCols).
			AddRow(300, 10, 2, "给月月红浇水", time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), "weekly", "pending", time.Now()))

	pestCols := []string{"id", "plant_species_id", "name"}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `disease_pests` WHERE plant_species_id = ? ORDER BY id ASC")).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows(pestCols).AddRow(400, 2, "月季黑斑病"))

	userCols := []string{"id", "username", "nickname"}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users` WHERE id IN")).
		WillReturnRows(sqlmock.NewRows(userCols).
			AddRow(10, "gardener", "绿手指").
			AddRow(11, "rose_fan", "玫瑰粉").
			AddRow(12, "lily", "百合"))
}

func TestPlantMergePreview(t *testing.T) {
	svc, mock := newMergeService(t)
	expectMergePlanQueries(mock)

	preview, err := svc.Preview(1, 2)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.KeepPlantName != "月季" || preview.SourcePlantName != "月月红" {
		t.Errorf("unexpected plant names: keep=%s source=%s", preview.KeepPlantName, preview.SourcePlantName)
	}
	if len(preview.AffectedUsers) != 3 {
		t.Errorf("expected 3 affected users, got %d", len(preview.AffectedUsers))
	}
	if len(preview.Gardens) != 2 || preview.Gardens[0].Action != "remove_duplicate" || preview.Gardens[1].Action != "move" {
		t.Errorf("unexpected garden impacts: %+v", preview.Gardens)
	}
	dup := preview.Gardens[0]
	if dup.Nickname != "阳台月季" || dup.Location != "阳台" || dup.CareReminderID != 55 {
		t.Errorf("duplicate garden snapshot lost nickname/location/reminder: %+v", dup)
	}
	if len(preview.Favorites) != 2 || preview.Favorites[0].Action != "remove_duplicate" || preview.Favorites[1].Action != "move" {
		t.Errorf("unexpected favorite impacts: %+v", preview.Favorites)
	}
	if len(preview.Reminders) != 1 || preview.Reminders[0].TaskTitle != "给月月红浇水" {
		t.Errorf("unexpected reminder impacts: %+v", preview.Reminders)
	}
	if len(preview.Pests) != 1 || preview.Pests[0].Name != "月季黑斑病" {
		t.Errorf("unexpected pest impacts: %+v", preview.Pests)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPlantMergePreviewSamePlant(t *testing.T) {
	svc, mock := newMergeService(t)
	if _, err := svc.Preview(1, 1); err == nil {
		t.Fatal("expected validation error when keep equals source")
	} else {
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.HTTPStatus != 422 {
			t.Errorf("expected 422 AppError, got %v", err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPlantMergePreviewSourceAlreadyMerged(t *testing.T) {
	svc, mock := newMergeService(t)
	plantCols := []string{"id", "name", "merged_into_id"}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `plant_species` WHERE `plant_species`.`id`")).
		WillReturnRows(sqlmock.NewRows(plantCols).AddRow(1, "月季", 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `plant_species` WHERE `plant_species`.`id`")).
		WillReturnRows(sqlmock.NewRows(plantCols).AddRow(2, "月月红", 5))

	if _, err := svc.Preview(1, 2); err == nil {
		t.Fatal("expected conflict error for already merged source")
	} else {
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.HTTPStatus != 409 {
			t.Errorf("expected 409 AppError, got %v", err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPlantMergeExecuteSuccess(t *testing.T) {
	svc, mock := newMergeService(t)

	// Pre-check plan on the outer connection.
	expectMergePlanQueries(mock)

	// The merge transaction: fresh reads, then all writes, atomically.
	mock.ExpectBegin()
	expectMergePlanQueries(mock)
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `user_gardens` WHERE `user_gardens`.`id` = ?")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_gardens` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `favorites` WHERE `favorites`.`id` = ?")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `favorites` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `care_reminders` SET `plant_species_id`=? WHERE plant_species_id = ?")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `disease_pests` SET `plant_species_id`=? WHERE plant_species_id = ?")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `plant_species` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `plant_merge_records`")).
		WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectCommit()

	rec, err := svc.Execute(1, 1, 2)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if rec.Status != "success" || rec.KeepPlantName != "月季" || rec.SourcePlantName != "月月红" {
		t.Errorf("unexpected record: %+v", rec)
	}
	if !strings.Contains(rec.Detail, "阳台月季") || !strings.Contains(rec.Detail, "remove_duplicate") {
		t.Errorf("merge detail lost garden snapshot: %s", rec.Detail)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPlantMergeExecuteRollbackOnError(t *testing.T) {
	svc, mock := newMergeService(t)

	expectMergePlanQueries(mock)
	mock.ExpectBegin()
	expectMergePlanQueries(mock)
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `user_gardens` WHERE `user_gardens`.`id` = ?")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_gardens` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `favorites` WHERE `favorites`.`id` = ?")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `favorites` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// The reminder move blows up mid-processing: everything must roll back.
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `care_reminders` SET `plant_species_id`=? WHERE plant_species_id = ?")).
		WillReturnError(errors.New("Deadlock found when trying to get lock"))
	mock.ExpectRollback()

	// The failed merge record is written outside the rolled-back transaction.
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `plant_merge_records`")).
		WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectCommit()

	_, err := svc.Execute(1, 1, 2)
	if err == nil {
		t.Fatal("expected error when the merge fails mid-processing")
	}
	var appErr *util.AppError
	if !errors.As(err, &appErr) || appErr.HTTPStatus != 500 {
		t.Errorf("expected 500 AppError, got %v", err)
	}
	if !strings.Contains(appErr.Message, "rolled back") {
		t.Errorf("error should state records stayed pre-merge, got: %s", appErr.Message)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
