//go:build integration_sqlite

// Full HTTP-level integration for the admin merge console.
//
//	go test -tags=integration_sqlite -run TestHTTPMerge ./internal/router/
package router

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/gbplantwiki/gbplantwiki/internal/config"
	"github.com/gbplantwiki/gbplantwiki/internal/model"
	"github.com/gbplantwiki/gbplantwiki/internal/util"
)

type apiEnvelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"message"`
	Data json.RawMessage `json:"data"`
}

func newMergeServer(t *testing.T) (*httptest.Server, *gorm.DB, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.PlantSpecies{}, &model.PlantMergeLog{}, &model.CareArticle{},
		&model.DiseasePest{}, &model.CareReminder{}, &model.Favorite{}, &model.UserGarden{},
		&model.Question{}, &model.Answer{},
	); err != nil {
		t.Fatal(err)
	}
	admin := &model.User{Username: "root", Email: "root@x", PasswordHash: "x", Nickname: "管理员", Role: "admin"}
	user := &model.User{Username: "fan", Email: "fan@x", PasswordHash: "x", Nickname: "花友", Role: "user"}
	db.Create(admin)
	db.Create(user)
	keep := model.PlantSpecies{Name: "龟背竹", Alias: "", Type: "foliage", ImageURLs: "[]"}
	dup := model.PlantSpecies{Name: "电线草", Alias: "龟背芋", Type: "foliage", ImageURLs: "[]"}
	db.Create(&keep)
	db.Create(&dup)
	db.Create(&model.UserGarden{UserID: user.ID, PlantSpeciesID: keep.ID, Nickname: "保留的", Location: "客厅"})
	db.Create(&model.UserGarden{UserID: user.ID, PlantSpeciesID: dup.ID, Nickname: "旧昵称", Location: "阳台", CareReminderID: 5})
	db.Create(&model.Favorite{UserID: user.ID, TargetType: "plant", TargetID: dup.ID})
	db.Create(&model.DiseasePest{PlantSpeciesID: dup.ID, Name: "叶斑"})
	db.Create(&model.CareReminder{UserID: user.ID, PlantSpeciesID: dup.ID, TaskTitle: "擦叶", Status: "pending"})

	cfg := &config.Config{JWTSecret: "test-secret", JWTExpire: time.Hour, RateLimitReq: 1000, RateLimitWin: time.Minute}
	r := Setup(cfg, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	token, err := util.GenerateToken(admin.ID, admin.Username, admin.Role, cfg.JWTSecret, cfg.JWTExpire)
	if err != nil {
		t.Fatal(err)
	}
	return ts, db, token
}

func doJSON(t *testing.T, method, url, token string, body interface{}) (int, apiEnvelope) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, url, rdr)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var env apiEnvelope
	_ = json.Unmarshal(raw, &env)
	return res.StatusCode, env
}

func TestHTTPMergePreviewThenExecute(t *testing.T) {
	ts, db, token := newMergeServer(t)

	// Preview.
	status, env := doJSON(t, http.MethodPost, ts.URL+"/api/v1/plants/merges/preview", token,
		map[string]interface{}{"target_plant_id": 1, "source_plant_ids": []uint{2}})
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("preview status=%d code=%d msg=%s", status, env.Code, env.Msg)
	}
	var preview map[string]interface{}
	json.Unmarshal(env.Data, &preview)
	summary := preview["summary"].(map[string]interface{})
	if summary["garden_rows"].(float64) != 1 || summary["garden_collisions"].(float64) != 1 ||
		summary["favorite_rows"].(float64) != 1 || summary["pest_rows"].(float64) != 1 ||
		summary["reminder_rows"].(float64) != 1 || summary["affected_users"].(float64) != 1 {
		t.Errorf("unexpected summary: %v", summary)
	}
	users := preview["affected_users"].([]interface{})
	if len(users) != 1 {
		t.Errorf("affected users = %d", len(users))
	}

	// Execute.
	status, env = doJSON(t, http.MethodPost, ts.URL+"/api/v1/plants/merges", token,
		map[string]interface{}{"target_plant_id": 1, "source_plant_ids": []uint{2}})
	if status != http.StatusOK || env.Code != 0 {
		t.Fatalf("execute status=%d code=%d msg=%s", status, env.Code, env.Msg)
	}
	var result map[string]interface{}
	json.Unmarshal(env.Data, &result)
	if result["garden_deduped"].(float64) != 1 || result["favorites_moved"].(float64) != 1 ||
		result["pests_moved"].(float64) != 1 || result["reminders_moved"].(float64) != 1 {
		t.Errorf("unexpected result: %v", result)
	}

	// Public plant list hides the duplicate (only 1 row, 龟背竹).
	status, env = doJSON(t, http.MethodGet, ts.URL+"/api/v1/plants", "", nil)
	if status != 200 {
		t.Fatalf("list status %d", status)
	}
	var page struct {
		List  []model.PlantSpecies `json:"list"`
		Total int64                `json:"total"`
	}
	json.Unmarshal(env.Data, &page)
	if page.Total != 1 || page.List[0].ID != 1 {
		t.Errorf("public list after merge = %+v", page.List)
	}

	// Old plant detail still reachable and points at keeper.
	status, env = doJSON(t, http.MethodGet, ts.URL+"/api/v1/plants/2", "", nil)
	if status != 200 {
		t.Fatalf("old plant detail status %d", status)
	}
	var old model.PlantSpecies
	json.Unmarshal(env.Data, &old)
	if old.MergedIntoID != 1 {
		t.Errorf("old plant merged_into_id = %d", old.MergedIntoID)
	}

	// Merge logs queryable.
	status, env = doJSON(t, http.MethodGet, ts.URL+"/api/v1/plants/merges", token, nil)
	if status != 200 {
		t.Fatalf("logs status %d", status)
	}
	var logPage struct {
		List  []model.PlantMergeLog `json:"list"`
		Total int64                 `json:"total"`
	}
	json.Unmarshal(env.Data, &logPage)
	if logPage.Total != 1 || logPage.List[0].Status != model.PlantMergeStatusSuccess {
		t.Errorf("logs = %+v", logPage)
	}

	// Collapsed garden: user 1 has exactly one row on keep; reminder carried over.
	var gardens []model.UserGarden
	db.Where("user_id = ?", 2).Find(&gardens)
	if len(gardens) != 1 || gardens[0].PlantSpeciesID != 1 || gardens[0].CareReminderID != 5 {
		t.Errorf("garden after merge = %+v", gardens)
	}
	if gardens[0].Nickname != "保留的" {
		t.Errorf("kept nickname should survive, got %q", gardens[0].Nickname)
	}

	// Non-admin cannot access the merge console.
	normalToken, _ := util.GenerateToken(2, "fan", "user", "test-secret", time.Hour)
	status, _ = doJSON(t, http.MethodPost, ts.URL+"/api/v1/plants/merges/preview", normalToken,
		map[string]interface{}{"target_plant_id": 1, "source_plant_ids": []uint{2}})
	if status != http.StatusForbidden {
		t.Errorf("non-admin preview status = %d, want 403", status)
	}
}

func TestHTTPMergeRejectsSelfMerge(t *testing.T) {
	ts, _, token := newMergeServer(t)
	status, env := doJSON(t, http.MethodPost, ts.URL+"/api/v1/plants/merges/preview", token,
		map[string]interface{}{"target_plant_id": 1, "source_plant_ids": []uint{1}})
	if status == http.StatusOK {
		t.Errorf("self-merge should fail, got 200: %s", env.Msg)
	}
}
