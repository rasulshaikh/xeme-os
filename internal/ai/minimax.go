// Package ai is the Xeme AI Engine — proprietary AI integration.
//
// Uses MiniMax's LLM API for personalization, research, and AI-augmented
// scoring. Falls back to local-only heuristics if the API is unreachable.
//
// This is a real AI layer, not a wrapper. The integration is direct HTTP
// to the MiniMax API; the prompts, context management, and output
// validation are all first-party.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type Engine struct {
	APIKey  string
	BaseURL string
	Model   string
	HTTP    *http.Client
}

const (
	defaultBaseURL = "https://api.MiniMax.chat/v1"
	defaultModel   = "MiniMax-M2"
)

func New() *Engine {
	apiKey := os.Getenv("XEME_AI_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("MINIMAX_API_KEY")
	}
	return &Engine{
		APIKey:  apiKey,
		BaseURL: getenv("XEME_AI_URL", defaultBaseURL),
		Model:   getenv("XEME_AI_MODEL", defaultModel),
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func (e *Engine) Health() error {
	if e.APIKey == "" {
		return fmt.Errorf("XEME_AI_KEY not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", e.BaseURL+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	resp, err := e.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("API status %d", resp.StatusCode)
	}
	return nil
}

type PersonalizeRequest struct {
	FirstName string
	LastName  string
	Title     string
	Company   string
	Industry  string
	Signal    string
	Context   string
}

func (e *Engine) Personalize(ctx context.Context, req PersonalizeRequest) (string, error) {
	if e.APIKey == "" {
		return e.localPersonalize(req), nil
	}
	prompt := buildPersonalizePrompt(req)
	resp, err := e.chat(ctx, []message{
		{Role: "system", Content: "You are a GTM copywriter. Output exactly one opening line of max 40 words, no preamble, no quotes, no Subject prefix. Reference a specific signal or context when available."},
		{Role: "user", Content: prompt},
	}, 0.8, 200)
	if err != nil {
		return e.localPersonalize(req), nil
	}
	return resp, nil
}

func buildPersonalizePrompt(r PersonalizeRequest) string {
	return fmt.Sprintf(`Write a single opening line for a cold email to %s %s, %s at %s (%s).

Public signal: %s
Additional context: %s

Rules:
- 1-2 sentences, max 40 words total
- Reference the signal specifically
- No I saw your post or generic openers
- Sound like a peer, not a salesperson`,
		r.FirstName, r.LastName, r.Title, r.Company, r.Industry, r.Signal, r.Context)
}

func (e *Engine) localPersonalize(r PersonalizeRequest) string {
	if r.Signal != "" {
		return fmt.Sprintf("Hi %s — saw %s, curious if thats a signal worth a quick conversation about how %s is approaching the same.", r.FirstName, truncate(r.Signal, 60), r.Company)
	}
	return fmt.Sprintf("Hi %s — your work at %s caught my attention. Worth a 15-min conversation?", r.FirstName, r.Company)
}

func (e *Engine) Research(ctx context.Context, query string) (string, error) {
	if e.APIKey == "" {
		return "Research unavailable: no API key configured (set XEME_AI_KEY)", nil
	}
	resp, err := e.chat(ctx, []message{
		{Role: "system", Content: "You are a B2B sales researcher. Given a query about a company or topic, return 2-3 concise sentences of public-knowledge context useful for sales outreach. Be specific. No preamble."},
		{Role: "user", Content: query},
	}, 0.3, 400)
	if err != nil {
		return "", err
	}
	return resp, nil
}

type Contact struct {
	Title    string
	Company  string
	Industry string
	Signal   string
	Email    string
	LinkedIn string
}

func (e *Engine) Score(ctx context.Context, lead Contact) (int, error) {
	if e.APIKey == "" {
		return 0, nil
	}
	prompt := fmt.Sprintf(`Score this lead for a B2B GTM target. Return ONLY a number 0-100, no explanation.

Title: %s
Company: %s
Industry: %s
Signal: %s
Email: %s
LinkedIn: %s

Scoring guidance:
- CMO/VP at 100-1000 person SaaS: 70-90
- SVP/Director at 50-500 person: 50-70
- Strong competitor-engagement signal: +10-20
- Generic title (Manager, Specialist): -20
- No email: -10`,
		lead.Title, lead.Company, lead.Industry, lead.Signal,
		boolStr(lead.Email != ""), boolStr(lead.LinkedIn != ""))
	resp, err := e.chat(ctx, []message{
		{Role: "system", Content: "You are an ICP scorer. Return only a single integer 0-100."},
		{Role: "user", Content: prompt},
	}, 0.1, 10)
	if err != nil {
		return 0, err
	}
	var score int
	_, _ = fmt.Sscanf(resp, "%d", &score)
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score, nil
}

func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
}

type chatResponse struct {
	Choices []struct {
		Message message `json:"message"`
	} `json:"choices"`
}

func (e *Engine) chat(ctx context.Context, msgs []message, temp float64, maxTokens int) (string, error) {
	body, _ := json.Marshal(chatRequest{
		Model:       e.Model,
		Messages:    msgs,
		Temperature: temp,
		MaxTokens:   maxTokens,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", e.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("API %d: %s", resp.StatusCode, string(raw))
	}
	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", err
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return cr.Choices[0].Message.Content, nil
}
