package sync

import (
	"strings"
	"testing"

	"github.com/brymastr/ynab-go"
)

func TestAdjustmentImportID(t *testing.T) {
	d := ynab.MustParseDate("2026-11-22")
	base := AdjustmentImportID("acct-1", d, 1000000, 1050000)

	if base != AdjustmentImportID("acct-1", d, 1000000, 1050000) {
		t.Fatal("not deterministic")
	}
	if !strings.HasPrefix(base, "QT:20261122:") {
		t.Fatalf("bad prefix: %s", base)
	}
	if len(base) != 28 || len(base) > ynab.MaxImportIDLen {
		t.Fatalf("bad length %d: %s", len(base), base)
	}
	cases := map[string]string{
		"account": AdjustmentImportID("acct-2", d, 1000000, 1050000),
		"date":    AdjustmentImportID("acct-1", ynab.MustParseDate("2026-11-23"), 1000000, 1050000),
		"old":     AdjustmentImportID("acct-1", d, 1000010, 1050000),
		"new":     AdjustmentImportID("acct-1", d, 1000000, 1050010),
	}
	for name, id := range cases {
		if id == base {
			t.Errorf("changing %s did not change ID", name)
		}
	}
}

func TestToCentMilliunits(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want ynab.Milliunits
	}{
		{"zero", 0, 0},
		{"plain", 123.45, 123450},
		{"rounds up", 100.006, 100010},
		{"rounds down", 100.004, 100000},
		{"negative", -50.25, -50250},
		{"negative rounds away from zero", -0.006, -10},
		{"float residue", 0.1 + 0.2, 300},
	}
	for _, tt := range tests {
		if got := ToCentMilliunits(tt.in); got != tt.want {
			t.Errorf("%s: ToCentMilliunits(%v) = %d, want %d", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestPlanAdjustment(t *testing.T) {
	tests := []struct {
		name             string
		ynabBal          ynab.Milliunits
		qBal             float64
		wantOld, wantNew ynab.Milliunits
		wantDelta        ynab.Milliunits
	}{
		{"increase", 1000000, 1050.5, 1000000, 1050500, 50500},
		{"decrease", 1000000, 900, 1000000, 900000, -100000},
		{"float residue equal", 300, 0.1 + 0.2, 300, 300, 0},
		{"sub-cent ynab milliunits", 1000004, 1000, 1000000, 1000000, 0},
		{"negative balances", -200000, -150.5, -200000, -150500, 49500},
	}
	for _, tt := range tests {
		o, n, d := PlanAdjustment(tt.ynabBal, tt.qBal)
		if o != tt.wantOld || n != tt.wantNew || d != tt.wantDelta {
			t.Errorf("%s: got (%d,%d,%d), want (%d,%d,%d)", tt.name, o, n, d, tt.wantOld, tt.wantNew, tt.wantDelta)
		}
	}
}

func TestIsDuplicateImportID(t *testing.T) {
	if IsDuplicateImportID(nil, "x") {
		t.Error("nil result")
	}
	res := &ynab.SaveResult{DuplicateImportIDs: []string{"a", "x"}}
	if !IsDuplicateImportID(res, "x") || IsDuplicateImportID(res, "y") {
		t.Error("wrong duplicate detection")
	}
}
