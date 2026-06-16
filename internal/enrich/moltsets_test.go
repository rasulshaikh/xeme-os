package enrich

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMoltSetsEngine_SearchCompanies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		if r.Header.Get("Authorization") == "" {
			http.Error(w, `{"error":"missing auth"}`, 401)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, `{"error":"bad content-type"}`, 400)
			return
		}
		json := `{
			"results": {
				"results": [
					{
						"name": "MoltSets",
						"domain": "moltsets.com",
						"industry": "Information Technology",
						"employee_range": "51-200",
						"revenue_range": "$10M - $20M",
						"linkedin_url": "https://linkedin.com/company/moltsets",
						"_score": 15.23
					}
				],
				"total": 1
			},
			"status": "ok",
			"metadata": {"tokens_charged": 1, "tokens_remaining": 999}
		}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(json))
	}))
	defer server.Close()

	cfg := &MoltSetsConfig{
		APIKey:  "ms_test_key_123",
		BaseURL: server.URL + "/",
	}
	engine := NewMoltSetsEngine(cfg)

	result, tokens, err := engine.SearchCompanies(context.Background(), SearchCompanyParams{
		Domain:  "moltsets.com",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("SearchCompanies failed: %v", err)
	}
	if len(result.Results.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results.Results))
	}
	if result.Results.Results[0].Name != "MoltSets" {
		t.Fatalf("expected name MoltSets, got %s", result.Results.Results[0].Name)
	}
	if result.Results.Results[0].Domain != "moltsets.com" {
		t.Fatalf("expected domain moltsets.com, got %s", result.Results.Results[0].Domain)
	}
	if tokens != 1 {
		t.Fatalf("expected 1 token charged, got %f", tokens)
	}
}

func TestMoltSetsEngine_EnrichEmail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			http.Error(w, `{"error":"missing auth"}`, 401)
			return
		}
		json := `{
			"results": {
				"email": "adam@moltsets.com",
				"type": "business",
				"score": "high",
				"linkedin": "https://linkedin.com/in/adam"
			},
			"status": "ok",
			"metadata": {"tokens_charged": 1, "tokens_remaining": 998}
		}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(json))
	}))
	defer server.Close()

	cfg := &MoltSetsConfig{
		APIKey:  "ms_test_key_123",
		BaseURL: server.URL + "/",
	}
	engine := NewMoltSetsEngine(cfg)

	result, _, err := engine.EnrichEmail(context.Background(), "https://linkedin.com/in/adam")
	if err != nil {
		t.Fatalf("EnrichEmail failed: %v", err)
	}
	if result.Results.Email != "adam@moltsets.com" {
		t.Fatalf("expected email adam@moltsets.com, got %s", result.Results.Email)
	}
	if result.Results.Type != "business" {
		t.Fatalf("expected type business, got %s", result.Results.Type)
	}
}

