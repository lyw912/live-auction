package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestChatCompletionsGeneratorListingDraftUsesStrictSchemaAndImages(t *testing.T) {
	var captured map[string]any
	gen, err := NewChatCompletionsGenerator(ChatProviderConfig{
		BaseURL:   "https://relay.example.test/v1",
		Model:     "model-a",
		APIKey:    "test-key",
		Timeout:   time.Second,
		MaxTokens: 1024,
	})
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}
	gen.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("auth header = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return chatJSONResponse(t, map[string]any{
			"title_candidates":    []string{"老银鎏金手镯"},
			"description":         "商家备注中的老银鎏金手镯，需补充证书和瑕疵照片后再展示。",
			"category":            "古董珠宝",
			"selling_points":      []string{"鎏金细节", "直播可展示纹理"},
			"condition_questions": []string{"是否有证书", "是否有修复痕迹"},
			"compliance_flags":    []string{"不得承诺真伪"},
			"requires_evidence":   []string{"证书", "瑕疵照片"},
			"unsupported_claims":  []string{},
			"confidence":          0.81,
			"rationale":           "基于图片和商家备注生成，发布前需人工核验。",
			"rule_suggestion": map[string]any{
				"start_price_cents":          10000,
				"increment_cents":            1000,
				"cap_price_cents":            80000,
				"duration_seconds":           180,
				"extend_window_seconds":      10,
				"extend_by_seconds":          10,
				"max_extend_count":           6,
				"fat_finger_threshold_cents": 30000,
			},
		}), nil
	})
	result, err := gen.GenerateStructured(context.Background(), StructuredRequest{
		Kind:          "listing_draft",
		PromptVersion: PromptVersionListingDraft,
		Input: map[string]any{
			"room_id":         "room_main",
			"seller_notes":    "老银鎏金手镯，有证书照片待补",
			"target_category": "古董珠宝",
			"image_urls":      []string{"https://cdn.example.test/item.jpg", "http://localhost/not-provider-fetchable.jpg"},
			"image_data_urls": []string{"data:image/png;base64,aGVsbG8=", "data:text/plain;base64,aGVsbG8="},
		},
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if result.Provider != "chat_completions_adapter" || result.Model != "model-a" {
		t.Fatalf("provider/model = %s/%s", result.Provider, result.Model)
	}
	if result.Output["description"] == "" {
		t.Fatalf("missing output: %#v", result.Output)
	}
	responseFormat := captured["response_format"].(map[string]any)
	if responseFormat["type"] != "json_schema" {
		t.Fatalf("response_format = %#v", responseFormat)
	}
	jsonSchema := responseFormat["json_schema"].(map[string]any)
	if jsonSchema["strict"] != true || jsonSchema["name"] != "listing_draft" {
		t.Fatalf("json_schema = %#v", jsonSchema)
	}
	messages := captured["messages"].([]any)
	user := messages[1].(map[string]any)
	content := user["content"].([]any)
	imageCount := 0
	textPayload := ""
	for _, part := range content {
		if part.(map[string]any)["type"] == "image_url" {
			imageCount++
		}
		if part.(map[string]any)["type"] == "text" {
			textPayload = part.(map[string]any)["text"].(string)
		}
	}
	if imageCount != 2 {
		t.Fatalf("image parts = %d, want HTTPS plus safe image data URL", imageCount)
	}
	if bytes.Contains([]byte(textPayload), []byte("aGVsbG8=")) {
		t.Fatalf("text payload should not include raw image data URL")
	}
}

func TestChatCompletionsGeneratorRejectsMalformedContent(t *testing.T) {
	gen, err := NewChatCompletionsGenerator(ChatProviderConfig{
		BaseURL: "https://relay.example.test/v1",
		Model:   "model-a",
		APIKey:  "test-key",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}
	gen.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString(`{"choices":[{"message":{"content":"not-json"}}]}`)),
			Request:    r,
		}, nil
	})
	_, err = gen.GenerateStructured(context.Background(), StructuredRequest{
		Kind:          "auction_commentary",
		PromptVersion: PromptVersionCommentary,
		Input: map[string]any{
			"auction_id":          "auc_1",
			"source_seq":          3,
			"event_type":          "bid_accepted",
			"current_price_cents": 12000,
		},
		Timeout: time.Second,
	})
	if err == nil {
		t.Fatal("expected malformed JSON error")
	}
}

func TestAnswerFromFactsCombinesMultipleRuleFacts(t *testing.T) {
	answer := AnswerFromFacts("auc_1", "起拍价和加价是多少", "测试拍品", "", map[string]any{
		"start_price_cents": int64(10000),
		"increment_cents":   int64(5000),
	})
	if answer.Answer != "每次加价 ¥50.00；起拍价 ¥100.00" {
		t.Fatalf("answer = %q", answer.Answer)
	}
	if len(answer.FactsUsed) != 2 || answer.FactsUsed[0] != "auction.increment_cents" || answer.FactsUsed[1] != "auction.start_price_cents" {
		t.Fatalf("facts = %#v", answer.FactsUsed)
	}
}

func TestChatCompletionsGeneratorSupportsSentinelAndProductQASchemas(t *testing.T) {
	kinds := []string{"sentinel_explanation", "product_qa"}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			schema, err := schemaForRequest(StructuredRequest{Kind: kind})
			if err != nil {
				t.Fatalf("schema: %v", err)
			}
			if schema.name != kind {
				t.Fatalf("schema name = %q", schema.name)
			}
			if schema.body["type"] != "object" {
				t.Fatalf("schema body = %#v", schema.body)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func chatJSONResponse(t *testing.T, value map[string]any) *http.Response {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}
	resp := map[string]any{
		"choices": []map[string]any{{
			"message": map[string]any{
				"content": string(content),
			},
		}},
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("encode response: %v", err)
	}
	header := make(http.Header)
	header.Set("Content-Type", "application/json")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     header,
		Body:       io.NopCloser(bytes.NewReader(raw)),
	}
}
