package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"

	"github.com/brymastr/ynab-go"
)

// ToCentMilliunits converts a dollar amount to YNAB milliunits, rounded to
// whole cents (so the result is always a multiple of 10). Rounding happens on
// the float value once, so float residue such as 0.1+0.2 != 0.3 disappears.
func ToCentMilliunits(dollars float64) ynab.Milliunits {
	return ynab.Milliunits(int64(math.Round(dollars*100)) * 10)
}

// RoundMilliunitsToCents rounds a milliunit amount to the nearest whole cent
// (half away from zero).
func RoundMilliunitsToCents(m ynab.Milliunits) ynab.Milliunits {
	return ynab.Milliunits(int64(math.Round(float64(m)/10)) * 10)
}

// PlanAdjustment computes the cent-rounded old balance (the YNAB account
// balance), new balance (the Questrade balance) and delta (new - old) used by
// every sync path. Sharing it keeps the paths from drifting apart. A zero delta
// means there is nothing to post.
func PlanAdjustment(ynabBalance ynab.Milliunits, questradeBalance float64) (oldBal, newBal, delta ynab.Milliunits) {
	oldBal = RoundMilliunitsToCents(ynabBalance)
	newBal = ToCentMilliunits(questradeBalance)
	return oldBal, newBal, newBal - oldBal
}

// AdjustmentImportID returns the YNAB import_id for a balance adjustment.
//
// Format: "QT:" + date as YYYYMMDD + ":" + the first 16 hex characters of
// sha256(accountID + "|" + YYYYMMDD + "|" + oldBal + "|" + newBal), with the
// balances as decimal milliunit integers. The result is 28 characters, within
// YNAB's 36 character limit (ynab.MaxImportIDLen).
//
// The ID identifies this specific adjustment: this YNAB account moving from
// balance X to balance Y on this date. Overlapping runs (scheduler plus a
// manual CLI run, or a retry after a timeout that actually succeeded) read the
// same balances and compute the same ID, so YNAB rejects the repeat instead of
// applying the delta twice. Any genuinely new balance change has a different
// old or new balance and therefore a different ID.
//
// This deliberately does not use YNAB's "YNAB:amount:date:occurrence" format:
// two legitimate adjustments of the same amount on one day would collide, and
// that format is reserved for bank-file imports. The account ID is hashed so
// identical balances in different accounts never share an ID (import_ids are
// per account in YNAB anyway, but this makes it explicit).
func AdjustmentImportID(accountID string, date ynab.Date, oldBal, newBal ynab.Milliunits) string {
	day := date.Time().Format("20060102")
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%d", accountID, day, int64(oldBal), int64(newBal))))
	return "QT:" + day + ":" + hex.EncodeToString(sum[:])[:16]
}

// containsString reports whether s is in list.
func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// IsDuplicateImportID reports whether YNAB's save result says importID already
// existed (HTTP 201 with duplicate_import_ids; not an error).
func IsDuplicateImportID(res *ynab.SaveResult, importID string) bool {
	return res != nil && containsString(res.DuplicateImportIDs, importID)
}
