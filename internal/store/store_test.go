package store

import (
	"context"
	"errors"
	"fmt"
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

// TestURLByShortCodeRejectsNulByte guards against a NUL in the path segment
// reaching the query. Postgres rejects NUL in text, turning a plain miss into a
// query error and a 500; SQLite would match nothing. Both must read as
// not-found, since a stored code can never contain one.
func TestURLByShortCodeRejectsNulByte(t *testing.T) {
	ctx := context.Background()
	db := openLegacyFixture(t)

	for _, code := range []string{"\x00", "AB\x00CD", "ABC123\x00"} {
		_, err := db.URLByShortCode(ctx, code)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("URLByShortCode(%q) = %v, want ErrNotFound", code, err)
		}
	}
}

func TestUpdateURLAndPublishDraftIsAtomic(t *testing.T) {
	ctx := context.Background()
	db := openLegacyFixture(t)
	link := &URL{
		ShortCode: "ATOMIC", LongURL: "https://before.example/",
		IsDraft: true, IsEnabled: false,
	}
	if err := db.CreateURL(ctx, link); err != nil {
		t.Fatalf("CreateURL: %v", err)
	}

	trigger := fmt.Sprintf(`CREATE TRIGGER reject_draft_publication
		BEFORE UPDATE OF is_draft ON urls
		WHEN OLD.id = %d AND OLD.is_draft = 1 AND NEW.is_draft = 0
		BEGIN SELECT RAISE(ABORT, 'publication rejected'); END`, link.ID)
	if _, err := db.ExecContext(ctx, trigger); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	link.LongURL = "https://after.example/"
	link.IsDraft = false
	if err := db.UpdateURLAndPublishDraft(ctx, link); err == nil {
		t.Fatal("UpdateURLAndPublishDraft succeeded despite publication failure")
	}
	stored, err := db.URLByShortCode(ctx, link.ShortCode)
	if err != nil {
		t.Fatalf("reload rolled-back draft: %v", err)
	}
	if stored.LongURL != "https://before.example/" || !stored.IsDraft || stored.IsEnabled {
		t.Errorf("failed publication left partial state: URL=%q draft=%v enabled=%v",
			stored.LongURL, stored.IsDraft, stored.IsEnabled)
	}

	if _, err := db.ExecContext(ctx, "DROP TRIGGER reject_draft_publication"); err != nil {
		t.Fatalf("drop failure trigger: %v", err)
	}
	if err := db.UpdateURLAndPublishDraft(ctx, link); err != nil {
		t.Fatalf("successful UpdateURLAndPublishDraft: %v", err)
	}
	stored, err = db.URLByShortCode(ctx, link.ShortCode)
	if err != nil {
		t.Fatalf("reload published draft: %v", err)
	}
	if stored.LongURL != link.LongURL || stored.IsDraft || !stored.IsEnabled {
		t.Errorf("successful publication state: URL=%q draft=%v enabled=%v",
			stored.LongURL, stored.IsDraft, stored.IsEnabled)
	}
}
