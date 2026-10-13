package core

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Only account metrics cross the bridge; the management key stays native.
type OpenRouterAccount struct {
	Configured      bool              `json:"configured"`
	Balance         *float64          `json:"balance"`
	Credits         *float64          `json:"credits"`
	Usage           *float64          `json:"usage"`
	CheckedAt       int64             `json:"checkedAt,omitempty"`
	Message         string            `json:"message,omitempty"`
	Models          []OpenRouterModel `json:"models"`
	ActivityFrom    string            `json:"activityFrom,omitempty"`
	ActivityTo      string            `json:"activityTo,omitempty"`
	ActivityMessage string            `json:"activityMessage,omitempty"`
}

func ValidateOpenRouterKey(key string) error {
	if key == "" || len(key) > 2048 || strings.IndexFunc(key, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
		return errors.New("Enter a valid OpenRouter management key.")
	}
	return nil
}

func ReadOpenRouterUsage(ctx context.Context, key string) (OpenRouterAccount, error) {
	return readOpenRouterUsage(ctx, key, nil)
}

func readOpenRouterUsage(ctx context.Context, key string, transport http.RoundTripper) (OpenRouterAccount, error) {
	result := OpenRouterAccount{Configured: true}
	var data struct {
		Data *struct {
			Credits *float64 `json:"total_credits"`
			Usage   *float64 `json:"total_usage"`
		} `json:"data"`
	}
	if err := openRouterJSON(ctx, key, "credits", transport, &data); err != nil {
		return result, err
	}
	if data.Data == nil || data.Data.Credits == nil || data.Data.Usage == nil {
		return result, errors.New("OpenRouter returned incomplete account totals.")
	}
	credits, usage := *data.Data.Credits, *data.Data.Usage
	if credits < 0 || usage < 0 || math.IsNaN(credits) || math.IsNaN(usage) || math.IsInf(credits, 0) || math.IsInf(usage, 0) {
		return result, errors.New("OpenRouter returned invalid account totals.")
	}
	balance := credits - usage // A negative balance is meaningful; do not clamp it.
	result.Credits, result.Usage, result.Balance = &credits, &usage, &balance
	result.CheckedAt = time.Now().Unix()
	models, from, to, err := readOpenRouterActivity(ctx, key, transport)
	result.Models, result.ActivityFrom, result.ActivityTo = models, from, to
	if err != nil {
		result.ActivityMessage = "Could not load model usage. " + err.Error()
	}
	return result, nil
}

func openRouterJSON(ctx context.Context, key, path string, transport http.RoundTripper, target any) error {
	if err := ValidateOpenRouterKey(key); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openrouter.ai/api/v1/"+path, nil)
	if err != nil {
		return errors.New("Could not prepare the OpenRouter usage check.")
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return errors.New("Could not reach OpenRouter. Try refreshing again.")
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return errors.New("OpenRouter rejected this key. Check it and try again.")
	case http.StatusForbidden:
		return errors.New("A management key is required to read account usage.")
	case http.StatusTooManyRequests:
		return errors.New("OpenRouter is rate limiting usage checks. Try again later.")
	default:
		return errors.New("OpenRouter could not report usage. Try refreshing again.")
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(target); err != nil {
		return errors.New("OpenRouter returned incomplete usage data.")
	}
	return nil
}

type OpenRouterModel struct {
	Model            string  `json:"model"`
	Usage            float64 `json:"usage"`
	BYOK             float64 `json:"byok"`
	Requests         int64   `json:"requests"`
	PromptTokens     int64   `json:"promptTokens"`
	CompletionTokens int64   `json:"completionTokens"`
	ReasoningTokens  int64   `json:"reasoningTokens"`
}

func readOpenRouterActivity(ctx context.Context, key string, transport http.RoundTripper) ([]OpenRouterModel, string, string, error) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	from, to := today.AddDate(0, 0, -30).Format(time.DateOnly), today.AddDate(0, 0, -1).Format(time.DateOnly)
	var response struct {
		Data *[]struct {
			Model            string   `json:"model"`
			ModelPermaslug   string   `json:"model_permaslug"`
			Usage            *float64 `json:"usage"`
			BYOK             float64  `json:"byok_usage_inference"`
			Requests         *int64   `json:"requests"`
			PromptTokens     *int64   `json:"prompt_tokens"`
			CompletionTokens *int64   `json:"completion_tokens"`
			ReasoningTokens  int64    `json:"reasoning_tokens"`
		} `json:"data"`
	}
	if err := openRouterJSON(ctx, key, "activity", transport, &response); err != nil {
		return nil, from, to, err
	}
	if response.Data == nil {
		return nil, from, to, errors.New("OpenRouter returned incomplete model activity.")
	}
	models := map[string]OpenRouterModel{}
	for _, row := range *response.Data {
		name := strings.TrimSpace(row.Model)
		if name == "" {
			name = strings.TrimSpace(row.ModelPermaslug)
		}
		if name == "" || row.Usage == nil || row.Requests == nil || row.PromptTokens == nil || row.CompletionTokens == nil {
			return nil, from, to, errors.New("OpenRouter returned incomplete model activity.")
		}
		if *row.Usage < 0 || math.IsInf(*row.Usage, 0) || row.BYOK < 0 || math.IsInf(row.BYOK, 0) || *row.Requests < 0 || *row.PromptTokens < 0 || *row.CompletionTokens < 0 || row.ReasoningTokens < 0 {
			return nil, from, to, errors.New("OpenRouter returned invalid model activity.")
		}
		model := models[name]
		model.Model = name
		model.Usage += *row.Usage
		model.BYOK += row.BYOK
		// Bound sums before adding untrusted counts; JSON numbers must also stay exact in JS.
		const maxCount int64 = 9007199254740991
		for _, pair := range [][2]int64{{model.Requests, *row.Requests}, {model.PromptTokens, *row.PromptTokens}, {model.CompletionTokens, *row.CompletionTokens}, {model.ReasoningTokens, row.ReasoningTokens}} {
			if pair[1] > maxCount-pair[0] {
				return nil, from, to, errors.New("OpenRouter returned invalid model activity.")
			}
		}
		model.Requests += *row.Requests
		model.PromptTokens += *row.PromptTokens
		model.CompletionTokens += *row.CompletionTokens
		model.ReasoningTokens += row.ReasoningTokens
		if math.IsInf(model.Usage, 0) || math.IsInf(model.BYOK, 0) || model.PromptTokens > maxCount-model.CompletionTokens {
			return nil, from, to, errors.New("OpenRouter returned invalid model activity.")
		}
		models[name] = model
	}
	result := make([]OpenRouterModel, 0, len(models))
	for _, model := range models {
		result = append(result, model)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Usage == result[j].Usage {
			return result[i].Model < result[j].Model
		}
		return result[i].Usage > result[j].Usage
	})
	return result, from, to, nil
}
