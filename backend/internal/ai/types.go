package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

const (
	PromptVersionListingDraft = "listing_draft_v1"
	PromptVersionCommentary   = "commentary_v1"
	PromptVersionSentinel     = "sentinel_v1"
	PromptVersionRecap        = "recap_v1"
	PromptVersionProductQA    = "product_qa_v1"
)

var ErrDisabled = errors.New("ai provider disabled")

type StructuredRequest struct {
	Kind          string
	PromptVersion string
	SchemaName    string
	Input         map[string]any
	Timeout       time.Duration
}

type StructuredResult struct {
	Provider string
	Model    string
	Output   map[string]any
	Safety   map[string]any
}

type Generator interface {
	GenerateStructured(ctx context.Context, req StructuredRequest) (StructuredResult, error)
}

type ListingDraftRequest struct {
	RoomID         string   `json:"room_id"`
	ImageURLs      []string `json:"image_urls"`
	ImageDataURLs  []string `json:"image_data_urls"`
	SellerNotes    string   `json:"seller_notes"`
	TargetCategory string   `json:"target_category"`
}

type RuleSuggestion struct {
	StartPriceCents         int64 `json:"start_price_cents"`
	IncrementCents          int64 `json:"increment_cents"`
	CapPriceCents           int64 `json:"cap_price_cents"`
	DurationSeconds         int   `json:"duration_seconds"`
	ExtendWindowSeconds     int   `json:"extend_window_seconds"`
	ExtendBySeconds         int   `json:"extend_by_seconds"`
	MaxExtendCount          int   `json:"max_extend_count"`
	FatFingerThresholdCents int64 `json:"fat_finger_threshold_cents"`
}

type ListingDraft struct {
	TitleCandidates     []string       `json:"title_candidates"`
	Description         string         `json:"description"`
	Category            string         `json:"category"`
	SellingPoints       []string       `json:"selling_points"`
	ConditionQuestions  []string       `json:"condition_questions"`
	ComplianceFlags     []string       `json:"compliance_flags"`
	RuleSuggestion      RuleSuggestion `json:"rule_suggestion"`
	Confidence          float64        `json:"confidence"`
	Rationale           string         `json:"rationale"`
	RequiresEvidence    []string       `json:"requires_evidence"`
	UnsupportedClaims   []string       `json:"unsupported_claims"`
	HumanReviewRequired bool           `json:"human_review_required"`
}

type CommentaryRequest struct {
	RoomID              string `json:"room_id"`
	AuctionID           string `json:"auction_id"`
	SourceSeq           int64  `json:"source_seq"`
	EventType           string `json:"event_type"`
	ItemTitle           string `json:"item_title"`
	CurrentPriceCents   int64  `json:"current_price_cents"`
	CurrentWinnerMasked string `json:"current_winner_masked"`
	ActiveBidders30s    int64  `json:"active_bidders_30s"`
	AcceptedBids30s     int64  `json:"accepted_bids_30s"`
}

type CommentaryQueueStats struct {
	Enqueued  int `json:"enqueued"`
	Processed int `json:"processed"`
	Failed    int `json:"failed"`
}

type AutoCommentaryWorkerOptions struct {
	WorkerID         string
	AuctionID        string
	PollInterval     time.Duration
	BatchSize        int
	Lease            time.Duration
	BackfillLookback time.Duration
}

type SystemMessage struct {
	ID        int64          `json:"id"`
	RoomID    string         `json:"room_id"`
	AuctionID string         `json:"auction_id,omitempty"`
	Source    string         `json:"source"`
	SourceSeq *int64         `json:"source_seq,omitempty"`
	Style     string         `json:"style"`
	Body      string         `json:"body"`
	Facts     map[string]any `json:"facts_json"`
	Safety    map[string]any `json:"safety_json"`
	CreatedAt time.Time      `json:"created_at"`
}

