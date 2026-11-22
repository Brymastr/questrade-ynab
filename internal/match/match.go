// Package match guesses which YNAB account each Questrade account should post
// to, from names alone. It's a suggestion engine: callers always show the
// result and let the user confirm before anything is saved.
package match

import (
	"sort"
	"strings"
)

// QuestradeAccount is the subset of a Questrade account the matcher needs.
type QuestradeAccount struct {
	Number string
	Type   string // "TFSA", "RRSP", "Margin", ...
}

// YNABAccount is the subset of a YNAB account the matcher needs.
type YNABAccount struct {
	ID   string
	Name string
	Type string // YNAB account type: "otherAsset", "checking", "creditCard", ...
}

// Suggestion is one proposed pairing, with the evidence behind it.
type Suggestion struct {
	QuestradeNumber string  `json:"questrade_account_number"`
	QuestradeType   string  `json:"questrade_type"`
	YNABAccountID   string  `json:"ynab_account_id"`
	YNABName        string  `json:"ynab_name"`
	Score           float64 `json:"score"`  // 0..1
	Reason          string  `json:"reason"` // human-readable evidence
}

// Threshold is the minimum score a pair needs to be suggested. It's set so a
// bare account-type match ("RRSP" appears in the YNAB name) is enough on its
// own; anything weaker isn't worth putting in front of the user.
const Threshold = 0.5

// Weights for each signal. They're additive and capped at 1.
const (
	wType      = 0.6  // YNAB name contains the Questrade account type
	wQuestrade = 0.25 // YNAB name contains "questrade"
	wNumber    = 0.4  // YNAB name contains the full account number
	wLast4     = 0.3  // YNAB name contains the last four digits as a token
	wAsset     = 0.05 // YNAB account is a tracking asset, the natural home
)

// typeAliases lets a YNAB name say "RSP" for an RRSP, and so on. Keys are
// lower-cased Questrade types; values are tokens that count as a match.
var typeAliases = map[string][]string{
	"rrsp":   {"rrsp", "rsp"},
	"tfsa":   {"tfsa"},
	"fhsa":   {"fhsa"},
	"resp":   {"resp"},
	"lira":   {"lira"},
	"lif":    {"lif"},
	"rrif":   {"rrif"},
	"margin": {"margin", "brokerage", "nonregistered", "non-registered", "taxable"},
	"cash":   {"cash"},
}

// liabilityTypes are YNAB account types an investment balance can never
// belong to. Pairs with these are never suggested.
var liabilityTypes = map[string]bool{
	"creditCard":     true,
	"lineOfCredit":   true,
	"otherLiability": true,
	"mortgage":       true,
	"autoLoan":       true,
	"studentLoan":    true,
	"personalLoan":   true,
	"medicalDebt":    true,
	"otherDebt":      true,
}

type candidate struct {
	qi, yi int
	score  float64
	reason string
}

// Suggest returns at most one suggestion per Questrade account and per YNAB
// account, chosen greedily by score. Accounts with no confident match are
// simply absent from the result. The result is ordered by score, descending.
func Suggest(qts []QuestradeAccount, ynabs []YNABAccount) []Suggestion {
	var cands []candidate
	for qi, q := range qts {
		for yi, y := range ynabs {
			if s, reason := score(q, y); s >= Threshold {
				cands = append(cands, candidate{qi, yi, s, reason})
			}
		}
	}

	// Highest score first; ties broken by YNAB name so the output is stable.
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].score != cands[j].score {
			return cands[i].score > cands[j].score
		}
		return ynabs[cands[i].yi].Name < ynabs[cands[j].yi].Name
	})

	usedQ := make(map[int]bool, len(qts))
	usedY := make(map[int]bool, len(ynabs))
	var out []Suggestion
	for i, c := range cands {
		if usedQ[c.qi] || usedY[c.yi] {
			continue
		}
		// A tie in either direction is ambiguous: two YNAB accounts that fit
		// this Questrade account equally well, or two Questrade accounts that
		// fit this YNAB account equally well. Don't guess; leave it for the
		// user. Retiring both sides keeps the tied pairs from being picked up
		// later by elimination.
		if ambiguous(cands, i, usedQ, usedY) {
			usedQ[c.qi] = true
			usedY[c.yi] = true
			continue
		}
		usedQ[c.qi] = true
		usedY[c.yi] = true
		q, y := qts[c.qi], ynabs[c.yi]
		out = append(out, Suggestion{
			QuestradeNumber: q.Number,
			QuestradeType:   q.Type,
			YNABAccountID:   y.ID,
			YNABName:        y.Name,
			Score:           c.score,
			Reason:          c.reason,
		})
	}
	return out
}

// ambiguous reports whether cands[i] ties with another unused pair that
// shares either its Questrade account or its YNAB account.
func ambiguous(cands []candidate, i int, usedQ, usedY map[int]bool) bool {
	c := cands[i]
	for j := i + 1; j < len(cands) && cands[j].score == c.score; j++ {
		o := cands[j]
		if o.qi == c.qi && o.yi != c.yi && !usedY[o.yi] {
			return true
		}
		if o.yi == c.yi && o.qi != c.qi && !usedQ[o.qi] {
			return true
		}
	}
	return false
}

// score rates one pair and explains the rating.
func score(q QuestradeAccount, y YNABAccount) (float64, string) {
	if liabilityTypes[y.Type] {
		return 0, ""
	}

	toks := tokens(y.Name)
	has := func(t string) bool {
		_, ok := toks[t]
		return ok
	}

	var s float64
	var reasons []string

	qType := strings.ToLower(strings.TrimSpace(q.Type))
	aliases := typeAliases[qType]
	if len(aliases) == 0 && qType != "" {
		aliases = []string{qType}
	}
	for _, a := range aliases {
		if has(a) {
			s += wType
			reasons = append(reasons, "name contains "+q.Type)
			break
		}
	}

	if has("questrade") {
		s += wQuestrade
		reasons = append(reasons, "name contains Questrade")
	}

	if n := strings.TrimSpace(q.Number); n != "" {
		if has(n) {
			s += wNumber
			reasons = append(reasons, "name contains account number")
		} else if len(n) >= 4 && has(n[len(n)-4:]) {
			s += wLast4
			reasons = append(reasons, "name contains last 4 digits")
		}
	}

	if y.Type == "otherAsset" {
		s += wAsset
	}

	if s > 1 {
		s = 1
	}
	return s, strings.Join(reasons, ", ")
}

// tokens lower-cases a name and splits it on anything that isn't a letter or
// digit, returning the set of words. "Questrade TFSA – 1234" becomes
// {questrade, tfsa, 1234}.
func tokens(name string) map[string]struct{} {
	out := make(map[string]struct{})
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out[b.String()] = struct{}{}
			b.Reset()
		}
	}
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}
