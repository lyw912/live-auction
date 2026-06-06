package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/storage"
)

type liveOpsTestPayload struct {
	ID         string `json:"id"`
	RoomID     string `json:"room_id"`
	Status     string `json:"status"`
	Title      string `json:"title"`
	Progress   int    `json:"progress"`
	MyTeam     string `json:"my_team"`
	Disclaimer string `json:"disclaimer"`
	LuckyDraw  struct {
		Status             string `json:"status"`
		Participants       int    `json:"participants"`
		MyEntryStatus      string `json:"my_entry_status"`
		MyRewardKey        string `json:"my_reward_key"`
		MyRewardLabel      string `json:"my_reward_label"`
		EligibleTaskCount  int    `json:"eligible_task_count"`
		CompletedTaskCount int    `json:"completed_task_count"`
		CanEnter           bool   `json:"can_enter"`
	} `json:"lucky_draw"`
	TeamScores []struct {
		Key   string `json:"key"`
		Count int    `json:"count"`
	} `json:"team_scores"`
	Tasks []struct {
		Key         string `json:"key"`
		Label       string `json:"label"`
		CompletedAt string `json:"completed_at"`
	} `json:"tasks"`
}

func TestLiveOpsCampaignRoutesPersistTaskProgress(t *testing.T) {
	db := openMonitorDB(t)
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6380"})
	t.Cleanup(func() { _ = rdb.Close() })

	roomID := "room_liveops_" + uuid.NewString()
	if _, err := db.Exec(context.Background(), `INSERT INTO rooms (id, host_id, status) VALUES ($1, 'host_1', 'OPEN')`, roomID); err != nil {
		t.Fatalf("insert room: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		INSERT INTO room_memberships (room_id, user_id, role, status)
		VALUES ($1, 'user_1', 'viewer', 'ACTIVE')
	`, roomID); err != nil {
		t.Fatalf("insert membership: %v", err)
	}

	router := NewRouter(testConfig(), &storage.Dependencies{Postgres: db, Redis: rdb}, slog.Default())

	payload := requestLiveOps(t, router, http.MethodGet, "/api/rooms/"+roomID+"/liveops", userHeaders("user_1", "user"), http.StatusOK)
	if payload.RoomID != roomID || payload.Status != "ACTIVE" || len(payload.Tasks) != 4 || payload.Progress != 0 {
		t.Fatalf("unexpected initial liveops payload: %#v", payload)
	}
	if payload.Disclaimer == "" {
		t.Fatalf("liveops disclaimer is required")
	}

	payload = requestLiveOps(t, router, http.MethodPost, "/api/rooms/"+roomID+"/liveops/tasks/watch", userHeaders("user_1", "user"), http.StatusOK)
	if payload.Progress != 1 || !taskCompleted(payload, "watch") {
		t.Fatalf("watch task was not completed: %#v", payload)
	}

	payload = requestLiveOps(t, router, http.MethodPost, "/api/rooms/"+roomID+"/liveops/tasks/watch", userHeaders("user_1", "user"), http.StatusOK)
	if payload.Progress != 1 || !taskCompleted(payload, "watch") {
		t.Fatalf("duplicate watch task should remain idempotent: %#v", payload)
	}

	assertAPIStatus(t, router, http.MethodPost, "/api/rooms/"+roomID+"/liveops/tasks/bad", nil, userHeaders("user_1", "user"), http.StatusBadRequest)
	assertAPIStatus(t, router, http.MethodPost, "/api/rooms/"+roomID+"/liveops/lucky-draw/enter", nil, userHeaders("user_1", "user"), http.StatusBadRequest)
	for _, task := range []string{"follow", "ask", "leaderboard"} {
		payload = requestLiveOps(t, router, http.MethodPost, "/api/rooms/"+roomID+"/liveops/tasks/"+task, userHeaders("user_1", "user"), http.StatusOK)
	}
	if !payload.LuckyDraw.CanEnter || payload.LuckyDraw.CompletedTaskCount != 4 {
		t.Fatalf("lucky draw should be enterable after tasks: %#v", payload.LuckyDraw)
	}
	payload = requestLiveOps(t, router, http.MethodPost, "/api/rooms/"+roomID+"/liveops/lucky-draw/enter", userHeaders("user_1", "user"), http.StatusOK)
	if payload.LuckyDraw.MyEntryStatus != "ENTERED" || payload.LuckyDraw.Participants != 1 {
		t.Fatalf("lucky draw entry not persisted: %#v", payload.LuckyDraw)
	}
	payload = requestLiveOps(t, router, http.MethodPost, "/api/rooms/"+roomID+"/liveops/lucky-draw/open", userHeaders("user_1", "user"), http.StatusOK)
	if payload.LuckyDraw.MyEntryStatus != "OPENED" || payload.LuckyDraw.MyRewardKey == "" || payload.LuckyDraw.MyRewardLabel == "" {
		t.Fatalf("lucky draw reward not opened: %#v", payload.LuckyDraw)
	}
	payload = requestLiveOpsWithBody(t, router, http.MethodPost, "/api/rooms/"+roomID+"/liveops/team", bytes.NewBufferString(`{"team_key":"story"}`), userHeaders("user_1", "user"), http.StatusOK)
	if payload.MyTeam != "story" || teamCount(payload, "story") != 1 {
		t.Fatalf("story team choice was not persisted: %#v", payload)
	}
	payload = requestLiveOpsWithBody(t, router, http.MethodPost, "/api/rooms/"+roomID+"/liveops/team", bytes.NewBufferString(`{"team_key":"craft"}`), userHeaders("user_1", "user"), http.StatusOK)
	if payload.MyTeam != "craft" || teamCount(payload, "craft") != 1 || teamCount(payload, "story") != 0 {
		t.Fatalf("craft team choice did not replace previous choice: %#v", payload)
	}
	assertAPIStatus(t, router, http.MethodPost, "/api/rooms/"+roomID+"/liveops/team", bytes.NewBufferString(`{"team_key":"bad"}`), userHeaders("user_1", "user"), http.StatusBadRequest)
	assertAPIStatus(t, router, http.MethodGet, "/api/rooms/"+roomID+"/liveops", nil, userHeaders("user_2", "user"), http.StatusForbidden)
}

func requestLiveOps(t *testing.T, router http.Handler, method string, path string, headers http.Header, want int) liveOpsTestPayload {
	return requestLiveOpsWithBody(t, router, method, path, nil, headers, want)
}

func requestLiveOpsWithBody(t *testing.T, router http.Handler, method string, path string, body *bytes.Buffer, headers http.Header, want int) liveOpsTestPayload {
	t.Helper()
	if body == nil {
		body = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, body)
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("%s %s status = %d, want %d body=%s", method, path, rec.Code, want, rec.Body.String())
	}
	var payload liveOpsTestPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode liveops payload: %v body=%s", err, rec.Body.String())
	}
	return payload
}

func taskCompleted(payload liveOpsTestPayload, key string) bool {
	for _, task := range payload.Tasks {
		if task.Key == key {
			return task.CompletedAt != ""
		}
	}
	return false
}

func teamCount(payload liveOpsTestPayload, key string) int {
	for _, team := range payload.TeamScores {
		if team.Key == key {
			return team.Count
		}
	}
	return 0
}
