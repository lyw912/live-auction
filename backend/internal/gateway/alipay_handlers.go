package gateway

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/config"
	apierrors "live-auction/backend/internal/platform/errors"
	"log/slog"
)

type alipayCreateRequest struct {
	Confirm bool `json:"confirm"`
}

type alipayClientAction struct {
	Type   string `json:"type"`
	Method string `json:"method"`
	URL    string `json:"url"`
	HTML   string `json:"html"`
}

type alipayCreateResponse struct {
	OrderID           string             `json:"order_id"`
	OrderStatus       string             `json:"order_status"`
	Provider          string             `json:"provider"`
	ProviderPaymentID string             `json:"provider_payment_id"`
	ClientAction      alipayClientAction `json:"client_action"`
}

type alipayQueryResponse struct {
	OrderID           string     `json:"order_id"`
	OrderStatus       string     `json:"order_status"`
	Provider          string     `json:"provider"`
	ProviderPaymentID string     `json:"provider_payment_id"`
	ProviderTradeNo   string     `json:"provider_trade_no,omitempty"`
	TradeStatus       string     `json:"trade_status"`
	PaidAt            *time.Time `json:"paid_at,omitempty"`
	DepositStatus     string     `json:"deposit_status,omitempty"`
}

type alipayOrderForPayment struct {
	OrderID     string
	AuctionID   string
	WinnerID    string
	AmountCents int64
	Status      string
	Title       string
}

type alipayGatewayResponse struct {
	AlipayTradeQueryResponse struct {
		Code        string `json:"code"`
		Msg         string `json:"msg"`
		SubCode     string `json:"sub_code"`
		SubMsg      string `json:"sub_msg"`
		OutTradeNo  string `json:"out_trade_no"`
		TradeNo     string `json:"trade_no"`
		TradeStatus string `json:"trade_status"`
		TotalAmount string `json:"total_amount"`
	} `json:"alipay_trade_query_response"`
	Sign string `json:"sign"`
}

func (h AuctionHandler) PayOrder(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	var req alipayCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", http.StatusBadRequest))
		return
	}
	if !req.Confirm {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "confirm must be true", http.StatusBadRequest))
		return
	}
	result, err := h.createAlipayPagePay(r.Context(), chi.URLParam(r, "id"), user.ID)
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) QueryAlipayOrder(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	result, err := h.queryAlipayOrder(r.Context(), chi.URLParam(r, "id"), user.ID)
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) AlipayNotify(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid alipay notify body", http.StatusBadRequest))
		return
	}
	result, err := h.handleAlipayNotify(r.Context(), r.PostForm)
	if err != nil {
		writeResult(w, r, http.StatusOK, result, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("success"))
}

func (h AuctionHandler) createAlipayPagePay(ctx context.Context, orderID string, userID string) (alipayCreateResponse, error) {
	cfg := h.Config
	if !cfg.AlipaySandboxEnabled {
		return alipayCreateResponse{}, apierrors.WithDetails(
			apierrors.New(apierrors.CodeInvalidArgument, "alipay sandbox is not enabled", http.StatusPreconditionFailed),
			map[string]any{"missing": []string{"ALIPAY_SANDBOX_ENABLED=true"}},
		)
	}
	missing := missingAlipayConfig(cfg)
	if len(missing) > 0 {
		return alipayCreateResponse{}, apierrors.WithDetails(
			apierrors.New(apierrors.CodeInvalidArgument, "alipay sandbox config is incomplete", http.StatusPreconditionFailed),
			map[string]any{"missing": missing},
		)
	}
	privateKey, err := loadAlipayPrivateKey(cfg)
	if err != nil {
		return alipayCreateResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "invalid alipay private key: "+err.Error(), http.StatusPreconditionFailed)
	}
	order, err := h.loadOrderForAlipay(ctx, orderID)
	if err != nil {
		return alipayCreateResponse{}, err
	}
	if order.WinnerID != userID {
		return alipayCreateResponse{}, apierrors.New(apierrors.CodeForbiddenRoom, "only winner can pay order", http.StatusForbidden)
	}
	if order.Status == "ORDER_EXPIRED" {
		return alipayCreateResponse{}, apierrors.New(apierrors.CodeOrderAlreadyExpired, "order already expired", http.StatusConflict)
	}
	providerPaymentID := "alipay_" + order.OrderID
	if err := h.markAlipayPaymentInitiated(ctx, order, userID, providerPaymentID); err != nil {
		return alipayCreateResponse{}, err
	}
	if order.Status != "PAID" {
		order.Status = "PAYMENT_INITIATED"
	}
	form, gatewayURL, formMethod, err := buildAlipayPagePayForm(cfg, privateKey, order, providerPaymentID)
	if err != nil {
		return alipayCreateResponse{}, err
	}
	return alipayCreateResponse{
		OrderID:           order.OrderID,
		OrderStatus:       order.Status,
		Provider:          "alipay_sandbox",
		ProviderPaymentID: providerPaymentID,
		ClientAction: alipayClientAction{
			Type:   "redirect_form",
			Method: formMethod,
			URL:    gatewayURL,
			HTML:   form,
		},
	}, nil
}

