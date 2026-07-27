package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestIsUniqueViolationOnDuplicateShortCode pins the driver-specific error
// shape that the API's 409 response depends on. If a driver upgrade changes it,
// duplicate custom codes would start returning 500 instead, and only this test
// would notice.
func TestIsUniqueViolationOnDuplicateShortCode(t *testing.T) {
	ctx := context.Background()
	db := openLegacyFixture(t)

	if err := db.CreateURL(ctx, &URL{ShortCode: "DUPE01", LongURL: "https://a.example.com/"}); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	err := db.CreateURL(ctx, &URL{ShortCode: "DUPE01", LongURL: "https://b.example.com/"})
	if err == nil {
		t.Fatal("second insert with the same short code succeeded; the unique index is missing")
	}
	if !IsUniqueViolation(err) {
		t.Errorf("IsUniqueViolation(%v) = false, want true", err)
	}
}

func TestIsUniqueViolationIgnoresOtherErrors(t *testing.T) {
	if IsUniqueViolation(nil) {
		t.Error("nil reported as a unique violation")
	}
	if IsUniqueViolation(errors.New("connection refused")) {
		t.Error("an unrelated error reported as a unique violation")
	}

	ctx := context.Background()
	db := openLegacyFixture(t)
	// A foreign-key failure is a constraint error, but not a unique one.
	err := db.RecordClick(ctx, &Click{URLID: 999999, Country: "France"})
	if err != nil && IsUniqueViolation(err) {
		t.Errorf("foreign-key failure %v reported as a unique violation", err)
	}
}

// TestNaiveUTCRelabelsRatherThanConverts guards the assumption that a driver
// handing back a zoned timestamp has labelled a stored naive-UTC value with the
// local zone. Converting the instant would shift every reading by the offset.
func TestNaiveUTCRelabelsRatherThanConverts(t *testing.T) {
	berlin := time.FixedZone("CEST", 2*60*60)
	got := naiveUTC(time.Date(2026, 5, 4, 12, 30, 45, 123456000, berlin))

	want := time.Date(2026, 5, 4, 12, 30, 45, 123456000, time.UTC)
	if !got.Equal(want) {
		t.Errorf("naiveUTC = %v, want %v (wall clock preserved, zone relabelled)", got, want)
	}
	if got.Location() != time.UTC {
		t.Errorf("naiveUTC returned location %v, want UTC", got.Location())
	}

	// Values already in UTC, or in a zero-offset zone, must pass through.
	utc := time.Date(2026, 5, 4, 12, 30, 45, 0, time.UTC)
	if !naiveUTC(utc).Equal(utc) {
		t.Error("a UTC value was altered")
	}
	zeroOffset := time.Date(2026, 5, 4, 12, 30, 45, 0, time.FixedZone("GMT", 0))
	if !naiveUTC(zeroOffset).Equal(utc) {
		t.Error("a zero-offset value was altered")
	}
}
