package gateway

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	apierrors "live-auction/backend/internal/platform/errors"
	"live-auction/backend/internal/storage"
)

type HostPrompterHandler struct {
	Deps *storage.Dependencies
}

type hostPrompt struct {
	ID             string     `json:"id"`
	Type           string     `json:"type"`
	Severity       string     `json:"severity"`
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	Action         string     `json:"action"`
	Source         string     `json:"source"`
	AuctionID      string     `json:"auction_id"`
	RoomID         string     `json:"room_id"`
	EventSeq       *int64     `json:"event_seq,omitempty"`
	GeneratedAt    time.Time  `json:"generated_at"`
	WindowSeconds  int        `json:"window_seconds"`
	MetricValue    int64      `json:"metric_value,omitempty"`
	MetricLabel    string     `json:"metric_label,omitempty"`
	ReferencePrice *int64     `json:"reference_price_cents,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

type hostPrompterAuctionState struct {
	AuctionID        string
	RoomID           string
	Status           string
	CurrentPrice     int64
	Increment        int64
	EndAt            pgtype.Timestamptz
	Seq              int64
	AcceptedBidCount int64
	ExtendCount      int32
	LastBidAt        pgtype.Timestamptz
	LastEventType    pgtype.Text
	LastEventSeq     pgtype.Int8
	OrderID          pgtype.Text
	OrderStatus      pgtype.Text
	OrderExpireAt    pgtype.Timestamptz
}

func (h HostPrompterHandler) Prompts(w http.ResponseWriter, r *http.Request) {
	auctionID := strings.TrimSpace(r.PathValue("id"))
	if auctionID == "" {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "auction id is required", http.StatusBadRequest))
		return
	}
	state, err := loadHostPrompterAuctionState(r.Context(), h.Deps.Postgres, auctionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, r, apierrors.New(apierrors.CodeAuctionNotFound, "auction not found", http.StatusNotFound))
			return
		}
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "host prompter query failed", http.StatusInternalServerError))
		return
	}
	recent, err := loadHostPrompterRecentCounts(r.Context(), h.Deps.Postgres, auctionID)
	if err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "host prompter query failed", http.StatusInternalServerError))
		return
	}
	now := time.Now().UTC()
	prompts := buildHostPrompts(state, recent, now)
	writeJSON(w, http.StatusOK, map[string]any{
		"auction_id":   state.AuctionID,
		"room_id":      state.RoomID,
		"generated_at": now,
		"prompts":      prompts,
	})
}

type hostPrompterRecentCounts struct {
	AcceptedBids30s int64
	RejectedBids30s int64
	UniqueUsers30s  int64
}

func loadHostPrompterAuctionState(ctx context.Context, db *pgxpool.Pool, auctionID string) (hostPrompterAuctionState, error) {
	var state hostPrompterAuctionState
	err := db.QueryRow(ctx, `
		WITH last_bid AS (
			SELECT max(created_at) AS last_bid_at
			FROM bids
			WHERE auction_id = $1 AND status = 'ACCEPTED'
		),
		last_event AS (
			SELECT event_type, seq
			FROM auction_events
			WHERE auction_id = $1
			ORDER BY seq DESC
			LIMIT 1
		)
		SELECT a.id, a.room_id, a.status, a.current_price_cents, a.increment_cents,
		       a.end_at, a.seq, a.accepted_bid_count, a.extend_count,
		       last_bid.last_bid_at, last_event.event_type, last_event.seq,
		       o.id, o.status, o.expire_at
		FROM auctions a
		LEFT JOIN last_bid ON true
		LEFT JOIN last_event ON true
		LEFT JOIN orders o ON o.auction_id = a.id
		WHERE a.id = $1
	`, auctionID).Scan(
		&state.AuctionID, &state.RoomID, &state.Status, &state.CurrentPrice, &state.Increment,
		&state.EndAt, &state.Seq, &state.AcceptedBidCount, &state.ExtendCount,
		&state.LastBidAt, &state.LastEventType, &state.LastEventSeq,
		&state.OrderID, &state.OrderStatus, &state.OrderExpireAt,
	)
	return state, err
}

func loadHostPrompterRecentCounts(ctx context.Context, db *pgxpool.Pool, auctionID string) (hostPrompterRecentCounts, error) {
	var counts hostPrompterRecentCounts
	err := db.QueryRow(ctx, `
		SELECT
		  count(*) FILTER (WHERE status = 'ACCEPTED' AND created_at >= now() - interval '30 seconds') AS accepted_30s,
		  count(*) FILTER (WHERE status = 'REJECTED' AND created_at >= now() - interval '30 seconds') AS rejected_30s,
		  count(DISTINCT user_id) FILTER (WHERE created_at >= now() - interval '30 seconds') AS users_30s
		FROM bids
		WHERE auction_id = $1
	`, auctionID).Scan(&counts.AcceptedBids30s, &counts.RejectedBids30s, &counts.UniqueUsers30s)
	return counts, err
}

func buildHostPrompts(state hostPrompterAuctionState, counts hostPrompterRecentCounts, now time.Time) []hostPrompt {
	prompts := make([]hostPrompt, 0, 5)
	if state.Status == "ACTIVE" {
		lastBidAgeSeconds := int64(0)
		if state.LastBidAt.Valid {
			lastBidAgeSeconds = int64(now.Sub(state.LastBidAt.Time).Seconds())
		} else {
			lastBidAgeSeconds = 9999
		}
		if state.AcceptedBidCount == 0 || lastBidAgeSeconds >= 15 {
			prompts = append(prompts, newHostPrompt(state, now, "no_bid", "MED", "冷场提醒", "近 15 秒没有有效出价，建议讲商品证据、瑕疵和包邮规则。", "open_talk_points", "bids", 30, lastBidAgeSeconds, "seconds_since_last_bid"))
		}
		if state.EndAt.Valid {
			remaining := int64(state.EndAt.Time.Sub(now).Seconds())
			if remaining >= 0 && remaining <= 10 {
				p := newHostPrompt(state, now, "last_10_seconds", "HIGH", "最后窗口", "竞拍进入最后 10 秒，建议提醒下一口有效价和延时规则。", "highlight_countdown", "auction", 10, remaining, "seconds_remaining")
				expiresAt := state.EndAt.Time
				p.ExpiresAt = &expiresAt
				prompts = append(prompts, p)
			}
		}
		if state.LastEventType.Valid && state.LastEventType.String == "auction_extended" {
			prompts = append(prompts, newHostPrompt(state, now, "extension_triggered", "HIGH", "延时已触发", "刚发生延时，建议解释服务端已更新结束时间，避免用户误解倒计时回跳。", "explain_extension", "auction_events", 30, int64(state.ExtendCount), "extend_count"))
		}
		if counts.RejectedBids30s >= 3 || counts.AcceptedBids30s >= 3 {
			prompts = append(prompts, newHostPrompt(state, now, "high_bid_frequency", "MED", "竞争升温", "近 30 秒出价或被拒绝请求密集，建议口播当前差价、封顶价和保证金规则。", "highlight_rules", "bids", 30, counts.AcceptedBids30s+counts.RejectedBids30s, "bid_events_30s"))
		}
	}
	if state.Status == "SOLD" && state.OrderStatus.Valid && state.OrderStatus.String == "ORDER_PENDING" {
		prompt := newHostPrompt(state, now, "sold_unpaid", "HIGH", "成交待支付", "成交订单仍待支付，建议提醒赢家支付倒计时并准备下一件承接。", "open_orders", "orders", 0, 1, "pending_order")
		if state.OrderExpireAt.Valid {
			expiresAt := state.OrderExpireAt.Time
			prompt.ExpiresAt = &expiresAt
		}
		prompts = append(prompts, prompt)
	}
	return prompts
}

func newHostPrompt(state hostPrompterAuctionState, now time.Time, typ, severity, title, body, action, source string, windowSeconds int, metricValue int64, metricLabel string) hostPrompt {
	var eventSeq *int64
	if state.LastEventSeq.Valid {
		seq := state.LastEventSeq.Int64
		eventSeq = &seq
	}
	var referencePrice *int64
	if state.Status == "ACTIVE" {
		price := state.CurrentPrice + state.Increment
		referencePrice = &price
	}
	return hostPrompt{
		ID:             state.AuctionID + ":" + typ,
		Type:           typ,
		Severity:       severity,
		Title:          title,
		Body:           body,
		Action:         action,
		Source:         source,
		AuctionID:      state.AuctionID,
		RoomID:         state.RoomID,
		EventSeq:       eventSeq,
		GeneratedAt:    now,
		WindowSeconds:  windowSeconds,
		MetricValue:    metricValue,
		MetricLabel:    metricLabel,
		ReferencePrice: referencePrice,
	}
}
