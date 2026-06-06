package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ChatProviderConfig struct {
	BaseURL   string
	Model     string
	APIKey    string
	Timeout   time.Duration
	MaxTokens int
}

type ChatCompletionsGenerator struct {
	cfg    ChatProviderConfig
	client *http.Client
}

func NewChatCompletionsGenerator(cfg ChatProviderConfig) (ChatCompletionsGenerator, error) {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.Model = strings.TrimSpace(cfg.Model)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	if cfg.BaseURL == "" || cfg.Model == "" || cfg.APIKey == "" {
		return ChatCompletionsGenerator{}, ErrDisabled
	}
	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ChatCompletionsGenerator{}, fmt.Errorf("invalid AI relay base url")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 45 * time.Second
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 2048
	}
	return ChatCompletionsGenerator{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}, nil
}

func (g ChatCompletionsGenerator) GenerateStructured(ctx context.Context, req StructuredRequest) (StructuredResult, error) {
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	schema, err := schemaForRequest(req)
	if err != nil {
		return StructuredResult{}, err
	}
	payload := map[string]any{
		"model":       g.cfg.Model,
		"messages":    chatMessagesForRequest(req),
		"temperature": temperatureForKind(req.Kind),
		"max_tokens":  g.cfg.MaxTokens,
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   schema.name,
				"strict": true,
				"schema": schema.body,
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return StructuredResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.cfg.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return StructuredResult{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+g.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(httpReq)
	if err != nil {
		return StructuredResult{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return StructuredResult{}, fmt.Errorf("AI relay status %d: %s", resp.StatusCode, cleanText(string(body), 180))
	}
	content, err := extractChatContent(body)
	if err != nil {
		return StructuredResult{}, err
	}
	output := map[string]any{}
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		return StructuredResult{}, fmt.Errorf("AI relay returned invalid JSON: %w", err)
	}
	if len(output) == 0 {
		return StructuredResult{}, errors.New("AI relay returned empty JSON")
	}
	return StructuredResult{
		Provider: "chat_completions_adapter",
		Model:    g.cfg.Model,
		Output:   output,
		Safety: map[string]any{
			"provider_mode":         "relay_chat_completions_json_schema",
			"prompt_version":        req.PromptVersion,
			"schema":                schema.name,
			"human_review_required": req.Kind == "listing_draft",
			"no_auto_publish":       req.Kind == "listing_draft",
		},
	}, nil
}

type strictSchema struct {
	name string
	body map[string]any
}

func schemaForRequest(req StructuredRequest) (strictSchema, error) {
	switch req.Kind {
	case "listing_draft":
		return strictSchema{name: "listing_draft", body: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required": []string{
				"title_candidates", "description", "category", "selling_points", "condition_questions",
				"compliance_flags", "requires_evidence", "unsupported_claims", "confidence", "rationale", "rule_suggestion",
			},
			"properties": map[string]any{
				"title_candidates":    stringArraySchema(1, 3),
				"description":         stringSchema(1, 360),
				"category":            stringSchema(1, 40),
				"selling_points":      stringArraySchema(1, 5),
				"condition_questions": stringArraySchema(1, 5),
				"compliance_flags":    stringArraySchema(0, 6),
				"requires_evidence":   stringArraySchema(0, 6),
				"unsupported_claims":  stringArraySchema(0, 6),
				"confidence":          map[string]any{"type": "number", "minimum": 0, "maximum": 1},
				"rationale":           stringSchema(1, 160),
				"rule_suggestion": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required": []string{
						"start_price_cents", "increment_cents", "cap_price_cents", "duration_seconds",
						"extend_window_seconds", "extend_by_seconds", "max_extend_count", "fat_finger_threshold_cents",
					},
					"properties": map[string]any{
						"start_price_cents":          integerSchema(0, 100000000),
						"increment_cents":            integerSchema(100, 10000000),
						"cap_price_cents":            integerSchema(0, 100000000),
						"duration_seconds":           integerSchema(30, 7200),
						"extend_window_seconds":      integerSchema(5, 60),
						"extend_by_seconds":          integerSchema(5, 60),
						"max_extend_count":           integerSchema(0, 30),
						"fat_finger_threshold_cents": integerSchema(0, 100000000),
					},
				},
			},
		}}, nil
	case "auction_commentary":
		return strictSchema{name: "auction_commentary", body: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"auction_id", "source_seq", "style", "body", "facts_used", "safety_labels"},
			"properties": map[string]any{
				"auction_id":    stringSchema(1, 80),
				"source_seq":    integerSchema(0, 1<<53-1),
				"style":         enumSchema([]string{"steady", "heat", "critical", "sold", "calm"}),
				"body":          stringSchema(1, 40),
				"facts_used":    stringArraySchema(1, 8),
				"safety_labels": stringArraySchema(0, 8),
			},
		}}, nil
	default:
		return strictSchema{}, fmt.Errorf("unsupported AI schema kind %q", req.Kind)
	}
}

func chatMessagesForRequest(req StructuredRequest) []map[string]any {
	system := "你是直播竞拍系统的受控助手。只输出符合 JSON schema 的 JSON，不要输出 Markdown。不得编造观众人数、折扣、库存、鉴定结论、隐藏最高价、用户隐私或竞拍结果。竞拍裁判只来自系统事实。"
	userText := "根据以下已审核输入生成结构化结果。"
	if req.Kind == "listing_draft" {
		userText = "为商家生成拍品草稿。必须标出需要人工核验的证据，不得承诺真伪、年代、升值或收益。"
	}
	if req.Kind == "auction_commentary" {
		userText = "生成一条不超过40字的直播系统解说。只能引用输入事实，不得诱导冲动消费，不得制造假紧迫。"
	}
	content := []map[string]any{{"type": "text", "text": userText + "\n输入 JSON:\n" + mustJSON(req.Input)}}
	if req.Kind == "listing_draft" {
		for _, imageURL := range stringSlice(req.Input["image_urls"]) {
			if isProviderFetchableHTTPS(imageURL) {
				content = append(content, map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": imageURL,
					},
				})
			}
		}
	}
	return []map[string]any{
		{"role": "system", "content": system},
		{"role": "user", "content": content},
	}
}

func extractChatContent(raw []byte) (string, error) {
	var payload struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	if len(payload.Choices) == 0 {
		return "", errors.New("AI relay returned no choices")
	}
	content := payload.Choices[0].Message.Content
	switch value := content.(type) {
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return "", errors.New("AI relay returned empty content")
		}
		return value, nil
	case []any:
		var builder strings.Builder
		for _, part := range value {
			partMap, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := partMap["text"].(string); ok {
				builder.WriteString(text)
			}
		}
		out := strings.TrimSpace(builder.String())
		if out == "" {
			return "", errors.New("AI relay returned empty content parts")
		}
		return out, nil
	default:
		return "", errors.New("AI relay returned unsupported content")
	}
}

func stringSchema(min int, max int) map[string]any {
	return map[string]any{"type": "string", "minLength": min, "maxLength": max}
}

func stringArraySchema(min int, max int) map[string]any {
	return map[string]any{
		"type":     "array",
		"minItems": min,
		"maxItems": max,
		"items":    stringSchema(1, 80),
	}
}

func integerSchema(min int64, max int64) map[string]any {
	return map[string]any{"type": "integer", "minimum": min, "maximum": max}
}

func enumSchema(values []string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

func temperatureForKind(kind string) float64 {
	if kind == "auction_commentary" {
		return 0.4
	}
	return 0.2
}

func mustJSON(value any) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func isProviderFetchableHTTPS(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Scheme == "https" && parsed.Host != ""
}
