package errors

import "net/http"

type Code string

const (
	CodeInvalidArgument                          Code = "INVALID_ARGUMENT"
	CodeUnauthorized                             Code = "UNAUTHORIZED"
	CodeForbiddenRoom                            Code = "FORBIDDEN_ROOM"
	CodeAuctionNotFound                          Code = "AUCTION_NOT_FOUND"
	CodeAuctionNotActive                         Code = "AUCTION_NOT_ACTIVE"
	CodeAuctionEnded                             Code = "AUCTION_ENDED"
	CodeBidTooLow                                Code = "BID_TOO_LOW"
	CodeBidIncrementMismatch                     Code = "BID_INCREMENT_MISMATCH"
	CodeBidAboveCap                              Code = "BID_ABOVE_CAP"
	CodeMaxBidTooLow                             Code = "MAX_BID_TOO_LOW"
	CodeMaxBidIncrementMismatch                  Code = "MAX_BID_INCREMENT_MISMATCH"
	CodeMaxBidAboveCap                           Code = "MAX_BID_ABOVE_CAP"
	CodeRejectedSelfLeading                      Code = "REJECTED_SELF_LEADING"
	CodeFatFingerConfirmRequired                 Code = "FAT_FINGER_CONFIRM_REQUIRED"
	CodeRateLimited                              Code = "RATE_LIMITED"
	CodeBidAuctionTooHot                         Code = "BID_AUCTION_TOO_HOT"
	CodeBidRetryLater                            Code = "BID_RETRY_LATER"
	CodeIdempotencyKeyReusedWithDifferentRequest Code = "IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST"
	CodeProcessingRetryLater                     Code = "PROCESSING_RETRY_LATER"
	CodeIdempotencyTimeout                       Code = "IDEMPOTENCY_TIMEOUT"
	CodeIdempotencyMaxRetriesExceeded            Code = "IDEMPOTENCY_MAX_RETRIES_EXCEEDED"
	CodeInvalidAuctionRule                       Code = "INVALID_AUCTION_RULE"
	CodeInvalidAuctionRuleCapUnreachable         Code = "INVALID_AUCTION_RULE_CAP_UNREACHABLE"
	CodeRuleFrozenAfterScheduled                 Code = "RULE_FROZEN_AFTER_SCHEDULED"
	CodeConflictRoomHasActiveAuction             Code = "CONFLICT_ROOM_HAS_ACTIVE_AUCTION"
	CodeConflictRoomHasNarration                 Code = "CONFLICT_ROOM_HAS_NARRATION"
	CodeInvalidNarrateTarget                     Code = "INVALID_NARRATE_TARGET"
	CodeOrderAlreadyExpired                      Code = "ORDER_ALREADY_EXPIRED"
	CodeConfirmUsed                              Code = "CONFIRM_USED"
	CodeSlowConsumer                             Code = "SLOW_CONSUMER"
)

type APIError struct {
	Code    Code           `json:"code"`
	Message string         `json:"message"`
	TraceID string         `json:"trace_id"`
	Details map[string]any `json:"details,omitempty"`
	Status  int            `json:"-"`
}

func (e APIError) Error() string {
	return string(e.Code) + ": " + e.Message
}

func New(code Code, message string, status int) APIError {
	if status == 0 {
		status = http.StatusBadRequest
	}
	return APIError{Code: code, Message: message, Status: status}
}
