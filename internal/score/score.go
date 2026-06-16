// Package score implements the Xeme ICP Scorer — 7-gate scoring that
// produces a Tier classification (Hot / Warm / Nurture).
package score

import (
	"strings"
)

// Scorer applies the 7-gate ICP scoring rubric.
type Scorer struct {
	TitleCMO         int
	TitleVPMarketing int
	TitleOther       int
	SignalEvaluating int
	SignalCommenting int
	SignalFollowing  int
	EmailVerified    int
	Tier1Threshold   int
	Tier2Threshold   int
}

func New() *Scorer {
	return &Scorer{
		TitleCMO:         50,
		TitleVPMarketing: 40,
		TitleOther:       20,
		SignalEvaluating: 25,
		SignalCommenting: 20,
		SignalFollowing:  15,
		EmailVerified:    5,
		Tier1Threshold:   70,
		Tier2Threshold:   50,
	}
}

// Lead is a single contact row to score.
type Lead struct {
	FirstName string
	LastName  string
	Title     string
	Company   string
	Domain    string
	Email     string
	LinkedIn  string
	Signal    string
	Score     int
	Tier      string
}

// Score runs the 7-gate rubric against a single lead.
func (s *Scorer) Score(in Lead) Lead {
	out := in
	out.Score = 0

	// Gate 1: Title
	lower := strings.ToLower(in.Title)
	if strings.Contains(lower, "cmo") || strings.Contains(lower, "chief marketing") {
		out.Score += s.TitleCMO
	} else if strings.Contains(lower, "vp") && (strings.Contains(lower, "marketing") || strings.Contains(lower, "demand")) {
		out.Score += s.TitleVPMarketing
	} else {
		out.Score += s.TitleOther
	}

	// Gate 5: Signal strength
	sigLower := strings.ToLower(in.Signal)
	if strings.Contains(sigLower, "evaluating") || strings.Contains(sigLower, "job change") || strings.Contains(sigLower, "g2") || strings.Contains(sigLower, "capterra") {
		out.Score += s.SignalEvaluating
	} else if strings.Contains(sigLower, "commented") || strings.Contains(sigLower, "posted") || strings.Contains(sigLower, "engaged") {
		out.Score += s.SignalCommenting
	} else {
		out.Score += s.SignalFollowing
	}

	// Gate 6: Email deliverability bonus
	if strings.Contains(in.Email, "@") {
		out.Score += s.EmailVerified
	}

	if out.Score > 100 {
		out.Score = 100
	}
	out.Tier = s.Tier(out.Score)
	return out
}

// Tier returns the tier label for a given score.
func (s *Scorer) Tier(score int) string {
	switch {
	case score >= s.Tier1Threshold:
		return "Tier 1 - Hot"
	case score >= s.Tier2Threshold:
		return "Tier 2 - Warm"
	default:
		return "Tier 3 - Nurture"
	}
}

// Batch scores a list of leads and returns scored copies.
func (s *Scorer) Batch(leads []Lead) []Lead {
	out := make([]Lead, len(leads))
	for i, l := range leads {
		out[i] = s.Score(l)
	}
	return out
}

// Summary reports counts by tier.
type Summary struct {
	Total      int
	Tier1      int
	Tier2      int
	Tier3      int
	WithEmails int
}

func Summarize(leads []Lead) Summary {
	var sum Summary
	sum.Total = len(leads)
	for _, l := range leads {
		switch l.Tier {
		case "Tier 1 - Hot":
			sum.Tier1++
		case "Tier 2 - Warm":
			sum.Tier2++
		case "Tier 3 - Nurture":
			sum.Tier3++
		}
		if strings.Contains(l.Email, "@") {
			sum.WithEmails++
		}
	}
	return sum
}
