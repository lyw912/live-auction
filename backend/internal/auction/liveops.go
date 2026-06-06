package auction

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	apierrors "live-auction/backend/internal/platform/errors"
)

type LiveOpsTask struct {
	Key         string     `json:"key"`
	Label       string     `json:"label"`
	Description string     `json:"description"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type LiveOpsCampaign struct {
	ID          string        `json:"id"`
	RoomID      string        `json:"room_id"`
	Status      string        `json:"status"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Tasks       []LiveOpsTask `json:"tasks"`
	Progress    int           `json:"progress"`
	MyTeam      string        `json:"my_team,omitempty"`
	TeamScores  []LiveOpsTeam `json:"team_scores"`
	Disclaimer  string        `json:"disclaimer"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

type LiveOpsTeam struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

var liveOpsTaskCatalog = []LiveOpsTask{
	{Key: "watch", Label: "看拍品", Description: "查看当前拍品和规则"},
	{Key: "follow", Label: "关注", Description: "关注直播间，方便回到本场"},
	{Key: "ask", Label: "问拍品", Description: "基于已展示信息提一个问题"},
	{Key: "leaderboard", Label: "看榜单", Description: "查看实时榜单和下一口价格"},
}

func (r *Repository) GetLiveOpsCampaign(ctx context.Context, roomID string, userID string) (LiveOpsCampaign, error) {
	if roomID == "" || userID == "" {
		return LiveOpsCampaign{}, apierrors.New(apierrors.CodeInvalidArgument, "room_id and user_id are required", http.StatusBadRequest)
	}
	campaign, err := r.ensureLiveOpsCampaign(ctx, roomID)
	if err != nil {
		return LiveOpsCampaign{}, err
	}
	progress, err := r.liveOpsProgress(ctx, campaign.ID, userID)
	if err != nil {
		return LiveOpsCampaign{}, err
	}
	tasks := make([]LiveOpsTask, 0, len(liveOpsTaskCatalog))
	for _, task := range liveOpsTaskCatalog {
		copy := task
		if completedAt, ok := progress[task.Key]; ok {
			copy.CompletedAt = &completedAt
		}
		tasks = append(tasks, copy)
	}
	campaign.Tasks = tasks
	campaign.Progress = len(progress)
	if err := r.attachLiveOpsTeams(ctx, &campaign, userID); err != nil {
		return LiveOpsCampaign{}, err
	}
	return campaign, nil
}

func (r *Repository) CompleteLiveOpsTask(ctx context.Context, roomID string, userID string, taskKey string) (LiveOpsCampaign, error) {
	if roomID == "" || userID == "" {
		return LiveOpsCampaign{}, apierrors.New(apierrors.CodeInvalidArgument, "room_id and user_id are required", http.StatusBadRequest)
	}
	if !validLiveOpsTask(taskKey) {
		return LiveOpsCampaign{}, apierrors.New(apierrors.CodeInvalidArgument, "unknown liveops task", http.StatusBadRequest)
	}
	campaign, err := r.ensureLiveOpsCampaign(ctx, roomID)
	if err != nil {
		return LiveOpsCampaign{}, err
	}
	if _, err := r.db.Exec(ctx, `
		INSERT INTO liveops_task_progress (campaign_id, room_id, user_id, task_key, payload_json)
		VALUES ($1, $2, $3, $4, jsonb_build_object('source', 'h5'))
		ON CONFLICT (campaign_id, user_id, task_key)
		DO UPDATE SET completed_at = liveops_task_progress.completed_at
	`, campaign.ID, roomID, userID, taskKey); err != nil {
		return LiveOpsCampaign{}, err
	}
	return r.GetLiveOpsCampaign(ctx, roomID, userID)
}

func (r *Repository) SelectLiveOpsTeam(ctx context.Context, roomID string, userID string, teamKey string) (LiveOpsCampaign, error) {
	if roomID == "" || userID == "" {
		return LiveOpsCampaign{}, apierrors.New(apierrors.CodeInvalidArgument, "room_id and user_id are required", http.StatusBadRequest)
	}
	if !validLiveOpsTeam(teamKey) {
		return LiveOpsCampaign{}, apierrors.New(apierrors.CodeInvalidArgument, "unknown liveops team", http.StatusBadRequest)
	}
	campaign, err := r.ensureLiveOpsCampaign(ctx, roomID)
	if err != nil {
		return LiveOpsCampaign{}, err
	}
	if _, err := r.db.Exec(ctx, `
		INSERT INTO liveops_team_choices (campaign_id, room_id, user_id, team_key)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (campaign_id, user_id)
		DO UPDATE SET team_key = EXCLUDED.team_key, updated_at = now()
	`, campaign.ID, roomID, userID, teamKey); err != nil {
		return LiveOpsCampaign{}, err
	}
	return r.GetLiveOpsCampaign(ctx, roomID, userID)
}

func (r *Repository) ensureLiveOpsCampaign(ctx context.Context, roomID string) (LiveOpsCampaign, error) {
	var campaign LiveOpsCampaign
	err := r.db.QueryRow(ctx, `
		SELECT id, room_id, status, title, description, updated_at
		FROM liveops_campaigns
		WHERE room_id = $1 AND status = 'ACTIVE'
		LIMIT 1
	`, roomID).Scan(&campaign.ID, &campaign.RoomID, &campaign.Status, &campaign.Title, &campaign.Description, &campaign.UpdatedAt)
	if err == nil {
		campaign.Disclaimer = liveOpsDisclaimer
		return campaign, nil
	}
	if err != pgx.ErrNoRows {
		return LiveOpsCampaign{}, err
	}
	id := "loc_" + uuid.NewString()
	err = r.db.QueryRow(ctx, `
		INSERT INTO liveops_campaigns (id, room_id, status, title, description, rules_json)
		VALUES ($1, $2, 'ACTIVE', '开拍前准备', '完成信息查看、关注、问答和榜单确认，帮助自己理解本场规则。',
		        jsonb_build_object('no_lottery', true, 'no_reward', true, 'no_price_or_winner_impact', true))
		ON CONFLICT (room_id) WHERE status = 'ACTIVE'
		DO UPDATE SET updated_at = liveops_campaigns.updated_at
		RETURNING id, room_id, status, title, description, updated_at
	`, id, roomID).Scan(&campaign.ID, &campaign.RoomID, &campaign.Status, &campaign.Title, &campaign.Description, &campaign.UpdatedAt)
	campaign.Disclaimer = liveOpsDisclaimer
	return campaign, err
}

func (r *Repository) liveOpsProgress(ctx context.Context, campaignID string, userID string) (map[string]time.Time, error) {
	rows, err := r.db.Query(ctx, `
		SELECT task_key, completed_at
		FROM liveops_task_progress
		WHERE campaign_id = $1 AND user_id = $2
	`, campaignID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var key string
		var completedAt time.Time
		if err := rows.Scan(&key, &completedAt); err != nil {
			return nil, err
		}
		out[key] = completedAt
	}
	return out, rows.Err()
}

func validLiveOpsTask(key string) bool {
	for _, task := range liveOpsTaskCatalog {
		if task.Key == key {
			return true
		}
	}
	return false
}

func (r *Repository) attachLiveOpsTeams(ctx context.Context, campaign *LiveOpsCampaign, userID string) error {
	rows, err := r.db.Query(ctx, `
		SELECT team_key, count(*)
		FROM liveops_team_choices
		WHERE campaign_id = $1
		GROUP BY team_key
	`, campaign.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	counts := map[string]int{"craft": 0, "story": 0}
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			return err
		}
		counts[key] = count
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := r.db.QueryRow(ctx, `
		SELECT team_key
		FROM liveops_team_choices
		WHERE campaign_id = $1 AND user_id = $2
	`, campaign.ID, userID).Scan(&campaign.MyTeam); err != nil && err != pgx.ErrNoRows {
		return err
	}
	campaign.TeamScores = []LiveOpsTeam{
		{Key: "craft", Label: "工艺派", Count: counts["craft"]},
		{Key: "story", Label: "故事派", Count: counts["story"]},
	}
	return nil
}

func validLiveOpsTeam(key string) bool {
	return key == "craft" || key == "story"
}

const liveOpsDisclaimer = "不含抽奖、现金奖励或中标优先权；不会影响价格、排名、成交或保证金。"
