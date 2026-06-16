// Package aeo implements the Xeme AEO/GEO Engine — Answer Engine Optimization
// and Generative Engine Optimization.
//
// Traditional SEO targets Google. AEO/GEO targets the new AI search layer:
//   - ChatGPT, Claude, Gemini, Perplexity, Copilot
//   - Google AI Overviews / SGE
//   - Bing Copilot, You.com, Phind, Komo
//
// These engines don't rank pages — they synthesize answers from many sources.
// To get cited by them, you need:
//   1. Track which prompts your customers ask (the "query space")
//   2. See which AI engines mention your brand
//   3. Find which sources they cite
//   4. Identify the gaps — queries where competitors are mentioned but you aren't
//   5. Generate content optimized for citation (structured, factual, quotable)
//
// This package provides:
//   - Prompt mining (what are people asking?)
//   - Brand mention tracking across AI engines (free via scraping)
//   - Citation source analysis
//   - Content gap detection
//   - AEO score (0-100) per domain
package aeo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// AEOConfig holds the AEO engine configuration.
type AEOConfig struct {
	Brand       string   // Your brand name (e.g. "Xeme")
	Domain      string   // Your domain (e.g. "xeme.app")
	Competitors []string // Competitor brand names
	Prompts     []string // Prompts to track (e.g. "best GTM tool for AI agents")
	Engines     []string // AI engines to query — chatgpt, claude, perplexity, gemini, etc.
	Timeout     time.Duration
}