func (h AuctionHandler) queryAlipayOrder(ctx context.Context, orderID string, userID string) (alipayQueryResponse, error) {
	cfg := h.Config
	missing := missingAlipayConfig(cfg)
	if !cfg.AlipaySandboxEnabled {
		missing = append([]string{"ALIPAY_SANDBOX_ENABLED=true"}, missing...)
	}
	if len(missing) > 0 {
		return alipayQueryResponse{}, apierrors.WithDetails(
			apierrors.New(apierrors.CodeInvalidArgument, "alipay sandbox config is incomplete", http.StatusPreconditionFailed),
			map[string]any{"missing": missing},
		)
	}
	privateKey, err := loadAlipayPrivateKey(cfg)
	if err != nil {
		return alipayQueryResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "invalid alipay private key: "+err.Error(), http.StatusPreconditionFailed)
	}
	publicKey, err := loadAlipayPublicKey(cfg)
	if err != nil {
		return alipayQueryResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "invalid alipay public key: "+err.Error(), http.StatusPreconditionFailed)
	}
	order, err := h.loadOrderForAlipay(ctx, orderID)
	if err != nil {
		return alipayQueryResponse{}, err
	}
	if order.WinnerID != userID {
		return alipayQueryResponse{}, apierrors.New(apierrors.CodeForbiddenRoom, "only winner can query order payment", http.StatusForbidden)
	}
	providerPaymentID := "alipay_" + order.OrderID
	gatewayResp, err := requestAlipayTradeQuery(ctx, cfg, privateKey, publicKey, providerPaymentID)
	if err != nil {
		return alipayQueryResponse{}, err
	}
	resp := gatewayResp.AlipayTradeQueryResponse
	if resp.Code != "10000" {
		return alipayQueryResponse{
				OrderID:           order.OrderID,
				OrderStatus:       order.Status,
				Provider:          "alipay_sandbox",
				ProviderPaymentID: providerPaymentID,
				TradeStatus:       resp.TradeStatus,
			}, apierrors.WithDetails(apierrors.New(apierrors.CodeProcessingRetryLater, "alipay trade is not paid yet", http.StatusAccepted), map[string]any{
				"alipay_code":     resp.Code,
				"alipay_sub_code": resp.SubCode,
				"alipay_message":  firstNonEmptyString(resp.SubMsg, resp.Msg),
			})
	}
	queryResult := alipayQueryResponse{
		OrderID:           order.OrderID,
		OrderStatus:       order.Status,
		Provider:          "alipay_sandbox",
		ProviderPaymentID: providerPaymentID,
		ProviderTradeNo:   resp.TradeNo,
		TradeStatus:       resp.TradeStatus,
	}
	if isAlipayPaidStatus(resp.TradeStatus) {
		providerEventID := "alipay_query_" + firstNonEmptyString(resp.TradeNo, providerPaymentID)
		payment, err := h.Repo.HandleProviderWebhook(ctx, auction.ProviderPaymentWebhook{
			Provider:          "alipay_sandbox",
			ProviderEventID:   providerEventID,
			ProviderPaymentID: providerPaymentID,
			OrderID:           order.OrderID,
			EventType:         "payment_succeeded",
			ProviderTradeNo:   resp.TradeNo,
			TradeStatus:       resp.TradeStatus,
			PaymentMethod:     cfg.AlipayPayMethod,
			Signature:         signTrustedAlipayWebhook(providerEventID, providerPaymentID, order.OrderID),
		}, trustedAlipayWebhookSecret(), traceID(ctx))
		if err != nil {
			return alipayQueryResponse{}, err
		}
		queryResult.OrderStatus = payment.OrderStatus
		queryResult.PaidAt = &payment.PaidAt
		queryResult.DepositStatus = payment.DepositStatus
	}
	return queryResult, nil
}