type SentinelAlert struct {
	ID                int64          `json:"id"`
	RoomID            string         `json:"room_id"`
	AuctionID         string         `json:"auction_id"`
	Severity          string         `json:"severity"`
	RiskType          string         `json:"risk_type"`
	Score             int            `json:"score"`
	Explanation       string         `json:"explanation"`
	RecommendedAction string         `json:"recommended_action"`
	Features          map[string]any `json:"features_json"`
	Status            string         `json:"status"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type SentinelEvaluationInput struct {
	RoomID     string          `json:"room_id"`
	AuctionID  string          `json:"auction_id"`
	ItemTitle  string          `json:"item_title"`
	Status     string          `json:"status"`
	Features   map[string]any  `json:"features"`
	Candidates []SentinelAlert `json:"candidates"`
}

type AuctionRecap struct {
	AuctionID       string         `json:"auction_id"`
	RoomID          string         `json:"room_id"`
	ItemTitle       string         `json:"item_title"`
	Status          string         `json:"status"`
	FinalPriceCents int64          `json:"final_price_cents"`
	WinnerMasked    string         `json:"winner_masked,omitempty"`
	AcceptedBids    int64          `json:"accepted_bids"`
	AcceptedBidders int64          `json:"accepted_bidders"`
	ExtendCount     int            `json:"extend_count"`
	Highlights      []string       `json:"highlights"`
	NextActions     []string       `json:"next_actions"`
	ShareCard       map[string]any `json:"share_card"`
	GeneratedAt     time.Time      `json:"generated_at"`
}

type HighlightAsset struct {
	ID            string         `json:"id"`
	AuctionID     string         `json:"auction_id"`
	RoomID        string         `json:"room_id"`
	JobID         string         `json:"job_id"`
	Status        string         `json:"status"`
	MediaType     string         `json:"media_type"`
	Title         string         `json:"title"`
	AssetURL      string         `json:"asset_url"`
	RenderProfile string         `json:"render_profile"`
	DurationMS    int            `json:"duration_ms"`
	Facts         map[string]any `json:"facts_json"`
	Risk          map[string]any `json:"risk_json"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type ProductQARequest struct {
	AuctionID string          `json:"auction_id"`
	ThreadID  string          `json:"thread_id,omitempty"`
	Question  string          `json:"question"`
	History   []ProductQATurn `json:"history,omitempty"`
}

type ProductQAAnswer struct {
	AuctionID        string   `json:"auction_id"`
	ThreadID         string   `json:"thread_id,omitempty"`
	Question         string   `json:"question,omitempty"`
	Answer           string   `json:"answer"`
	FactsUsed        []string `json:"facts_used"`
	SafetyNote       string   `json:"safety_note"`
	FollowUpPrompts  []string `json:"follow_up_prompts,omitempty"`
	ContextTurnCount int      `json:"context_turn_count,omitempty"`
}

type ProductQATurn struct {
	Question  string   `json:"question"`
	Answer    string   `json:"answer"`
	FactsUsed []string `json:"facts_used,omitempty"`
}

type AuctionAISettings struct {
	AuctionID             string    `json:"auction_id"`
	AutoCommentaryEnabled bool      `json:"auto_commentary_enabled"`
	UpdatedBy             string    `json:"updated_by,omitempty"`
	UpdatedAt             time.Time `json:"updated_at"`
}

func InputHash(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func NormalizeListingDraft(raw map[string]any, req ListingDraftRequest) ListingDraft {
	draft := ListingDraft{
		TitleCandidates:     stringSlice(raw["title_candidates"]),
		Description:         cleanText(stringValue(raw["description"]), 360),
		Category:            cleanText(firstNonEmpty(stringValue(raw["category"]), req.TargetCategory, "非标拍品"), 40),
		SellingPoints:       limitStrings(stringSlice(raw["selling_points"]), 5, 28),
		ConditionQuestions:  limitStrings(stringSlice(raw["condition_questions"]), 5, 36),
		ComplianceFlags:     limitStrings(stringSlice(raw["compliance_flags"]), 6, 36),
		RequiresEvidence:    limitStrings(stringSlice(raw["requires_evidence"]), 6, 36),
		UnsupportedClaims:   limitStrings(stringSlice(raw["unsupported_claims"]), 6, 36),
		Confidence:          clampFloat(floatValue(raw["confidence"]), 0, 1),
		Rationale:           cleanText(stringValue(raw["rationale"]), 160),
		HumanReviewRequired: true,
	}
	if len(draft.TitleCandidates) == 0 {
		title := titleFromNotes(req.SellerNotes, req.TargetCategory)
		draft.TitleCandidates = []string{title}
	}
	draft.TitleCandidates = limitStrings(draft.TitleCandidates, 3, 32)
	if draft.Description == "" {
		draft.Description = descriptionFromNotes(req.SellerNotes, req.TargetCategory)
	}
	if len(draft.ConditionQuestions) == 0 {
		draft.ConditionQuestions = []string{"请补充来源证明", "请确认瑕疵、尺寸和配件"}
	}
	draft.RuleSuggestion = normalizeRuleSuggestion(raw["rule_suggestion"], req)
	draft.UnsupportedClaims = append(draft.UnsupportedClaims, unsafeClaims(draft.Description)...)
	if len(draft.UnsupportedClaims) > 0 && !contains(draft.ComplianceFlags, "需要证据后再展示真伪/年代承诺") {
		draft.ComplianceFlags = append(draft.ComplianceFlags, "需要证据后再展示真伪/年代承诺")
	}
	if draft.Confidence == 0 {
		draft.Confidence = 0.72
	}
	if draft.Rationale == "" {
		draft.Rationale = "基于商家备注生成草稿；标题、描述和规则必须由主播确认后发布。"
	}
	return draft
}

func BuildFallbackListingDraft(req ListingDraftRequest) ListingDraft {
	return NormalizeListingDraft(map[string]any{
		"title_candidates": []string{titleFromNotes(req.SellerNotes, req.TargetCategory)},
		"description":      descriptionFromNotes(req.SellerNotes, req.TargetCategory),
		"category":         firstNonEmpty(req.TargetCategory, "非标拍品"),
		"selling_points":   []string{"直播间可展示细节", "规则建议需人工确认"},
		"condition_questions": []string{
			"是否有证书或购买凭证",
			"是否存在修复、磕碰或明显瑕疵",
		},
		"compliance_flags":  []string{"AI 草稿不可直接作为鉴定承诺"},
		"requires_evidence": []string{"证书", "来源说明", "瑕疵照片"},
		"confidence":        0.68,
	}, req)
}

func BuildCommentary(req CommentaryRequest) (body string, style string, safety map[string]any) {
	style = "steady"
	switch req.EventType {
	case "bid_accepted":
		style = "heat"
		body = "价格来到" + FormatCents(req.CurrentPriceCents) + "，榜首已刷新。"
	case "extended":
		style = "critical"
		body = "最后窗口继续加时，先看真实倒计时。"
	case "auction_sold", "sold":
		style = "sold"
		body = "落锤成交：" + req.ItemTitle + " " + FormatCents(req.CurrentPriceCents) + "。"
	case "auction_cancelled", "cancelled":
		style = "calm"
		body = "本场已由主播取消，资金和订单以系统记录为准。"
	case "auction_ended", "ended":
		style = "calm"
		body = "本场结束，未成交拍品等待主播安排。"
	default:
		if req.AcceptedBids30s >= 3 {
			style = "heat"
			body = "近30秒出价升温，当前价" + FormatCents(req.CurrentPriceCents) + "。"
		} else {
			body = "当前价" + FormatCents(req.CurrentPriceCents) + "，按规则出价。"
		}
	}
	return cleanText(body, 40), style, map[string]any{
		"facts_used":        []string{"auction_id", "source_seq", "current_price_cents", "event_type"},
		"no_hidden_max_bid": true,
		"no_fake_urgency":   true,
	}
}

func NormalizeSentinelAlerts(raw map[string]any, input SentinelEvaluationInput) []SentinelAlert {
	rawAlerts, _ := raw["alerts"].([]any)
	out := make([]SentinelAlert, 0, len(rawAlerts))
	allowedRiskTypes := map[string]bool{}
	candidatesByType := map[string]SentinelAlert{}
	for _, candidate := range input.Candidates {
		allowedRiskTypes[candidate.RiskType] = true
		candidatesByType[candidate.RiskType] = candidate
	}
	for _, item := range rawAlerts {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		riskType := cleanText(stringValue(itemMap["risk_type"]), 64)
		if !allowedRiskTypes[riskType] {
			continue
		}
		base := candidatesByType[riskType]
		score := int(clampFloat(floatValue(itemMap["score"]), 1, 100))
		if score == 0 {
			score = base.Score
		}
		base.Score = score
		base.Severity = normalizeSeverity(firstNonEmpty(stringValue(itemMap["severity"]), base.Severity))
		base.Explanation = cleanText(firstNonEmpty(stringValue(itemMap["explanation"]), base.Explanation), 160)
		base.RecommendedAction = cleanText(firstNonEmpty(stringValue(itemMap["recommended_action"]), base.RecommendedAction), 120)
		base.Features = ensureMap(base.Features)
		base.Features["provider_reviewed"] = true
		base.Features["facts_used"] = limitStrings(stringSlice(itemMap["facts_used"]), 8, 48)
		out = append(out, base)
	}
	if len(out) == 0 {
		return input.Candidates
	}
	return out
}

func NormalizeProductQAAnswer(raw map[string]any, fallback ProductQAAnswer, allowedFacts map[string]string) ProductQAAnswer {
	answer := ProductQAAnswer{
		AuctionID:        fallback.AuctionID,
		ThreadID:         fallback.ThreadID,
		Question:         fallback.Question,
		Answer:           cleanText(stringValue(raw["answer"]), 180),
		FactsUsed:        limitStrings(stringSlice(raw["facts_used"]), 8, 64),
		SafetyNote:       cleanText(firstNonEmpty(stringValue(raw["safety_note"]), "只回答已审核拍品和规则信息。"), 80),
		FollowUpPrompts:  limitStrings(stringSlice(raw["follow_up_prompts"]), 3, 28),
		ContextTurnCount: fallback.ContextTurnCount,
	}
	if answer.Answer == "" {
		return fallback
	}
	if strings.Contains(answer.Answer, "保真") || strings.Contains(answer.Answer, "升值") || strings.Contains(answer.Answer, "稳赚") || strings.Contains(answer.Answer, "隐藏") {
		return fallback
	}
	validFacts := []string{}
	for _, fact := range answer.FactsUsed {
		if _, ok := allowedFacts[fact]; ok {
			validFacts = append(validFacts, fact)
		}
	}
	if len(validFacts) == 0 {
		return fallback
	}
	answer.FactsUsed = validFacts
	if strings.TrimSpace(answer.SafetyNote) == "" {
		answer.SafetyNote = "不提供真伪、投资收益或隐藏出价信息。"
	}
	if len(answer.FollowUpPrompts) == 0 {
		answer.FollowUpPrompts = fallback.FollowUpPrompts
	}
	return answer
}

func BuildRecap(input AuctionRecap) AuctionRecap {
	input.GeneratedAt = time.Now().UTC()
	input.Highlights = append(input.Highlights, "成交价 "+FormatCents(input.FinalPriceCents))
	if input.AcceptedBidders > 0 {
		input.Highlights = append(input.Highlights, "真实参与出价 "+int64Text(input.AcceptedBidders)+" 人")
	}
	if input.ExtendCount > 0 {
		input.Highlights = append(input.Highlights, "末段延时 "+int64Text(int64(input.ExtendCount))+" 次")
	}
	if len(input.NextActions) == 0 {
		if input.Status == "SOLD" {
			input.NextActions = []string{"提醒赢家完成支付", "准备下一件承接热度"}
		} else {
			input.NextActions = []string{"复盘起拍价和讲解节奏", "补充商品证据后重新排期"}
		}
	}
	input.ShareCard = map[string]any{
		"title":             input.ItemTitle,
		"status":            input.Status,
		"final_price_cents": input.FinalPriceCents,
		"accepted_bids":     input.AcceptedBids,
		"privacy":           "buyer identities masked",
	}
	return input
}

func AnswerFromFacts(auctionID string, question string, itemTitle string, description string, rules map[string]any) ProductQAAnswer {
	q := strings.ToLower(strings.TrimSpace(question))
	answers := []string{}
	facts := []string{}
	if strings.Contains(q, "标题") || strings.Contains(q, "拍品") || strings.Contains(q, "商品") {
		answers = append(answers, itemTitle)
		facts = append(facts, "item.title")
	}
	if (strings.Contains(q, "描述") || strings.Contains(q, "瑕疵") || strings.Contains(q, "来源")) && strings.TrimSpace(description) != "" {
		answers = append(answers, cleanText(description, 120))
		facts = append(facts, "item.description")
	}
	if strings.Contains(q, "加价") {
		answers = append(answers, "每次加价 "+FormatCents(int64Value(rules["increment_cents"])))
		facts = append(facts, "auction.increment_cents")
	}
	if strings.Contains(q, "起拍") {
		answers = append(answers, "起拍价 "+FormatCents(int64Value(rules["start_price_cents"])))
		facts = append(facts, "auction.start_price_cents")
	}
	if strings.Contains(q, "封顶") {
		capValue := int64Value(rules["cap_price_cents"])
		if capValue > 0 {
			answers = append(answers, "封顶价 "+FormatCents(capValue))
			facts = append(facts, "auction.cap_price_cents")
		}
	}
	if len(answers) == 0 {
		return ProductQAAnswer{AuctionID: auctionID, Question: cleanText(question, 120), Answer: "未提供", FactsUsed: []string{}, SafetyNote: "只回答已审核拍品和规则信息。", FollowUpPrompts: defaultProductQAFollowUps(description, rules)}
	}
	answer := cleanText(strings.Join(answers, "；"), 180)
	return ProductQAAnswer{AuctionID: auctionID, Question: cleanText(question, 120), Answer: answer, FactsUsed: facts, SafetyNote: "不提供真伪、投资收益或隐藏出价信息。", FollowUpPrompts: defaultProductQAFollowUps(description, rules)}
}

func defaultProductQAFollowUps(description string, rules map[string]any) []string {
	prompts := []string{"起拍价和加价是多少？"}
	if strings.TrimSpace(description) != "" {
		prompts = append(prompts, "有瑕疵或来源说明吗？")
	}
	if int64Value(rules["cap_price_cents"]) > 0 {
		prompts = append(prompts, "封顶价是多少？")
	}
	return limitStrings(prompts, 3, 28)
}

func normalizeSeverity(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "LOW", "MED", "HIGH":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return "MED"
	}
}

func FormatCents(cents int64) string {
	return fmt.Sprintf("¥%.2f", float64(cents)/100)
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func stringSlice(v any) []string {
	switch value := v.(type) {
	case []string:
		return value
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if s := stringValue(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func limitStrings(values []string, max int, maxRunes int) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = cleanText(value, maxRunes)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
		if len(out) >= max {
			break
		}
	}
	return out
}

func cleanText(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if maxRunes <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return value
}

func floatValue(v any) float64 {
	switch value := v.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		out, _ := value.Float64()
		return out
	default:
		return 0
	}
}

func int64Value(v any) int64 {
	switch value := v.(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	case json.Number:
		out, _ := value.Int64()
		return out
	default:
		return 0
	}
}

func clampFloat(v float64, min float64, max float64) float64 {
	return math.Max(min, math.Min(max, v))
}

func normalizeRuleSuggestion(v any, req ListingDraftRequest) RuleSuggestion {
	raw, _ := v.(map[string]any)
	start := int64Value(raw["start_price_cents"])
	inc := int64Value(raw["increment_cents"])
	capValue := int64Value(raw["cap_price_cents"])
	if start <= 0 {
		start = 10_000
	}
	if inc <= 0 {
		inc = 1_000
	}
	if capValue <= start+inc {
		capValue = start + inc*20
	}
	if remainder := (capValue - start) % inc; remainder != 0 {
		capValue += inc - remainder
	}
	return RuleSuggestion{
		StartPriceCents:         start,
		IncrementCents:          inc,
		CapPriceCents:           capValue,
		DurationSeconds:         clampInt(int64Value(raw["duration_seconds"]), 60, 1800, 180),
		ExtendWindowSeconds:     clampInt(int64Value(raw["extend_window_seconds"]), 10, 30, 10),
		ExtendBySeconds:         clampInt(int64Value(raw["extend_by_seconds"]), 10, 30, 10),
		MaxExtendCount:          clampInt(int64Value(raw["max_extend_count"]), 1, 10, 5),
		FatFingerThresholdCents: maxInt64(int64Value(raw["fat_finger_threshold_cents"]), inc*8),
	}
}

func clampInt(v int64, min int, max int, fallback int) int {
	if v == 0 {
		return fallback
	}
	if v < int64(min) {
		return min
	}
	if v > int64(max) {
		return max
	}
	return int(v)
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func titleFromNotes(notes string, category string) string {
	notes = cleanText(notes, 18)
	if notes == "" {
		notes = firstNonEmpty(category, "精选拍品")
	}
	return notes + "直播竞拍"
}

func descriptionFromNotes(notes string, category string) string {
	notes = cleanText(notes, 120)
	if notes == "" {
		notes = firstNonEmpty(category, "非标拍品") + "，请主播补充证书、尺寸、瑕疵和来源后上架。"
	}
	return notes + "。本内容为 AI 草稿，发布前需主播确认。"
}

var unsafeClaimPattern = regexp.MustCompile(`(?i)(真品|保真|绝对|官方认证|升值|稳赚|guaranteed|authentic|certified)`)

func unsafeClaims(text string) []string {
	if unsafeClaimPattern.MatchString(text) {
		return []string{"疑似真伪/收益承诺"}
	}
	return nil
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func int64Text(value int64) string {
	b, _ := json.Marshal(value)
	return string(b)
}
