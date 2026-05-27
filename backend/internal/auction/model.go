package auction

import "time"

type Status string

const (
	StatusDraft     Status = "DRAFT"
	StatusScheduled Status = "SCHEDULED"
	StatusActive    Status = "ACTIVE"
	StatusSold      Status = "SOLD"
	StatusEnded     Status = "ENDED"
	StatusCancelled Status = "CANCELLED"
)

type Item struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	ImageURL    *string   `json:"image_url,omitempty"`
	Description *string   `json:"description,omitempty"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type Rule struct {
	DurationSeconds         int        `json:"duration_seconds"`
	ExtendWindowSeconds     int        `json:"extend_window_seconds"`
	ExtendBySeconds         int        `json:"extend_by_seconds"`
	MaxExtendCount          int        `json:"max_extend_count"`
	FatFingerThresholdCents *int64     `json:"fat_finger_threshold_cents,omitempty"`
	DepositBPS              int16      `json:"deposit_bps"`
	DepositFloorCents       int64      `json:"deposit_floor_cents"`
	DepositCapCents         int64      `json:"deposit_cap_cents"`
	FrozenAt                *time.Time `json:"frozen_at,omitempty"`
}

type UpdateRulesInput struct {
	StartPriceCents int64  `json:"start_price_cents"`
	IncrementCents  int64  `json:"increment_cents"`
	CapPriceCents   *int64 `json:"cap_price_cents"`
	Rule
}

type CancelInput struct {
	Reason string `json:"reason"`
}

type Auction struct {
	ID                string     `json:"id"`
	RoomID            string     `json:"room_id"`
	ItemID            string     `json:"item_id"`
	Status            Status     `json:"status"`
	IsNarrating       bool       `json:"is_narrating"`
	CurrentPriceCents int64      `json:"current_price_cents"`
	CurrentWinnerID   *string    `json:"current_winner_id,omitempty"`
	StartPriceCents   int64      `json:"start_price_cents"`
	IncrementCents    int64      `json:"increment_cents"`
	CapPriceCents     *int64     `json:"cap_price_cents,omitempty"`
	StartAt           *time.Time `json:"start_at,omitempty"`
	EndAt             *time.Time `json:"end_at,omitempty"`
	ServerTimeMS      int64      `json:"server_time_ms"`
	Version           int64      `json:"version"`
	Seq               int64      `json:"seq"`
	AcceptedBidCount  int64      `json:"accepted_bid_count"`
	ExtendCount       int        `json:"extend_count"`
	RuleVersion       int        `json:"rule_version"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	Item              Item       `json:"item"`
	Rule              Rule       `json:"rule"`
}

type MaxBidIntentStatus string

const (
	MaxBidIntentStatusActive    MaxBidIntentStatus = "ACTIVE"
	MaxBidIntentStatusCancelled MaxBidIntentStatus = "CANCELLED"
	MaxBidIntentStatusExhausted MaxBidIntentStatus = "EXHAUSTED"
	MaxBidIntentStatusTerminal  MaxBidIntentStatus = "TERMINAL"
)

type MaxBidIntentSource string

const (
	MaxBidIntentSourcePreBid MaxBidIntentSource = "PRE_BID"
	MaxBidIntentSourceMaxBid MaxBidIntentSource = "MAX_BID"
)

type MaxBidIntent struct {
	ID             string             `json:"id"`
	AuctionID      string             `json:"auction_id"`
	UserID         string             `json:"user_id"`
	MaxAmountCents int64              `json:"max_amount_cents"`
	Status         MaxBidIntentStatus `json:"status"`
	Source         MaxBidIntentSource `json:"source"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	CancelledAt    *time.Time         `json:"cancelled_at,omitempty"`
	ExhaustedAt    *time.Time         `json:"exhausted_at,omitempty"`
	LastAppliedSeq *int64             `json:"last_applied_seq,omitempty"`
	Version        int64              `json:"version"`
}

type MaxBidIntentInput struct {
	MaxAmountCents int64              `json:"max_amount_cents"`
	ClientSeenSeq  int64              `json:"client_seen_seq"`
	Source         MaxBidIntentSource `json:"source"`
}

type MaxBidIntentResponse struct {
	Result string       `json:"result"`
	Intent MaxBidIntent `json:"intent"`
}

type LeaderboardEntry struct {
	Rank        int       `json:"rank"`
	UserID      string    `json:"user_id"`
	UserMasked  string    `json:"user_masked"`
	AmountCents int64     `json:"amount_cents"`
	BidCount    int64     `json:"bid_count"`
	LastBidAt   time.Time `json:"last_bid_at"`
	IsCurrent   bool      `json:"is_current"`
}

type Leaderboard struct {
	AuctionID           string             `json:"auction_id"`
	Seq                 int64              `json:"seq"`
	ServerTimeMS        int64              `json:"server_time_ms"`
	CurrentPriceCents   int64              `json:"current_price_cents"`
	CurrentWinnerID     *string            `json:"current_winner_id,omitempty"`
	MyRank              *int               `json:"my_rank,omitempty"`
	MyBestAmountCents   *int64             `json:"my_best_amount_cents,omitempty"`
	GapToLeaderCents    *int64             `json:"gap_to_leader_cents,omitempty"`
	GapToNextRankCents  *int64             `json:"gap_to_next_rank_cents,omitempty"`
	NextValidBidCents   int64              `json:"next_valid_bid_cents"`
	State               string             `json:"state"`
	LeaderAmountCents   int64              `json:"leader_amount_cents"`
	AcceptedBidderCount int64              `json:"accepted_bidder_count"`
	ActiveBidders30s    int64              `json:"active_bidders_30s,omitempty"`
	AcceptedBids30s     int64              `json:"accepted_bids_30s,omitempty"`
	PriceVelocityCPM    int64              `json:"price_velocity_cents_per_min,omitempty"`
	Entries             []LeaderboardEntry `json:"entries"`
}