func (h AuctionHandler) handleAlipayNotify(ctx context.Context, form url.Values) (auction.PaymentResponse, error) {
	cfg := h.Config
	publicKey, err := loadAlipayPublicKey(cfg)
	if err != nil {
		return auction.PaymentResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "invalid alipay public key: "+err.Error(), http.StatusPreconditionFailed)
	}
	if !verifyAlipayFormSignature(form, publicKey) {
		return auction.PaymentResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "invalid alipay notify signature", http.StatusUnauthorized)
	}
	providerPaymentID := form.Get("out_trade_no")
	orderID := strings.TrimPrefix(providerPaymentID, "alipay_")
	if providerPaymentID == "" || orderID == providerPaymentID {
		return auction.PaymentResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "invalid alipay out_trade_no", http.StatusBadRequest)
	}
	tradeStatus := form.Get("trade_status")
	if !isAlipayPaidStatus(tradeStatus) {
		return auction.PaymentResponse{
			OrderID:           orderID,
			OrderStatus:       "PAYMENT_INITIATED",
			ProviderPaymentID: providerPaymentID,
		}, nil
	}
	providerEventID := "alipay_notify_" + firstNonEmptyString(form.Get("trade_no"), form.Get("notify_id"), providerPaymentID)
	return h.Repo.HandleProviderWebhook(ctx, auction.ProviderPaymentWebhook{
		Provider:          "alipay_sandbox",
		ProviderEventID:   providerEventID,
		ProviderPaymentID: providerPaymentID,
		OrderID:           orderID,
		EventType:         "payment_succeeded",
		ProviderTradeNo:   form.Get("trade_no"),
		TradeStatus:       tradeStatus,
		PaymentMethod:     cfg.AlipayPayMethod,
		Signature:         signTrustedAlipayWebhook(providerEventID, providerPaymentID, orderID),
	}, trustedAlipayWebhookSecret(), traceID(ctx))
}

func (h AuctionHandler) loadOrderForAlipay(ctx context.Context, orderID string) (alipayOrderForPayment, error) {
	var row alipayOrderForPayment
	err := h.Deps.Postgres.QueryRow(ctx, `
		SELECT o.id, o.auction_id, o.winner_id, o.amount_cents, o.status, COALESCE(i.title, o.auction_id)
		FROM orders o
		JOIN auctions a ON a.id = o.auction_id
		LEFT JOIN items i ON i.id = a.item_id
		WHERE o.id = $1
	`, orderID).Scan(&row.OrderID, &row.AuctionID, &row.WinnerID, &row.AmountCents, &row.Status, &row.Title)
	if err != nil {
		if err == pgx.ErrNoRows {
			return alipayOrderForPayment{}, apierrors.New(apierrors.CodeOrderAlreadyExpired, "order not found", http.StatusNotFound)
		}
		return alipayOrderForPayment{}, err
	}
	return row, nil
}

