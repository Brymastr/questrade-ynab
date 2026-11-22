package match

import "testing"

func TestSuggestPicksNamedQuestradeAccounts(t *testing.T) {
	qts := []QuestradeAccount{
		{Number: "26684748", Type: "Margin"},
		{Number: "51884608", Type: "TFSA"},
		{Number: "52226742", Type: "RRSP"},
		{Number: "52970275", Type: "FHSA"},
	}
	ynabs := []YNABAccount{
		{ID: "visa", Name: "Scotia Momentum Visa Infinite – 1016", Type: "creditCard"},
		{ID: "chq", Name: "Ultimate Package – 5321", Type: "checking"},
		{ID: "tchq", Name: "Tangerine Chequing", Type: "checking"},
		{ID: "tsav", Name: "Tangerine Savings", Type: "savings"},
		{ID: "loc", Name: "ScotiaLine Line Of Credit – 7010", Type: "lineOfCredit"},
		{ID: "qmargin", Name: "Questrade Margin", Type: "otherAsset"},
		{ID: "qtfsa", Name: "Questrade TFSA", Type: "otherAsset"},
		{ID: "qrrsp", Name: "Questrade RRSP", Type: "otherAsset"},
		{ID: "qfhsa", Name: "Questrade FHSA", Type: "otherAsset"},
		{ID: "rivian", Name: "Rivian RRSP", Type: "otherAsset"},
		{ID: "espp", Name: "Etrade ESPP", Type: "otherAsset"},
		{ID: "rsu", Name: "Etrade RSU", Type: "otherAsset"},
	}

	got := Suggest(qts, ynabs)
	want := map[string]string{
		"26684748": "qmargin",
		"51884608": "qtfsa",
		"52226742": "qrrsp",
		"52970275": "qfhsa",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d suggestions, want %d: %+v", len(got), len(want), got)
	}
	for _, s := range got {
		if want[s.QuestradeNumber] != s.YNABAccountID {
			t.Errorf("%s (%s) → %s, want %s", s.QuestradeNumber, s.QuestradeType, s.YNABAccountID, want[s.QuestradeNumber])
		}
		if s.Score < Threshold {
			t.Errorf("%s scored %.2f, below threshold", s.QuestradeNumber, s.Score)
		}
		if s.Reason == "" {
			t.Errorf("%s has no reason", s.QuestradeNumber)
		}
	}
}

func TestSuggestPrefersQuestradeOverOtherBrokerage(t *testing.T) {
	qts := []QuestradeAccount{{Number: "1", Type: "RRSP"}}
	ynabs := []YNABAccount{
		{ID: "rivian", Name: "Rivian RRSP", Type: "otherAsset"},
		{ID: "q", Name: "Questrade RRSP", Type: "otherAsset"},
	}
	got := Suggest(qts, ynabs)
	if len(got) != 1 || got[0].YNABAccountID != "q" {
		t.Fatalf("got %+v, want the Questrade RRSP", got)
	}
}

func TestSuggestUsesAccountNumber(t *testing.T) {
	qts := []QuestradeAccount{{Number: "51884608", Type: "TFSA"}}
	ynabs := []YNABAccount{
		{ID: "a", Name: "TFSA", Type: "otherAsset"},
		{ID: "b", Name: "TFSA – 4608", Type: "otherAsset"},
	}
	got := Suggest(qts, ynabs)
	if len(got) != 1 || got[0].YNABAccountID != "b" {
		t.Fatalf("got %+v, want the account with the matching digits", got)
	}
}

func TestSuggestSkipsAmbiguousTies(t *testing.T) {
	qts := []QuestradeAccount{{Number: "1", Type: "RRSP"}}
	ynabs := []YNABAccount{
		{ID: "a", Name: "RRSP one", Type: "otherAsset"},
		{ID: "b", Name: "RRSP two", Type: "otherAsset"},
	}
	if got := Suggest(qts, ynabs); len(got) != 0 {
		t.Fatalf("got %+v, want no suggestion for a tie", got)
	}
}

func TestSuggestOneToOne(t *testing.T) {
	qts := []QuestradeAccount{
		{Number: "1", Type: "RRSP"},
		{Number: "2", Type: "RRSP"},
	}
	ynabs := []YNABAccount{{ID: "q", Name: "Questrade RRSP", Type: "otherAsset"}}
	got := Suggest(qts, ynabs)
	// Both Questrade accounts want the same YNAB account and tie, so neither
	// is suggested rather than picking one arbitrarily.
	if len(got) != 0 {
		t.Fatalf("got %+v, want no suggestion when two accounts tie for one target", got)
	}
}

func TestSuggestNeverPicksLiabilities(t *testing.T) {
	qts := []QuestradeAccount{{Number: "1", Type: "Margin"}}
	ynabs := []YNABAccount{{ID: "loc", Name: "Questrade Margin Loan", Type: "lineOfCredit"}}
	if got := Suggest(qts, ynabs); len(got) != 0 {
		t.Fatalf("got %+v, want nothing for a liability", got)
	}
}

func TestSuggestNothingBelowThreshold(t *testing.T) {
	qts := []QuestradeAccount{{Number: "1", Type: "TFSA"}}
	ynabs := []YNABAccount{{ID: "q", Name: "Questrade", Type: "otherAsset"}}
	if got := Suggest(qts, ynabs); len(got) != 0 {
		t.Fatalf("got %+v, want nothing for a brand-only match", got)
	}
}