func TestMoltSetsEngine_EnrichPhone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			http.Error(w, `{"error":"missing auth"}`, 401)
			return
		}
		json := `{
			"results": {
				"phone": "+1-555-123-4567",
				"carrier": "T-Mobile",
				"validated": "true",
				"linkedin": "https://linkedin.com/in/adam"
			},
			"status": "ok",
			"metadata": {"tokens_charged": 1, "tokens_remaining": 997}
		}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(json))
	}))
	defer server.Close()

	cfg := &MoltSetsConfig{
		APIKey:  "ms_test_key_123",
		BaseURL: server.URL + "/",
	}
	engine := NewMoltSetsEngine(cfg)

	result, _, err := engine.EnrichPhone(context.Background(), EnrichPhoneParams{
		LinkedInURL: "https://linkedin.com/in/adam",
	})
	if err != nil {
		t.Fatalf("EnrichPhone failed: %v", err)
	}
	if result.Results.Phone != "+1-555-123-4567" {
		t.Fatalf("expected phone +1-555-123-4567, got %s", result.Results.Phone)
	}
	if result.Results.Carrier != "T-Mobile" {
		t.Fatalf("expected carrier T-Mobile, got %s", result.Results.Carrier)
	}
}

func TestMoltSetsEngine_Health_NoKey(t *testing.T) {
	cfg := &MoltSetsConfig{APIKey: ""}
	engine := NewMoltSetsEngine(cfg)
	err := engine.Health(context.Background())
	if err == nil {
		t.Fatal("expected error when API key is empty")
	}
}

func TestMoltSetsEngine_SearchPeople(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			http.Error(w, `{"error":"missing auth"}`, 401)
			return
		}
		json := `{
			"results": {
				"results": [
					{
						"full_name": "Adam Smith",
						"first_name": "Adam",
						"last_name": "Smith",
						"title": "CEO",
						"email": "adam@moltsets.com",
						"phone": "+15551234567",
						"linkedin_url": "https://linkedin.com/in/adam",
						"company": "MoltSets",
						"domain": "moltsets.com",
						"industry": "Information Technology",
						"location": "San Francisco, CA",
						"_score": 20.5
					}
				],
				"total": 1
			},
			"status": "ok",
			"metadata": {"tokens_charged": 1, "tokens_remaining": 996}
		}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(json))
	}))
	defer server.Close()

	cfg := &MoltSetsConfig{
		APIKey:  "ms_test_key_123",
		BaseURL: server.URL + "/",
	}
	engine := NewMoltSetsEngine(cfg)

	result, _, err := engine.SearchPeople(context.Background(), SearchPeopleParams{
		CompanyDomain: "moltsets.com",
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("SearchPeople failed: %v", err)
	}
	if len(result.Results.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results.Results))
	}
	if result.Results.Results[0].FullName != "Adam Smith" {
		t.Fatalf("expected full_name 'Adam Smith', got %s", result.Results.Results[0].FullName)
	}
	if result.Results.Results[0].Email != "adam@moltsets.com" {
		t.Fatalf("expected email adam@moltsets.com, got %s", result.Results.Results[0].Email)
	}
	if result.Results.Results[0].Phone != "+15551234567" {
		t.Fatalf("expected phone +15551234567, got %s", result.Results.Results[0].Phone)
	}
}

func TestMoltSetsEngine_CostCap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json := `{"results": {"results": []}, "status": "ok", "metadata": {"tokens_charged": 1}}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(json))
	}))
	defer server.Close()

	cfg := &MoltSetsConfig{
		APIKey:     "ms_test_key_123",
		BaseURL:     server.URL + "/",
		CostCapUSD: 0.01, // very low cap
	}
	engine := NewMoltSetsEngine(cfg)

	// First call should work
	_, _, err := engine.SearchCompanies(context.Background(), SearchCompanyParams{})
	if err != nil {
		t.Fatalf("first call failed unexpectedly: %v", err)
	}
	// TokensUsed is now at ~1, which exceeds 0.01 USD cap
	// Second call should fail with cost cap
	_, _, err = engine.SearchCompanies(context.Background(), SearchCompanyParams{})
	if err == nil {
		t.Fatal("expected cost cap error on second call, got nil")
	}
}

func TestMoltSetsEngine_GetAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			http.Error(w, `{"error":"missing auth"}`, 401)
			return
		}
		json := `{
			"email": "adam@moltsets.com",
			"plan": "pro",
			"status": "active",
			"created_at": "2024-01-01T00:00:00Z",
			"metadata": {"tokens_remaining": 10000}
		}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(json))
	}))
	defer server.Close()

	cfg := &MoltSetsConfig{
		APIKey:  "ms_test_key_123",
		BaseURL: server.URL + "/",
	}
	engine := NewMoltSetsEngine(cfg)

	account, _, err := engine.GetAccount(context.Background())
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if account.Email != "adam@moltsets.com" {
		t.Fatalf("expected email adam@moltsets.com, got %s", account.Email)
	}
	if account.Plan != "pro" {
		t.Fatalf("expected plan pro, got %s", account.Plan)
	}
	if account.Metadata.TokensRemaining != 10000 {
		t.Fatalf("expected 10000 tokens remaining, got %f", account.Metadata.TokensRemaining)
	}
}
