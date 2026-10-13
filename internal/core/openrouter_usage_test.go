package core

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type usageTransport func(*http.Request) (*http.Response, error)

func (f usageTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOpenRouterAccountUsage(t *testing.T) {
	for _, test := range []struct {
		body    string
		status  int
		balance *float64
		message string
	}{
		{`{"data":{"total_credits":100.5,"total_usage":25.75}}`, 200, ptr(74.75), ""},
		{`{"data":{"total_credits":0,"total_usage":0}}`, 200, ptr(0), ""},
		{`{"data":{"total_credits":1,"total_usage":2}}`, 200, ptr(-1), ""},
		{`{"data":{"total_credits":100}}`, 200, nil, "incomplete"},
		{`{"data":{"total_credits":null,"total_usage":0}}`, 200, nil, "incomplete"},
		{`{"data":{"total_credits":-1,"total_usage":0}}`, 200, nil, "invalid"},
		{`not json`, 200, nil, "incomplete"},
		{`{"error":"secret-fixture-key"}`, 401, nil, "rejected"},
		{`{"error":"secret-fixture-key"}`, 403, nil, "management key"},
		{``, 429, nil, "rate limiting"},
		{``, 500, nil, "could not report"},
		{``, 302, nil, "could not report"},
	} {
		t.Run(fmt.Sprintf("%d-%s", test.status, test.body), func(t *testing.T) {
			calls := 0
			transport := usageTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() == "https://openrouter.ai/api/v1/activity" {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[]}`)), Header: http.Header{}}, nil
				}
				if r.Method != "GET" || r.URL.String() != "https://openrouter.ai/api/v1/credits" || r.Header.Get("Authorization") != "Bearer secret-fixture-key" || r.Body != nil {
					t.Fatal("unexpected account query")
				}
				return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body)), Header: http.Header{"Location": []string{"https://example.com/leak"}}}, nil
			})
			result, err := readOpenRouterUsage(context.Background(), "secret-fixture-key", transport)
			expected := 1
			if test.balance != nil {
				expected = 2
			}
			if calls != expected {
				t.Fatal("redirect followed")
			}
			if test.balance != nil {
				if err != nil || result.Balance == nil || *result.Balance != *test.balance || !result.Configured || result.CheckedAt == 0 {
					t.Fatalf("unexpected totals: %+v %v", result, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.message) || strings.Contains(err.Error(), "secret-fixture-key") || result.Balance != nil {
				t.Fatalf("unsafe or incorrect error: %+v %v", result, err)
			}
		})
	}
	for _, key := range []string{"", "a\nb", "a\x00b", "a b", strings.Repeat("a", 2049)} {
		if ValidateOpenRouterKey(key) == nil {
			t.Fatal("invalid key accepted")
		}
	}
	if ValidateOpenRouterKey("sk-or-v1-fixture") != nil {
		t.Fatal("valid key rejected")
	}
	transport := usageTransport(func(r *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("network failure with secret-fixture-key")
	})
	if _, err := readOpenRouterUsage(context.Background(), "secret-fixture-key", transport); err == nil || strings.Contains(err.Error(), "secret-fixture-key") {
		t.Fatal("transport details leaked")
	}
}

func ptr(value float64) *float64 { return &value }

func TestOpenRouterModelActivity(t *testing.T) {
	body := `{"data":[
 {"date":"2026-10-01","endpoint_id":"a","model":"provider/model-a","usage":2,"requests":2,"prompt_tokens":100,"completion_tokens":20,"reasoning_tokens":5},
 {"date":"2026-10-02","endpoint_id":"b","model":"provider/model-a","usage":3,"byok_usage_inference":1.25,"requests":3,"prompt_tokens":200,"completion_tokens":30,"reasoning_tokens":10},
 {"model":"provider/model-b","usage":8,"requests":1,"prompt_tokens":50,"completion_tokens":10},
 {"model_permaslug":"provider/model-c","usage":5,"requests":0,"prompt_tokens":0,"completion_tokens":0}
 ]}`
	transport := usageTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.String() != "https://openrouter.ai/api/v1/activity" || r.Header.Get("Authorization") != "Bearer secret-fixture-key" || r.Body != nil {
			t.Fatal("unexpected activity query")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	models, from, to, err := readOpenRouterActivity(context.Background(), "secret-fixture-key", transport)
	if err != nil || len(models) != 3 || models[0].Model != "provider/model-b" || models[1].Model != "provider/model-a" || models[2].Model != "provider/model-c" {
		t.Fatalf("unexpected model aggregation: %+v %v", models, err)
	}
	a := models[1]
	if a.Usage != 5 || a.BYOK != 1.25 || a.Requests != 5 || a.PromptTokens != 300 || a.CompletionTokens != 50 || a.ReasoningTokens != 15 {
		t.Fatalf("incorrect sums across endpoints/days: %+v", a)
	}
	start, _ := time.Parse(time.DateOnly, from)
	end, _ := time.Parse(time.DateOnly, to)
	if end.Sub(start) != 29*24*time.Hour || to != time.Now().UTC().AddDate(0, 0, -1).Format(time.DateOnly) {
		t.Fatal("period must include 30 complete UTC days")
	}
	for _, test := range []struct {
		body      string
		status    int
		wantError bool
	}{
		{`{"data":[]}`, 200, false},
		{`{"data":null}`, 200, true},
		{`{"data":[{"model":"missing-fields"}]}`, 200, true},
		{`{"data":[{"model":"a","usage":-1,"requests":1,"prompt_tokens":0,"completion_tokens":0}]}`, 200, true},
		{`{"data":[{"model":"a","usage":1,"requests":1,"prompt_tokens":9007199254740991,"completion_tokens":1}]}`, 200, true},
		{`not json`, 200, true},
		{`{"error":"secret-fixture-key"}`, 403, true},
		{``, 302, true},
	} {
		body = test.body
		calls := 0
		client := usageTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Location": []string{"https://example.com/leak"}}}, nil
		})
		values, _, _, err := readOpenRouterActivity(context.Background(), "secret-fixture-key", client)
		if calls != 1 || (err != nil) != test.wantError || (err != nil && strings.Contains(err.Error(), "secret-fixture-key")) || (!test.wantError && (values == nil || len(values) != 0)) {
			t.Fatalf("incorrect activity response: %+v %v", values, err)
		}
	}
	// Activity outages must not discard valid credits or prevent connecting a valid key.
	client := usageTransport(func(r *http.Request) (*http.Response, error) {
		status, body := 200, `{"data":{"total_credits":20,"total_usage":5}}`
		if r.URL.Path == "/api/v1/activity" {
			status, body = 500, `{"error":"secret-fixture-key"}`
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	account, err := readOpenRouterUsage(context.Background(), "secret-fixture-key", client)
	if err != nil || account.Balance == nil || *account.Balance != 15 || account.ActivityMessage == "" || account.Models != nil || strings.Contains(account.ActivityMessage, "secret-fixture-key") {
		t.Fatalf("activity failure lost valid account totals: %+v %v", account, err)
	}
}
