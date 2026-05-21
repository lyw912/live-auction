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