func (h AuctionHandler) markAlipayPaymentInitiated(ctx context.Context, order alipayOrderForPayment, userID string, providerPaymentID string) error {
	tx, err := h.Deps.Postgres.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var existingProviderID *string
	var status string
	if err := tx.QueryRow(ctx, `
		SELECT status, provider_payment_id
		FROM orders
		WHERE id = $1
		FOR UPDATE
	`, order.OrderID).Scan(&status, &existingProviderID); err != nil {
		return err
	}
	if existingProviderID != nil && *existingProviderID != providerPaymentID {
		return apierrors.New(apierrors.CodeInvalidArgument, "provider payment id does not match order", http.StatusConflict)
	}
	if status == "ORDER_EXPIRED" {
		return apierrors.New(apierrors.CodeOrderAlreadyExpired, "order already expired", http.StatusConflict)
	}
	now := time.Now().UTC()
	if status != "PAID" {
		if _, err := tx.Exec(ctx, `
			UPDATE orders
			SET status = 'PAYMENT_INITIATED',
			    provider_payment_id = $2,
			    payment_initiated_at = COALESCE(payment_initiated_at, $3)
			WHERE id = $1
		`, order.OrderID, providerPaymentID, now); err != nil {
			return err
		}
	}
	payload, _ := json.Marshal(map[string]any{
		"source":              "alipay_page_pay",
		"user_id":             userID,
		"provider_payment_id": providerPaymentID,
	})
	_, err = tx.Exec(ctx, `
		INSERT INTO payment_events (provider, provider_event_id, provider_payment_id, order_id, event_type, signature_valid, processed_at, payload_json, trace_id)
		VALUES ('alipay_sandbox', $1, $2, $3, 'payment_initiated', true, now(), $4, $5)
		ON CONFLICT (provider, provider_event_id) DO NOTHING
	`, "alipay_init_"+order.OrderID, providerPaymentID, order.OrderID, payload, traceID(ctx))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func missingAlipayConfig(cfg config.Config) []string {
	missing := []string{}
	if strings.TrimSpace(cfg.AlipayAppID) == "" {
		missing = append(missing, "ALIPAY_APP_ID")
	}
	if strings.TrimSpace(cfg.AlipayPrivateKey) == "" && strings.TrimSpace(cfg.AlipayPrivateKeyPath) == "" {
		missing = append(missing, "ALIPAY_PRIVATE_KEY or ALIPAY_PRIVATE_KEY_PATH")
	}
	if strings.TrimSpace(cfg.AlipayPublicKey) == "" && strings.TrimSpace(cfg.AlipayPublicKeyPath) == "" {
		missing = append(missing, "ALIPAY_PUBLIC_KEY or ALIPAY_PUBLIC_KEY_PATH")
	}
	return missing
}

func loadAlipayPrivateKey(cfg config.Config) (*rsa.PrivateKey, error) {
	raw := strings.TrimSpace(cfg.AlipayPrivateKey)
	if raw == "" && cfg.AlipayPrivateKeyPath != "" {
		data, err := os.ReadFile(cfg.AlipayPrivateKeyPath)
		if err != nil {
			return nil, err
		}
		raw = string(data)
	}
	raw = strings.ReplaceAll(raw, "\\n", "\n")
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		raw = "-----BEGIN PRIVATE KEY-----\n" + raw + "\n-----END PRIVATE KEY-----"
		block, _ = pem.Decode([]byte(raw))
	}
	if block == nil {
		return nil, fmt.Errorf("private key pem decode failed")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, fmt.Errorf("private key is not RSA")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

func loadAlipayPublicKey(cfg config.Config) (*rsa.PublicKey, error) {
	raw := strings.TrimSpace(cfg.AlipayPublicKey)
	if raw == "" && cfg.AlipayPublicKeyPath != "" {
		data, err := os.ReadFile(cfg.AlipayPublicKeyPath)
		if err != nil {
			return nil, err
		}
		raw = string(data)
	}
	raw = strings.ReplaceAll(raw, "\\n", "\n")
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		raw = "-----BEGIN PUBLIC KEY-----\n" + raw + "\n-----END PUBLIC KEY-----"
		block, _ = pem.Decode([]byte(raw))
	}
	if block == nil {
		return nil, fmt.Errorf("public key pem decode failed")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		if cert, certErr := x509.ParseCertificate(block.Bytes); certErr == nil {
			if rsaKey, ok := cert.PublicKey.(*rsa.PublicKey); ok {
				return rsaKey, nil
			}
		}
		return nil, err
	}
	rsaKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not RSA")
	}
	return rsaKey, nil
}