// DefaultAEOConfig returns a config from env vars.
func DefaultAEOConfig() *AEOConfig {
	return &AEOConfig{
		Brand:       os.Getenv("XEME_AEO_BRAND"),
		Domain:      os.Getenv("XEME_AEO_DOMAIN"),
		Competitors: splitCSV(os.Getenv("XEME_AEO_COMPETITORS")),
		Prompts:     splitCSV(os.Getenv("XEME_AEO_PROMPTS")),
		Engines:     []string{"perplexity", "you.com", "phind", "komo", "bing_copilot"},
		Timeout:     30 * time.Second,
	}
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Engine is the Xeme AEO/GEO Engine.
type Engine struct {
	Config *AEOConfig
	HTTP   *http.Client
}

// New creates a new AEO engine.
func New(cfg *AEOConfig) *Engine {
	if cfg == nil {
		cfg = DefaultAEOConfig()
	}
	return &Engine{
		Config: cfg,
		HTTP:   &http.Client{Timeout: cfg.Timeout},
	}
}

// Version returns the engine version.
func (e *Engine) Version() string {
	return "xeme-aeo v1.0.0"
}

// Health checks if the engine is operational.
func (e *Engine) Health(ctx context.Context) error {
	if e.Config.Brand == "" {
		return fmt.Errorf("XEME_AEO_BRAND not set")
	}
	return nil
}

// ── Result types ─────────────────────────────────────────────────────────────

// MentionResult captures whether/how a brand was mentioned by an AI engine.
type MentionResult struct {
	Brand        string  `json:"brand"`
	Domain       string  `json:"domain,omitempty"`
	Engine       string  `json:"engine"`
	Prompt       string  `json:"prompt"`
	Mentioned    bool    `json:"mentioned"`
	Position     int     `json:"position"`     // 0 = not mentioned, 1-10 = rank in response
	CitationURL  string  `json:"citation_url,omitempty"`
	CitationText string  `json:"citation_text,omitempty"`
	Sentiment    string  `json:"sentiment"`    // positive | neutral | negative
	Confidence   float64 `json:"confidence"`   // 0-1, how confident we are in the match
	CheckedAt    string  `json:"checked_at"`
	RawResponse  string  `json:"raw_response,omitempty"`
}

// CitationSource is a URL cited by an AI engine in its answer.
type CitationSource struct {
	URL       string  `json:"url"`
	Domain    string  `json:"domain"`
	Title     string  `json:"title,omitempty"`
	Snippet   string  `json:"snippet,omitempty"`
	Favicon   string  `json:"favicon,omitempty"`
	Engine    string  `json:"engine"`
	Prompt    string  `json:"prompt"`
	Brand     string  `json:"brand"`
	CheckedAt string  `json:"checked_at"`
}

// ContentGap identifies a query where competitors appear but you don't.
type ContentGap struct {
	Prompt            string   `json:"prompt"`
	MentionedBrands   []string `json:"mentioned_brands"`
	YourMentioned     bool     `json:"your_mentioned"`
	Opportunity       string   `json:"opportunity"` // high | medium | low
	RecommendedAction string   `json:"recommended_action"`
	WordCount         int      `json:"word_count_target"`
	ContentType       string   `json:"content_type"` // comparison, how-to, listicle, definition
}

// AEOScore is the overall AEO score for a brand.
type AEOScore struct {
	Brand             string  `json:"brand"`
	Domain            string  `json:"domain"`
	OverallScore      int     `json:"overall_score"`     // 0-100
	MentionRate       float64 `json:"mention_rate"`      // 0-1
	AvgPosition       float64 `json:"avg_position"`
	CitationRate      float64 `json:"citation_rate"`     // 0-1, % of prompts where cited
	SentimentScore    float64 `json:"sentiment_score"`   // -1 to 1
	EngineBreakdown   map[string]EngineScore `json:"engine_breakdown"`
	TopCitations      []string `json:"top_citations"`
	TopGaps           []ContentGap `json:"top_gaps"`
	CheckedAt         string  `json:"checked_at"`
}

// EngineScore is the per-engine AEO score.
type EngineScore struct {
	Engine       string  `json:"engine"`
	MentionRate  float64 `json:"mention_rate"`
	AvgPosition  float64 `json:"avg_position"`
	CitationRate float64 `json:"citation_rate"`
	Checks       int     `json:"checks"`
}

// ── Mention checking (free via scraping) ─────────────────────────────────────

// CheckMention asks an AI engine a prompt and analyzes the response for brand mentions.
// Free implementation: scrapes engines that have public endpoints (You.com, Phind, Komo).
// For engines that require API (ChatGPT, Claude, Perplexity API), use CheckMentionAPI.
func (e *Engine) CheckMention(ctx context.Context, engineName, prompt string) (*MentionResult, error) {
	switch engineName {
	case "you.com":
		return e.checkYouCom(ctx, prompt)
	case "phind":
		return e.checkPhind(ctx, prompt)
	case "komo":
		return e.checkKomo(ctx, prompt)
	case "bing_copilot":
		return e.checkBingCopilot(ctx, prompt)
	default:
		return nil, fmt.Errorf("unsupported engine: %s (use CheckMentionAPI for chatgpt/claude/perplexity)", engineName)
	}
}

func (e *Engine) checkYouCom(ctx context.Context, prompt string) (*MentionResult, error) {
	// You.com has a public chat endpoint
	apiURL := "https://chat-api.you.com/smart-chat"
	body := map[string]interface{}{
		"input":  prompt,
		"chat_style": "default",
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Xeme-OS/2.0 (AEO-Engine)")

	resp, err := e.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("you.com request: %w", err)
	}
	defer resp.Body.Close()

	raw := make([]byte, 1<<20)
	n, _ := resp.Body.Read(raw)
	text := string(raw[:n])

	result := &MentionResult{
		Brand:     e.Config.Brand,
		Domain:    e.Config.Domain,
		Engine:    "you.com",
		Prompt:    prompt,
		RawResponse: truncate(text, 500),
		CheckedAt: time.Now().Format(time.RFC3339),
	}

	// Analyze response for brand mention + citations
	result.Mentioned, result.Position, result.Sentiment = analyzeMention(text, e.Config.Brand, e.Config.Competitors)
	if citationURL := extractFirstCitation(text); citationURL != "" {
		result.CitationURL = citationURL
		result.CitationText = extractCitationSnippet(text, citationURL)
	}
	result.Confidence = 0.7

	return result, nil
}

func (e *Engine) checkPhind(ctx context.Context, prompt string) (*MentionResult, error) {
	// Phind has a public API (no key needed for basic usage)
	apiURL := "https://https.api.phind.com/infer/"
	body := map[string]interface{}{
		"question":          prompt,
		"codeModelContext":  "phind",
		"model":             "phind",
		"is_search_enabled": true,
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Xeme-OS/2.0 (AEO-Engine)")

	resp, err := e.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("phind request: %w", err)
	}
	defer resp.Body.Close()

	raw := make([]byte, 1<<20)
	n, _ := resp.Body.Read(raw)
	text := string(raw[:n])

	result := &MentionResult{
		Brand:     e.Config.Brand,
		Domain:    e.Config.Domain,
		Engine:    "phind",
		Prompt:    prompt,
		RawResponse: truncate(text, 500),
		CheckedAt: time.Now().Format(time.RFC3339),
	}
	result.Mentioned, result.Position, result.Sentiment = analyzeMention(text, e.Config.Brand, e.Config.Competitors)
	if citationURL := extractFirstCitation(text); citationURL != "" {
		result.CitationURL = citationURL
		result.CitationText = extractCitationSnippet(text, citationURL)
	}
	result.Confidence = 0.7
	return result, nil
}

func (e *Engine) checkKomo(ctx context.Context, prompt string) (*MentionResult, error) {
	// Komo has a public chat endpoint
	apiURL := "https://komo.ai/api/chat"
	body := map[string]interface{}{
		"content": prompt,
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Xeme-OS/2.0 (AEO-Engine)")

	resp, err := e.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("komo request: %w", err)
	}
	defer resp.Body.Close()

	raw := make([]byte, 1<<20)
	n, _ := resp.Body.Read(raw)
	text := string(raw[:n])

	result := &MentionResult{
		Brand:     e.Config.Brand,
		Domain:    e.Config.Domain,
		Engine:    "komo",
		Prompt:    prompt,
		RawResponse: truncate(text, 500),
		CheckedAt: time.Now().Format(time.RFC3339),
	}
	result.Mentioned, result.Position, result.Sentiment = analyzeMention(text, e.Config.Brand, e.Config.Competitors)
	if citationURL := extractFirstCitation(text); citationURL != "" {
		result.CitationURL = citationURL
		result.CitationText = extractCitationSnippet(text, citationURL)
	}
	result.Confidence = 0.65
	return result, nil
}

func (e *Engine) checkBingCopilot(ctx context.Context, prompt string) (*MentionResult, error) {
	// Bing Copilot is web-based; we'd need a headless browser.
	// For now, return a stub indicating manual check needed.
	return &MentionResult{
		Brand:     e.Config.Brand,
		Domain:    e.Config.Domain,
		Engine:    "bing_copilot",
		Prompt:    prompt,
		CheckedAt: time.Now().Format(time.RFC3339),
		RawResponse: "Bing Copilot requires browser automation — use potter_browser_* or playwright to check",
		Confidence: 0.0,
	}, nil
}

// CheckMentionAPI checks a paid AI engine (ChatGPT, Claude, Perplexity) — requires user-supplied API key.
func (e *Engine) CheckMentionAPI(ctx context.Context, engineName, prompt, apiKey string) (*MentionResult, error) {
	switch engineName {
	case "perplexity":
		return e.checkPerplexityAPI(ctx, prompt, apiKey)
	case "openai":
		return e.checkOpenAI(ctx, prompt, apiKey)
	case "anthropic":
		return e.checkAnthropic(ctx, prompt, apiKey)
	default:
		return nil, fmt.Errorf("unsupported API engine: %s", engineName)
	}
}

func (e *Engine) checkPerplexityAPI(ctx context.Context, prompt, apiKey string) (*MentionResult, error) {
	apiURL := "https://api.perplexity.ai/chat/completions"
	body := map[string]interface{}{
		"model": "sonar",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Xeme-OS/2.0 (AEO-Engine)")

	resp, err := e.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw := make([]byte, 1<<20)
	n, _ := resp.Body.Read(raw)

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Citations []string `json:"citations"`
	}
	if err := json.Unmarshal(raw[:n], &result); err != nil {
		return nil, fmt.Errorf("parse perplexity: %w", err)
	}

	answerText := ""
	if len(result.Choices) > 0 {
		answerText = result.Choices[0].Message.Content
	}

	mention := &MentionResult{
		Brand:     e.Config.Brand,
		Domain:    e.Config.Domain,
		Engine:    "perplexity",
		Prompt:    prompt,
		RawResponse: truncate(answerText, 500),
		CheckedAt: time.Now().Format(time.RFC3339),
	}
	mention.Mentioned, mention.Position, mention.Sentiment = analyzeMention(answerText, e.Config.Brand, e.Config.Competitors)
	if len(result.Citations) > 0 {
		mention.CitationURL = result.Citations[0]
	}
	mention.Confidence = 0.95
	return mention, nil
}

func (e *Engine) checkOpenAI(ctx context.Context, prompt, apiKey string) (*MentionResult, error) {
	apiURL := "https://api.openai.com/v1/chat/completions"
	body := map[string]interface{}{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw := make([]byte, 1<<20)
	n, _ := resp.Body.Read(raw)

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	_ = json.Unmarshal(raw[:n], &result)

	answerText := ""
	if len(result.Choices) > 0 {
		answerText = result.Choices[0].Message.Content
	}
	mention := &MentionResult{
		Brand:     e.Config.Brand,
		Domain:    e.Config.Domain,
		Engine:    "openai",
		Prompt:    prompt,
		RawResponse: truncate(answerText, 500),
		CheckedAt: time.Now().Format(time.RFC3339),
	}
	mention.Mentioned, mention.Position, mention.Sentiment = analyzeMention(answerText, e.Config.Brand, e.Config.Competitors)
	mention.Confidence = 0.9
	return mention, nil
}

func (e *Engine) checkAnthropic(ctx context.Context, prompt, apiKey string) (*MentionResult, error) {
	apiURL := "https://api.anthropic.com/v1/messages"
	body := map[string]interface{}{
		"model":      "claude-haiku-4-5-20251001",
		"max_tokens": 1024,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw := make([]byte, 1<<20)
	n, _ := resp.Body.Read(raw)

	var result struct {
		Content []struct {
			Text string `json:"text"`
			Type string `json:"type"`
		} `json:"content"`
	}
	_ = json.Unmarshal(raw[:n], &result)

	answerText := ""
	for _, c := range result.Content {
		if c.Type == "text" {
			answerText += c.Text
		}
	}
	mention := &MentionResult{
		Brand:     e.Config.Brand,
		Domain:    e.Config.Domain,
		Engine:    "anthropic",
		Prompt:    prompt,
		RawResponse: truncate(answerText, 500),
		CheckedAt: time.Now().Format(time.RFC3339),
	}
	mention.Mentioned, mention.Position, mention.Sentiment = analyzeMention(answerText, e.Config.Brand, e.Config.Competitors)
	mention.Confidence = 0.9
	return mention, nil
}

// ── Batch operations ─────────────────────────────────────────────────────────

// CheckBrand runs all configured prompts against all configured engines.
func (e *Engine) CheckBrand(ctx context.Context) ([]*MentionResult, error) {
	var results []*MentionResult
	for _, prompt := range e.Config.Prompts {
		for _, engine := range e.Config.Engines {
			result, err := e.CheckMention(ctx, engine, prompt)
			if err != nil {
				result = &MentionResult{
					Brand:     e.Config.Brand,
					Engine:    engine,
					Prompt:    prompt,
					CheckedAt: time.Now().Format(time.RFC3339),
					Sentiment: "error",
					RawResponse: err.Error(),
				}
			}
			results = append(results, result)
			// Be polite — small delay between requests
			time.Sleep(500 * time.Millisecond)
		}
	}
	return results, nil
}

// FindCitations extracts all citation URLs from a list of mention results.
func (e *Engine) FindCitations(results []*MentionResult) []CitationSource {
	var sources []CitationSource
	for _, r := range results {
		if r.CitationURL == "" {
			continue
		}
		domain := extractDomain(r.CitationURL)
		sources = append(sources, CitationSource{
			URL:       r.CitationURL,
			Domain:    domain,
			Snippet:   r.CitationText,
			Engine:    r.Engine,
			Prompt:    r.Prompt,
			Brand:     r.Brand,
			CheckedAt: r.CheckedAt,
		})
	}
	return sources
}

// FindGaps identifies content gaps — prompts where competitors are mentioned but you aren't.
func (e *Engine) FindGaps(results []*MentionResult) []ContentGap {
	promptMap := map[string][]*MentionResult{}
	for _, r := range results {
		promptMap[r.Prompt] = append(promptMap[r.Prompt], r)
	}

	var gaps []ContentGap
	for prompt, promptResults := range promptMap {
		mentionedBrands := []string{}
		yourMentioned := false
		for _, r := range promptResults {
			if r.Mentioned && r.Brand == e.Config.Brand {
				yourMentioned = true
			}
			if r.Mentioned {
				mentionedBrands = append(mentionedBrands, r.Brand)
			}
		}
		if yourMentioned {
			continue
		}
		gaps = append(gaps, ContentGap{
			Prompt:            prompt,
			MentionedBrands:   mentionedBrands,
			YourMentioned:     false,
			Opportunity:       "high",
			RecommendedAction: "Create a comparison page or definitive guide targeting this query",
			WordCount:         1500,
			ContentType:       suggestContentType(prompt),
		})
	}
	return gaps
}

// Score computes the overall AEO score from a set of mention results.
func (e *Engine) Score(results []*MentionResult) *AEOScore {
	score := &AEOScore{
		Brand:           e.Config.Brand,
		Domain:          e.Config.Domain,
		EngineBreakdown: map[string]EngineScore{},
		CheckedAt:       time.Now().Format(time.RFC3339),
	}

	engineStats := map[string]*struct {
		checks, mentions, cited int
		posSum                  float64
	}{}
	mentionedCount := 0
	citedCount := 0
	posTotal := 0
	posCount := 0
	sentimentSum := 0.0
	sentimentCount := 0

	for _, r := range results {
		// Update per-engine stats
		es, ok := engineStats[r.Engine]
		if !ok {
			es = &struct {
				checks, mentions, cited int
				posSum                  float64
			}{}
			engineStats[r.Engine] = es
		}
		es.checks++
		if r.Mentioned {
			es.mentions++
			mentionedCount++
			if r.Position > 0 {
				es.posSum += float64(r.Position)
				posTotal += r.Position
				posCount++
			}
		}
		if r.CitationURL != "" {
			es.cited++
			citedCount++
		}
		// Sentiment
		switch r.Sentiment {
		case "positive":
			sentimentSum += 1.0
			sentimentCount++
		case "negative":
			sentimentSum -= 1.0
			sentimentCount++
		case "neutral":
			sentimentCount++
		}
	}

	total := len(results)
	if total == 0 {
		return score
	}
	score.MentionRate = float64(mentionedCount) / float64(total)
	score.CitationRate = float64(citedCount) / float64(total)
	if posCount > 0 {
		score.AvgPosition = float64(posTotal) / float64(posCount)
	}
	if sentimentCount > 0 {
		score.SentimentScore = sentimentSum / float64(sentimentCount)
	}

	for engine, es := range engineStats {
		esScore := EngineScore{Engine: engine, Checks: es.checks}
		if es.checks > 0 {
			esScore.MentionRate = float64(es.mentions) / float64(es.checks)
			esScore.CitationRate = float64(es.cited) / float64(es.checks)
			if es.mentions > 0 {
				esScore.AvgPosition = es.posSum / float64(es.mentions)
			}
		}
		score.EngineBreakdown[engine] = esScore
	}

	// Overall 0-100 score: 50% mention rate + 30% citation rate + 20% position (inverse)
	mentionComponent := score.MentionRate * 50
	citationComponent := score.CitationRate * 30
	positionComponent := 0.0
	if score.AvgPosition > 0 {
		// Position 1 = 20pts, position 5 = 12pts, position 10 = 4pts
		positionComponent = (1.0 - (score.AvgPosition - 1.0) / 9.0) * 20
		if positionComponent < 0 {
			positionComponent = 0
		}
	}
	score.OverallScore = int(mentionComponent + citationComponent + positionComponent)

	// Top citations
	citations := e.FindCitations(results)
	domainCount := map[string]int{}
	for _, c := range citations {
		domainCount[c.Domain]++
	}
	for d := range domainCount {
		score.TopCitations = append(score.TopCitations, d)
	}

	// Top gaps
	score.TopGaps = e.FindGaps(results)

	return score
}

// ── Content optimization ───────────────────────────────────────────────────

// OptimizeContent analyzes a piece of content and suggests AEO/GEO improvements.
type ContentOptimization struct {
	URL              string  `json:"url"`
	WordCount        int     `json:"word_count"`
	HasFAQs          bool    `json:"has_faqs"`
	HasSchema        bool    `json:"has_schema_markup"`
	HasStats         bool    `json:"has_statistics"`
	HasQuotes        bool    `json:"has_quotes"`
	HasCitations     bool    `json:"has_citations"`
	ReadabilityScore float64 `json:"readability_score"` // 0-1
	Issues           []string `json:"issues"`
	Recommendations  []string `json:"recommendations"`
}

// Optimize checks a URL for AEO-readiness.
func (e *Engine) Optimize(ctx context.Context, targetURL string) (*ContentOptimization, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Xeme-OS/2.0 (AEO-Engine)")
	resp, err := e.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw := make([]byte, 1<<20)
	n, _ := resp.Body.Read(raw)
	html := string(raw[:n])

	opt := &ContentOptimization{
		URL:       targetURL,
		WordCount: countWords(stripHTML(html)),
	}

	opt.HasFAQs = hasFAQSection(html)
	opt.HasSchema = hasSchemaMarkup(html)
	opt.HasStats = hasStatistics(html)
	opt.HasQuotes = hasBlockquotes(html)
	opt.HasCitations = hasCitations(html)
	opt.ReadabilityScore = readabilityScore(html)

	// Issue detection
	if opt.WordCount < 300 {
		opt.Issues = append(opt.Issues, "Content too short — AI engines prefer 800+ words")
		opt.Recommendations = append(opt.Recommendations, "Expand to 800-2000 words with detailed examples")
	}
	if !opt.HasFAQs {
		opt.Issues = append(opt.Issues, "Missing FAQ section — AI engines cite FAQs heavily")
		opt.Recommendations = append(opt.Recommendations, "Add 5-10 FAQ items using FAQ schema markup")
	}
	if !opt.HasSchema {
		opt.Issues = append(opt.Issues, "No schema.org markup — engines can't parse structured data")
		opt.Recommendations = append(opt.Recommendations, "Add Article, FAQPage, HowTo, or Product schema")
	}
	if !opt.HasStats {
		opt.Issues = append(opt.Issues, "No statistics found — engines cite data points more than opinions")
		opt.Recommendations = append(opt.Recommendations, "Include 3-5 specific numbers (e.g. '3x more meetings, 60% cost reduction')")
	}
	if !opt.HasQuotes {
		opt.Issues = append(opt.Issues, "No expert quotes — engines attribute and cite named sources")
		opt.Recommendations = append(opt.Recommendations, "Include 2-3 expert quotes with clear attribution")
	}
	if opt.ReadabilityScore < 0.5 {
		opt.Issues = append(opt.Issues, "Low readability — AI engines prefer clear, direct writing")
		opt.Recommendations = append(opt.Recommendations, "Shorten sentences, use simpler words, add subheadings")
	}

	return opt, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────

// analyzeMention returns mentioned, position, sentiment by scanning the response text.
func analyzeMention(text, brand string, competitors []string) (mentioned bool, position int, sentiment string) {
	if text == "" || brand == "" {
		return false, 0, "neutral"
	}
	lower := strings.ToLower(text)
	brandLower := strings.ToLower(brand)

	// Find position in text
	idx := strings.Index(lower, brandLower)
	if idx == -1 {
		// Try alternate forms
		altForms := []string{
			strings.ToLower(brand + " os"),
			strings.ReplaceAll(strings.ToLower(brand), " ", ""),
		}
		for _, form := range altForms {
			if i := strings.Index(lower, form); i != -1 {
				idx = i
				mentioned = true
				break
			}
		}
	} else {
		mentioned = true
	}

	// Estimate position (1-10) based on order of appearance vs competitors
	if mentioned {
		positions := []int{idx}
		for _, comp := range competitors {
			compLower := strings.ToLower(comp)
			if ci := strings.Index(lower, compLower); ci != -1 {
				positions = append(positions, ci)
			}
		}
		// Sort to find rank
		for i := 0; i < len(positions); i++ {
			rank := 1
			for j := 0; j < len(positions); j++ {
				if i != j && positions[j] < positions[i] {
					rank++
				}
			}
			if rank > position {
				position = rank
			}
		}
	}

	// Sentiment (simple heuristic — look for nearby positive/negative words)
	if mentioned {
		// Look at the context around the brand mention (100 chars before/after)
		start := idx - 100
		if start < 0 {
			start = 0
		}
		end := idx + 100
		if end > len(text) {
			end = len(text)
		}
		context := strings.ToLower(text[start:end])
		posWords := []string{"best", "great", "excellent", "recommend", "powerful", "leading", "top", "popular"}
		negWords := []string{"bad", "poor", "broken", "issues", "fails", "expensive", "limited", "lacks"}
		posScore := 0
		negScore := 0
		for _, w := range posWords {
			if strings.Contains(context, w) {
				posScore++
			}
		}
		for _, w := range negWords {
			if strings.Contains(context, w) {
				negScore++
			}
		}
		if posScore > negScore {
			sentiment = "positive"
		} else if negScore > posScore {
			sentiment = "negative"
		} else {
			sentiment = "neutral"
		}
	}
	return
}

func extractFirstCitation(text string) string {
	// Look for URLs in the text
	re := regexp.MustCompile(`https?://[^\s\)\]\,"'<]+`)
	matches := re.FindStringSubmatch(text)
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

func extractCitationSnippet(text, citationURL string) string {
	idx := strings.Index(text, citationURL)
	if idx == -1 {
		return ""
	}
	start := idx + len(citationURL)
	if start+200 > len(text) {
		return text[start:]
	}
	return text[start : start+200]
}

func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}

func suggestContentType(prompt string) string {
	lower := strings.ToLower(prompt)
	switch {
	case strings.Contains(lower, "vs") || strings.Contains(lower, " vs "):
		return "comparison"
	case strings.Contains(lower, "how to") || strings.Contains(lower, "how do"):
		return "how-to"
	case strings.Contains(lower, "best") || strings.Contains(lower, "top"):
		return "listicle"
	case strings.Contains(lower, "what is") || strings.Contains(lower, "what are"):
		return "definition"
	default:
		return "guide"
	}
}

func countWords(s string) int {
	return len(strings.Fields(s))
}

func stripHTML(s string) string {
	re := regexp.MustCompile(`<[^>]+>`)
	return re.ReplaceAllString(s, " ")
}

func hasFAQSection(html string) bool {
	lower := strings.ToLower(html)
	return strings.Contains(lower, "faq") || strings.Contains(lower, "frequently asked")
}

func hasSchemaMarkup(html string) bool {
	return strings.Contains(html, "schema.org") || strings.Contains(html, "application/ld+json")
}

func hasStatistics(html string) bool {
	// Look for numbers in % or $ or with units
	re := regexp.MustCompile(`\b\d+(\.\d+)?%|\$\d+|\b\d+(\.\d+)?x\b`)
	return re.MatchString(html)
}

func hasBlockquotes(html string) bool {
	return strings.Contains(html, "<blockquote")
}

func hasCitations(html string) bool {
	re := regexp.MustCompile(`<cite|<a[^>]*rel="[^"]*nofollow"`)
	return re.MatchString(html)
}

func readabilityScore(html string) float64 {
	// Simple Flesch reading-ease proxy
	text := stripHTML(html)
	sentences := strings.Count(text, ".") + strings.Count(text, "!") + strings.Count(text, "?")
	if sentences == 0 {
		return 0.5
	}
	words := countWords(text)
	if words == 0 {
		return 0.5
	}
	avgWordsPerSentence := float64(words) / float64(sentences)
	// Lower is better — 15 words/sentence is ideal
	if avgWordsPerSentence <= 15 {
		return 1.0
	}
	if avgWordsPerSentence >= 30 {
		return 0.2
	}
	return 1.0 - (avgWordsPerSentence-15.0)/15.0*0.8
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}