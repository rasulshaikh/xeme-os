// Mock Xeme upstream servers for testing the live paths.
// Spins up three HTTP servers on localhost:
//
//   :4901  Xeme Signal      (XEME_SIGNAL_DSN)
//   :4902  Xeme Enrichment  (XEME_ENRICH_BASE_URL)
//   :4903  Xeme CRM/Ledger  (XEME_CRM_URL)
//
// Each server logs every request to stdout so the test can verify
// the live paths were actually exercised.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

var (
	signalHits  atomic.Int64
	enrichHits  atomic.Int64
	crmHits     atomic.Int64
	startTime   = time.Now()
	requestLog  = make(chan string, 100)
)

func main() {
	go func() {
		for line := range requestLog {
			fmt.Println("[mock]", line)
		}
	}()

	muxSignal := http.NewServeMux()
	muxSignal.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		log1("SIGNAL", "GET", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "service": "xeme-signal-mock"})
	})
	muxSignal.HandleFunc("/api/v1/linkedin/posts/", func(w http.ResponseWriter, r *http.Request) {
		signalHits.Add(1)
		log1("SIGNAL", r.Method, r.URL.Path)
		// Parse post ID from path
		// /api/v1/linkedin/posts/{id}/engagers
		engagers := []map[string]interface{}{
			{
				"actor": map[string]interface{}{
					"name":  "Live Test 1",
					"title": "VP Marketing",
					"url":   "https://linkedin.com/in/live-test-1",
				},
				"type": "liker",
			},
			{
				"actor": map[string]interface{}{
					"name":  "Live Test 2",
					"title": "Chief Marketing Officer",
					"url":   "https://linkedin.com/in/live-test-2",
				},
				"type": "commenter",
			},
			{
				"actor": map[string]interface{}{
					"name":  "Live Test 3",
					"title": "Director of Demand Generation",
					"url":   "https://linkedin.com/in/live-test-3",
				},
				"type": "liker",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items": engagers,
			"total": len(engagers),
		})
	})

	muxEnrich := http.NewServeMux()
	muxEnrich.HandleFunc("/v1/health", func(w http.ResponseWriter, r *http.Request) {
		log1("ENRICH", "GET", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "service": "xeme-enrich-mock"})
	})
	muxEnrich.HandleFunc("/v1/balance", func(w http.ResponseWriter, r *http.Request) {
		log1("ENRICH", "GET", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"balance": 487.32, "currency": "XEME"})
	})
	muxEnrich.HandleFunc("/v1/enrich/person", func(w http.ResponseWriter, r *http.Request) {
		enrichHits.Add(1)
		body, _ := io.ReadAll(r.Body)
		log1("ENRICH", "POST", r.URL.Path+"  body="+string(body))
		var req struct {
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
			Company   string `json:"company_name"`
			Domain    string `json:"domain"`
			LinkedIn  string `json:"linkedin_url"`
		}
		json.Unmarshal(body, &req)

		// Synthesize a "live" enrichment result
		domain := req.Domain
		if domain == "" && req.Company != "" {
			domain = req.Company + ".com"
		}
		var email string
		if domain != "" {
			first := req.FirstName
			last := req.LastName
			if first != "" && last != "" {
				email = first + "." + last + "@" + domain
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"email":            email,
			"phone":            "+1-555-" + fmt.Sprintf("%04d", enrichHits.Load()%10000),
			"first_name":       req.FirstName,
			"last_name":        req.LastName,
			"full_name":        req.FirstName + " " + req.LastName,
			"title":            "Head of Marketing", // pretend we found one
			"company":          req.Company,
			"domain":           domain,
			"linkedin_url":     req.LinkedIn,
			"confidence":       0.91,
			"provider":         "xeme-enrich-mock",
			"credits_used":     0.04,
			"verified_email":   true,
		})
	})

	muxCRM := http.NewServeMux()
	muxCRM.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		crmHits.Add(1)
		body, _ := io.ReadAll(r.Body)
		log1("CRM", "POST", r.URL.Path+"  body="+truncate(string(body), 200))

		var req struct {
			Query     string                 `json:"query"`
			Variables map[string]interface{} `json:"variables"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		switch {
		case contains(req.Query, "__typename"):
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{"__typename": "Query"},
			})
		case contains(req.Query, "createXemeContact"):
			data, _ := req.Variables["data"].(map[string]interface{})
			first, _ := data["name"].(map[string]interface{})
			firstName, _ := first["firstName"].(string)
			id := fmt.Sprintf("xeme_live_%d", crmHits.Load())
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"createXemeContact": map[string]interface{}{
						"id":         id,
						"firstName":  firstName,
						"createdAt":  time.Now().UTC().Format(time.RFC3339),
						"__typename": "XemeContact",
					},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"data": nil})
		}
	})

	go http.ListenAndServe(":4901", muxSignal)
	go http.ListenAndServe(":4902", muxEnrich)
	go http.ListenAndServe(":4903", muxCRM)

	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  Xeme Mock Upstream Servers — running")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  Xeme Signal      :4901/api/v1/...")
	fmt.Println("  Xeme Enrich      :4902/v1/...")
	fmt.Println("  Xeme CRM/Ledger  :4903/graphql")
	fmt.Println()
	fmt.Println("Set env vars to use these in xeme:")
	fmt.Println("  export XEME_SIGNAL_API_KEY=test-signal-key")
	fmt.Println("  export XEME_SIGNAL_DSN=http://localhost:4901")
	fmt.Println("  export XEME_ENRICH_API_KEY=test-enrich-key")
	fmt.Println("  export XEME_ENRICH_BASE_URL=http://localhost:4902")
	fmt.Println()
	fmt.Println("Hit counts so far: signal=0 enrich=0 crm=0")
	fmt.Println()

	// Periodically print hit counts
	go func() {
		for range time.Tick(5 * time.Second) {
			uptime := time.Since(startTime).Round(time.Second)
			fmt.Printf("[mock stats] uptime=%v signal=%d enrich=%d crm=%d\n",
				uptime, signalHits.Load(), enrichHits.Load(), crmHits.Load())
		}
	}()

	// Block forever
	select {}
}

func log1(service, method, path string) {
	requestLog <- fmt.Sprintf("%-6s %s %s", service, method, path)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// Print stats on SIGINT
func init() {
	c := make(chan os.Signal, 1)
	go func() {
		<-c
		fmt.Printf("\n[mock final] signal=%d enrich=%d crm=%d\n",
			signalHits.Load(), enrichHits.Load(), crmHits.Load())
		os.Exit(0)
	}()
}
