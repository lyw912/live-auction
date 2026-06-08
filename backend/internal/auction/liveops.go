package auction

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"net/http"
	"strconv"
	"strings"
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
	ID          string           `json:"id"`
	RoomID      string           `json:"room_id"`
	Status      string           `json:"status"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Tasks       []LiveOpsTask    `json:"tasks"`
	Progress    int              `json:"progress"`
	MyTeam      string           `json:"my_team,omitempty"`
	TeamScores  []LiveOpsTeam    `json:"team_scores"`
	LuckyDraw   LiveOpsLuckyDraw `json:"lucky_draw"`
	Disclaimer  string           `json:"disclaimer"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

type LiveOpsRewardConfig struct {
	Enabled           bool   `json:"enabled"`
	Title             string `json:"title"`
	Description       string `json:"description"`
	RewardName        string `json:"reward_name"`
	RewardQuota       int    `json:"reward_quota"`
	RequiredTaskCount int    `json:"required_task_count"`
}

type LiveOpsHostSummary struct {
	Campaign          LiveOpsCampaign      `json:"campaign"`
	RewardConfig     LiveOpsRewardConfig  `json:"reward_config"`
	ParticipantCount  int                  `json:"participant_count"`
	QualifiedCount    int                  `json:"qualified_count"`
	OpenedCount       int                  `json:"opened_count"`
	PreferenceSummary []LiveOpsTeam        `json:"preference_summary"`
	RecentRewards     []LiveOpsRewardEntry `json:"recent_rewards"`
}

type LiveOpsRewardEntry struct {
	UserID      string     `json:"user_id"`
	UserMasked  string     `json:"user_masked"`
	Status      string     `json:"status"`
	RewardLabel string     `json:"reward_label,omitempty"`
	EnteredAt   time.Time  `json:"entered_at"`
	OpenedAt    *time.Time `json:"opened_at,omitempty"`
}

type LiveOpsTeam struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

type LiveOpsLuckyDraw struct {
	Status             string     `json:"status"`
	Title              string     `json:"title"`
	Description        string     `json:"description"`
	OpensAt            time.Time  `json:"opens_at"`
	ServerTime         time.Time  `json:"server_time"`
	Participants       int        `json:"participants"`
	MyEntryStatus      string     `json:"my_entry_status,omitempty"`
	MyRewardKey        string     `json:"my_reward_key,omitempty"`
	MyRewardLabel      string     `json:"my_reward_label,omitempty"`
	EligibleTaskCount  int        `json:"eligible_task_count"`
	CompletedTaskCount int        `json:"completed_task_count"`
	CanEnter           bool       `json:"can_enter"`
	OpenedAt           *time.Time `json:"opened_at,omitempty"`
}

type LiveOpsCampaignRuleConfig struct {
	Reward LiveOpsRewardConfig `json:"reward"`
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
	if err := r.attachLiveOpsLuckyDraw(ctx, &campaign, userID); err != nil {
		return LiveOpsCampaign{}, err
	}
	return campaign, nil
}

func (r *Repository) GetLiveOpsHostSummary(ctx context.Context, roomID string) (LiveOpsHostSummary, error) {
	if roomID == "" {
		return LiveOpsHostSummary{}, apierrors.New(apierrors.CodeInvalidArgument, "room_id is required", http.StatusBadRequest)
	}
	campaign, err := r.ensureLiveOpsCampaign(ctx, roomID)
	if err != nil {
		return LiveOpsHostSummary{}, err
	}
	if err := r.attachLiveOpsTeams(ctx, &campaign, ""); err != nil {
		return LiveOpsHostSummary{}, err
	}
	if err := r.attachLiveOpsLuckyDraw(ctx, &campaign, ""); err != nil {
		return LiveOpsHostSummary{}, err
	}
	config, err := r.liveOpsRewardConfig(ctx, campaign.ID)
	if err != nil {
		return LiveOpsHostSummary{}, err
	}
	qualifiedCount, err := r.liveOpsQualifiedCount(ctx, campaign.ID)
	if err != nil {
		return LiveOpsHostSummary{}, err
	}
	recentRewards, err := r.liveOpsRecentRewards(ctx, campaign.ID)
	if err != nil {
		return LiveOpsHostSummary{}, err
	}
	return LiveOpsHostSummary{
		Campaign:          campaign,
		RewardConfig:     config,
		ParticipantCount:  campaign.LuckyDraw.Participants,
		QualifiedCount:    qualifiedCount,
		OpenedCount:       liveOpsOpenedRewardCount(recentRewards),
		PreferenceSummary: campaign.TeamScores,
		RecentRewards:     recentRewards,
	}, nil
}