func buildAlipayPagePayForm(cfg config.Config, privateKey *rsa.PrivateKey, order alipayOrderForPayment, providerPaymentID string) (string, string, string, error) {
	gatewayURL := strings.TrimSpace(cfg.AlipayGatewayURL)
	if gatewayURL == "" {
		gatewayURL = "https://openapi-sandbox.dl.alipaydev.com/gateway.do"
	}
	payMethod := firstNonEmptyString(strings.TrimSpace(cfg.AlipayPayMethod), "alipay.trade.page.pay")
	productCode := firstNonEmptyString(strings.TrimSpace(cfg.AlipayProductCode), "FAST_INSTANT_TRADE_PAY")
	biz := map[string]string{
		"out_trade_no": providerPaymentID,
		"product_code": productCode,
		"total_amount": centsToAlipayAmount(order.AmountCents),
		"subject":      truncateAlipaySubject(order.Title),
	}
	if payMethod == "alipay.trade.wap.pay" && strings.TrimSpace(cfg.AlipayReturnURL) != "" {
		biz["quit_url"] = strings.TrimSpace(cfg.AlipayReturnURL)
	}
	bizContent, err := json.Marshal(biz)
	if err != nil {
		return "", "", "", err
	}
	params := map[string]string{
		"app_id":      cfg.AlipayAppID,
		"method":      payMethod,
		"format":      "JSON",
		"charset":     "utf-8",
		"sign_type":   "RSA2",
		"timestamp":   time.Now().Format("2006-01-02 15:04:05"),
		"version":     "1.0",
		"biz_content": string(bizContent),
	}
	if cfg.AlipayIncludeNotifyURL && cfg.AlipayNotifyURL != "" {
		params["notify_url"] = cfg.AlipayNotifyURL
	}
	if (cfg.AlipayIncludeReturnURL || payMethod == "alipay.trade.wap.pay") && cfg.AlipayReturnURL != "" {
		params["return_url"] = cfg.AlipayReturnURL
	}
	signature, err := signAlipayParams(params, privateKey)
	if err != nil {
		return "", "", "", err
	}
	params["sign"] = signature
	formMethod := http.MethodGet
	if payMethod == "alipay.trade.wap.pay" {
		formMethod = http.MethodPost
	}
	slog.Info("alipay_page_pay_form_built",
		"gateway_url", gatewayURL,
		"app_id", cfg.AlipayAppID,
		"method", payMethod,
		"out_trade_no", providerPaymentID,
		"amount", centsToAlipayAmount(order.AmountCents),
		"product_code", productCode,
		"has_notify_url", params["notify_url"] != "",
		"has_return_url", params["return_url"] != "",
		"has_quit_url", biz["quit_url"] != "",
		"return_url", params["return_url"],
	)
	return renderAutoSubmitForm(formMethod, gatewayURL, params), gatewayURL, formMethod, nil
}

func signAlipayParams(params map[string]string, privateKey *rsa.PrivateKey) (string, error) {
	canonical := alipayCanonicalString(params)
	digest := sha256.Sum256([]byte(canonical))
	sig, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

func verifyAlipayParams(params map[string]string, publicKey *rsa.PublicKey) bool {
	signature := params["sign"]
	if signature == "" {
		return false
	}
	canonical := alipayCanonicalString(params)
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	digest := sha256.Sum256([]byte(canonical))
	return rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], sig) == nil
}

func verifyAlipayFormSignature(values url.Values, publicKey *rsa.PublicKey) bool {
	params := make(map[string]string, len(values))
	for key, value := range values {
		if len(value) == 0 {
			continue
		}
		params[key] = value[0]
	}
	delete(params, "sign_type")
	return verifyAlipayParams(params, publicKey)
}

func verifyAlipayRawContent(rawContent string, signature string, publicKey *rsa.PublicKey) bool {
	if rawContent == "" || signature == "" {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	digest := sha256.Sum256([]byte(rawContent))
	return rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], sig) == nil
}

func alipayCanonicalString(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if key == "sign" || value == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+params[key])
	}
	return strings.Join(parts, "&")
}

