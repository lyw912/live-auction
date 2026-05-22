package auction

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	apierrors "live-auction/backend/internal/platform/errors"
)

type ChatMessage struct {
	ID        int64     `json:"id"`
	RoomID    string    `json:"room_id"`
	UserID    string    `json:"user_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateChatInput struct {
	ClientMsgID string `json:"client_msg_id"`
	Body        string `json:"body"`
}

func (r *Repository) CreateChatMessage(ctx context.Context, roomID string, userID string, input CreateChatInput, traceID string) (ChatMessage, error) {
	body := strings.TrimSpace(input.Body)
	if roomID == "" || userID == "" || body == "" {
		return ChatMessage{}, apierrors.New(apierrors.CodeInvalidArgument, "room_id, user_id and body are required", http.StatusBadRequest)
	}
	if len([]rune(body)) > 80 {
		return ChatMessage{}, apierrors.New(apierrors.CodeRateLimited, "chat body exceeds 80 chars", http.StatusBadRequest)
	}
	clientMsgID := input.ClientMsgID
	if clientMsgID == "" {
		clientMsgID = "msg_" + uuid.NewString()
	}
	var msg ChatMessage
	err := r.db.QueryRow(ctx, `
		INSERT INTO chat_messages (room_id, user_id, client_msg_id, body)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (room_id, user_id, client_msg_id)
		DO UPDATE SET body = chat_messages.body
		RETURNING id, room_id, user_id, body, created_at
	`, roomID, userID, clientMsgID, body).Scan(&msg.ID, &msg.RoomID, &msg.UserID, &msg.Body, &msg.CreatedAt)
	if err != nil {
		return ChatMessage{}, err
	}
	_, _ = r.db.Exec(ctx, `
		INSERT INTO user_activity_events (room_id, user_id, event_type, source, trace_id, payload_json)
		VALUES ($1, $2, 'chat_sent', 'h5', $3, jsonb_build_object('chat_id', $4))
	`, roomID, userID, traceID, msg.ID)
	return msg, nil
}

func (r *Repository) ListChatMessages(ctx context.Context, roomID string, limit int) ([]ChatMessage, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, room_id, user_id, body, created_at
		FROM chat_messages
		WHERE room_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`, roomID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := []ChatMessage{}
	for rows.Next() {
		var msg ChatMessage
		if err := rows.Scan(&msg.ID, &msg.RoomID, &msg.UserID, &msg.Body, &msg.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}