func (r *Repository) UpdateLiveOpsRewardConfig(ctx context.Context, roomID string, input LiveOpsRewardConfig) (LiveOpsHostSummary, error) {
	if roomID == "" {
		return LiveOpsHostSummary{}, apierrors.New(apierrors.CodeInvalidArgument, "room_id is required", http.StatusBadRequest)
	}
	campaign, err := r.ensureLiveOpsCampaign(ctx, roomID)
	if err != nil {
		return LiveOpsHostSummary{}, err
	}
	config := normalizeLiveOpsRewardConfig(input)
	rules := LiveOpsCampaignRuleConfig{Reward: config}
	raw, err := json.Marshal(rules)
	if err != nil {
		return LiveOpsHostSummary{}, err
	}
	if _, err := r.db.Exec(ctx, `
		UPDATE liveops_campaigns
		SET title = $1, description = $2, rules_json = $3::jsonb, updated_at = now()
		WHERE id = $4
	`, config.Title, config.Description, string(raw), campaign.ID); err != nil {
		return LiveOpsHostSummary{}, err
	}
	return r.GetLiveOpsHostSummary(ctx, roomID)
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

func (r *Repository) EnterLiveOpsLuckyDraw(ctx context.Context, roomID string, userID string) (LiveOpsCampaign, error) {
	if roomID == "" || userID == "" {
		return LiveOpsCampaign{}, apierrors.New(apierrors.CodeInvalidArgument, "room_id and user_id are required", http.StatusBadRequest)
	}
	campaign, err := r.GetLiveOpsCampaign(ctx, roomID, userID)
	if err != nil {
		return LiveOpsCampaign{}, err
	}
	if !campaign.LuckyDraw.CanEnter {
		return LiveOpsCampaign{}, apierrors.New(apierrors.CodeInvalidArgument, "complete interaction tasks before entering reward activity", http.StatusBadRequest)
	}
	if _, err := r.db.Exec(ctx, `
		INSERT INTO liveops_lucky_draw_entries (campaign_id, room_id, user_id, entry_status)
		VALUES ($1, $2, $3, 'ENTERED')
		ON CONFLICT (campaign_id, user_id)
		DO UPDATE SET entry_status = liveops_lucky_draw_entries.entry_status
	`, campaign.ID, roomID, userID); err != nil {
		return LiveOpsCampaign{}, err
	}
	return r.GetLiveOpsCampaign(ctx, roomID, userID)
}

func (r *Repository) OpenLiveOpsLuckyDraw(ctx context.Context, roomID string, userID string) (LiveOpsCampaign, error) {
	if roomID == "" || userID == "" {
		return LiveOpsCampaign{}, apierrors.New(apierrors.CodeInvalidArgument, "room_id and user_id are required", http.StatusBadRequest)
	}
	campaign, err := r.GetLiveOpsCampaign(ctx, roomID, userID)
	if err != nil {
		return LiveOpsCampaign{}, err
	}
	if campaign.LuckyDraw.MyEntryStatus == "" {
		return LiveOpsCampaign{}, apierrors.New(apierrors.CodeInvalidArgument, "enter reward activity before opening", http.StatusBadRequest)
	}
	config, err := r.liveOpsRewardConfig(ctx, campaign.ID)
	if err != nil {
		return LiveOpsCampaign{}, err
	}
	rewardKey, rewardLabel := deterministicLuckyDrawReward(campaign.ID, userID, config)
	if _, err := r.db.Exec(ctx, `
		UPDATE liveops_lucky_draw_entries
		SET entry_status = 'OPENED', reward_key = $1, reward_label = $2, opened_at = COALESCE(opened_at, now())
		WHERE campaign_id = $3 AND user_id = $4
	`, rewardKey, rewardLabel, campaign.ID, userID); err != nil {
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
		VALUES ($1, $2, 'ACTIVE', $3, $4, $5::jsonb)
		ON CONFLICT (room_id) WHERE status = 'ACTIVE'
		DO UPDATE SET updated_at = liveops_campaigns.updated_at
		RETURNING id, room_id, status, title, description, updated_at
	`, id, roomID, defaultLiveOpsRewardConfig.Title, defaultLiveOpsRewardConfig.Description, defaultLiveOpsRulesJSON()).Scan(&campaign.ID, &campaign.RoomID, &campaign.Status, &campaign.Title, &campaign.Description, &campaign.UpdatedAt)
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
		{Key: "craft", Label: "看工艺", Count: counts["craft"]},
		{Key: "story", Label: "听故事", Count: counts["story"]},
	}
	return nil
}

func (r *Repository) attachLiveOpsLuckyDraw(ctx context.Context, campaign *LiveOpsCampaign, userID string) error {
	var participants int
	if err := r.db.QueryRow(ctx, `
		SELECT count(*)
		FROM liveops_lucky_draw_entries
		WHERE campaign_id = $1
	`, campaign.ID).Scan(&participants); err != nil {
		return err
	}
	config, err := r.liveOpsRewardConfig(ctx, campaign.ID)
	if err != nil {
		return err
	}
	requiredTaskCount := config.RequiredTaskCount
	if requiredTaskCount <= 0 || requiredTaskCount > len(liveOpsTaskCatalog) {
		requiredTaskCount = len(liveOpsTaskCatalog)
	}
	draw := LiveOpsLuckyDraw{
		Status:             "READY",
		Title:              config.Title,
		Description:        config.Description,
		OpensAt:            campaign.UpdatedAt.Add(90 * time.Second),
		ServerTime:         time.Now().UTC(),
		Participants:       participants,
		EligibleTaskCount:  requiredTaskCount,
		CompletedTaskCount: campaign.Progress,
		CanEnter:           config.Enabled && campaign.Progress >= requiredTaskCount,
	}
	var openedAt *time.Time
	if userID != "" {
		if err := r.db.QueryRow(ctx, `
			SELECT entry_status, COALESCE(reward_key, ''), COALESCE(reward_label, ''), opened_at
			FROM liveops_lucky_draw_entries
			WHERE campaign_id = $1 AND user_id = $2
		`, campaign.ID, userID).Scan(&draw.MyEntryStatus, &draw.MyRewardKey, &draw.MyRewardLabel, &openedAt); err != nil && err != pgx.ErrNoRows {
			return err
		}
	}
	draw.OpenedAt = openedAt
	if draw.MyEntryStatus == "OPENED" {
		draw.Status = "OPENED"
	} else if draw.MyEntryStatus == "ENTERED" {
		draw.Status = "ENTERED"
	}
	campaign.LuckyDraw = draw
	return nil
}

func (r *Repository) liveOpsRewardConfig(ctx context.Context, campaignID string) (LiveOpsRewardConfig, error) {
	var raw []byte
	if err := r.db.QueryRow(ctx, `
		SELECT rules_json
		FROM liveops_campaigns
		WHERE id = $1
	`, campaignID).Scan(&raw); err != nil {
		return LiveOpsRewardConfig{}, err
	}
	var rules LiveOpsCampaignRuleConfig
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &rules)
	}
	return normalizeLiveOpsRewardConfig(rules.Reward), nil
}

func (r *Repository) liveOpsQualifiedCount(ctx context.Context, campaignID string) (int, error) {
	var count int
	config, err := r.liveOpsRewardConfig(ctx, campaignID)
	if err != nil {
		return 0, err
	}
	required := config.RequiredTaskCount
	if required <= 0 || required > len(liveOpsTaskCatalog) {
		required = len(liveOpsTaskCatalog)
	}
	err = r.db.QueryRow(ctx, `
		SELECT count(*)
		FROM (
		  SELECT user_id
		  FROM liveops_task_progress
		  WHERE campaign_id = $1
		  GROUP BY user_id
		  HAVING count(DISTINCT task_key) >= $2
		) q
	`, campaignID, required).Scan(&count)
	return count, err
}

func (r *Repository) liveOpsRecentRewards(ctx context.Context, campaignID string) ([]LiveOpsRewardEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT user_id, entry_status, COALESCE(reward_label, ''), entered_at, opened_at
		FROM liveops_lucky_draw_entries
		WHERE campaign_id = $1
		ORDER BY entered_at DESC
		LIMIT 20
	`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LiveOpsRewardEntry, 0)
	for rows.Next() {
		var row LiveOpsRewardEntry
		if err := rows.Scan(&row.UserID, &row.Status, &row.RewardLabel, &row.EnteredAt, &row.OpenedAt); err != nil {
			return nil, err
		}
		row.UserMasked = maskLiveOpsUser(row.UserID)
		out = append(out, row)
	}
	return out, rows.Err()
}

func validLiveOpsTeam(key string) bool {
	return key == "craft" || key == "story"
}

func deterministicLuckyDrawReward(campaignID string, userID string, config LiveOpsRewardConfig) (string, string) {
	label := strings.TrimSpace(config.RewardName)
	if label == "" {
		label = defaultLiveOpsRewardConfig.RewardName
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(campaignID + ":" + userID + ":" + label))
	return "configured_" + strings.ToLower(strconv.FormatUint(uint64(hash.Sum32()), 36)), label
}

const liveOpsDisclaimer = "互动任务只用于直播间氛围和讲解偏好，不影响价格、排名、成交、保证金或订单权益。"

var defaultLiveOpsRewardConfig = LiveOpsRewardConfig{
	Enabled:           true,
	Title:             "直播间权益",
	Description:       "完成互动任务后领取直播间展示权益；当前不发放真实优惠券、奖品或订单权益。",
	RewardName:        "主播优先答疑",
	RewardQuota:       20,
	RequiredTaskCount: 3,
}

func normalizeLiveOpsRewardConfig(input LiveOpsRewardConfig) LiveOpsRewardConfig {
	out := input
	if strings.TrimSpace(out.Title) == "" {
		out.Title = defaultLiveOpsRewardConfig.Title
	}
	if strings.TrimSpace(out.Description) == "" {
		out.Description = defaultLiveOpsRewardConfig.Description
	}
	if strings.TrimSpace(out.RewardName) == "" {
		out.RewardName = defaultLiveOpsRewardConfig.RewardName
	}
	if out.RewardQuota <= 0 || out.RewardQuota > 9999 {
		out.RewardQuota = defaultLiveOpsRewardConfig.RewardQuota
	}
	if out.RequiredTaskCount <= 0 || out.RequiredTaskCount > len(liveOpsTaskCatalog) {
		out.RequiredTaskCount = defaultLiveOpsRewardConfig.RequiredTaskCount
	}
	return out
}

func defaultLiveOpsRulesJSON() string {
	raw, err := json.Marshal(LiveOpsCampaignRuleConfig{Reward: defaultLiveOpsRewardConfig})
	if err != nil {
		return `{}`
	}
	return string(raw)
}

func liveOpsOpenedRewardCount(rows []LiveOpsRewardEntry) int {
	count := 0
	for _, row := range rows {
		if row.Status == "OPENED" {
			count++
		}
	}
	return count
}

func maskLiveOpsUser(userID string) string {
	if userID == "" {
		return "-"
	}
	if len(userID) <= 4 {
		return userID
	}
	return userID[:2] + "***" + userID[len(userID)-2:]
}