func requestAlipayTradeQuery(ctx context.Context, cfg config.Config, privateKey *rsa.PrivateKey, publicKey *rsa.PublicKey, providerPaymentID string) (alipayGatewayResponse, error) {
	bizContent, err := json.Marshal(map[string]string{"out_trade_no": providerPaymentID})
	if err != nil {
		return alipayGatewayResponse{}, err
	}
	params := map[string]string{
		"app_id":      cfg.AlipayAppID,
		"method":      "alipay.trade.query",
		"format":      "JSON",
		"charset":     "utf-8",
		"sign_type":   "RSA2",
		"timestamp":   time.Now().Format("2006-01-02 15:04:05"),
		"version":     "1.0",
		"biz_content": string(bizContent),
	}
	signature, err := signAlipayParams(params, privateKey)
	if err != nil {
		return alipayGatewayResponse{}, err
	}
	params["sign"] = signature
	form := url.Values{}
	for key, value := range params {
		form.Set(key, value)
	}
	gatewayURL := strings.TrimSpace(cfg.AlipayGatewayURL)
	if gatewayURL == "" {
		gatewayURL = "https://openapi-sandbox.dl.alipaydev.com/gateway.do"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gatewayURL, strings.NewReader(form.Encode()))
	if err != nil {
		return alipayGatewayResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")
	client := http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return alipayGatewayResponse{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return alipayGatewayResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return alipayGatewayResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "alipay trade query http "+strconv.Itoa(resp.StatusCode), http.StatusBadGateway)
	}
	var parsed alipayGatewayResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return alipayGatewayResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "invalid alipay trade query response", http.StatusBadGateway)
	}
	signedContent, ok := extractAlipaySignedResponseContent(string(body), "alipay_trade_query_response")
	if !ok || !verifyAlipayRawContent(signedContent, parsed.Sign, publicKey) {
		return alipayGatewayResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "invalid alipay query signature", http.StatusBadGateway)
	}
	return parsed, nil
}

func extractAlipaySignedResponseContent(raw string, responseType string) (string, bool) {
	responseIndex := strings.Index(raw, `"`+responseType+`"`)
	if responseIndex < 0 {
		return "", false
	}
	start := strings.Index(raw[responseIndex:], "{")
	if start < 0 {
		return "", false
	}
	start += responseIndex
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return raw[start : i+1], true
			}
		}
	}
	return "", false
}

func isAlipayPaidStatus(status string) bool {
	return status == "TRADE_SUCCESS" || status == "TRADE_FINISHED"
}

func trustedAlipayWebhookSecret() string {
	return "trusted_alipay_rsa2_verified"
}

func signTrustedAlipayWebhook(providerEventID string, providerPaymentID string, orderID string) string {
	webhook := auction.ProviderPaymentWebhook{
		ProviderEventID:   providerEventID,
		ProviderPaymentID: providerPaymentID,
		OrderID:           orderID,
		EventType:         "payment_succeeded",
	}
	return auction.SignProviderWebhook(webhook, trustedAlipayWebhookSecret())
}

func renderAutoSubmitForm(method string, action string, params map[string]string) string {
	method = strings.ToLower(strings.TrimSpace(method))
	if method != "get" {
		method = "post"
	}
	formAction := action
	if method == "post" {
		if parsed, err := url.Parse(action); err == nil {
			query := parsed.Query()
			if query.Get("charset") == "" && strings.TrimSpace(params["charset"]) != "" {
				query.Set("charset", params["charset"])
				parsed.RawQuery = query.Encode()
				formAction = parsed.String()
			}
		}
	}
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>支付宝沙箱支付</title></head><body>`)
	b.WriteString(`<form id="alipay-submit" method="`)
	b.WriteString(method)
	b.WriteString(`" action="`)
	b.WriteString(html.EscapeString(formAction))
	b.WriteString(`">`)
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		b.WriteString(`<input type="hidden" name="`)
		b.WriteString(html.EscapeString(key))
		b.WriteString(`" value="`)
		b.WriteString(html.EscapeString(params[key]))
		b.WriteString(`">`)
	}
	b.WriteString(`</form><script>document.getElementById("alipay-submit").submit();</script></body></html>`)
	return b.String()
}

func centsToAlipayAmount(cents int64) string {
	if cents <= 0 {
		return "0.01"
	}
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

func truncateAlipaySubject(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "直播竞拍订单"
	}
	runes := []rune(subject)
	if len(runes) > 120 {
		return string(runes[:120])
	}
	return subject
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
