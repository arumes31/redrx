package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arumes31/redrx/internal/security"
)

// openLegacyFixture copies testdata/legacy_python.db — written by the Flask /
// SQLAlchemy application this service replaces — and opens the copy, so tests
// can write without mutating the committed fixture.
func openLegacyFixture(t *testing.T) *DB {
	t.Helper()

	src, err := os.ReadFile(filepath.Join("testdata", "legacy_python.db"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "legacy.db")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}

	db, err := Open(context.Background(), "sqlite:///"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Migrate first, exactly as run() does before the listener binds. Columns
	// this build added after the Python release only exist afterwards, so a
	// query issued against the raw fixture is testing a state the service never
	// serves from. TestMigrateLeavesLegacyDataIntact covers the migration
	// itself preserving every row.
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate fixture: %v", err)
	}
	return db
}

func TestMigrateLeavesLegacyDataIntact(t *testing.T) {
	ctx := context.Background()
	db := openLegacyFixture(t)

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate on an existing Python database: %v", err)
	}

	for table, want := range map[string]int{"users": 2, "urls": 3, "clicks": 5} {
		var n int
		if err := db.QueryRow(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != want {
			t.Errorf("%s row count = %d after migrate, want %d", table, n, want)
		}
	}
}

func TestReadsLegacyUsersAndVerifiesTheirPasswords(t *testing.T) {
	ctx := context.Background()
	db := openLegacyFixture(t)

	cases := []struct{ login, password string }{
		{"alice", "alice-password"},         // stored as scrypt
		{"bob@example.com", "bob-password"}, // stored as pbkdf2
	}
	for _, tc := range cases {
		u, err := db.UserByLogin(ctx, tc.login)
		if err != nil {
			t.Fatalf("UserByLogin(%q): %v", tc.login, err)
		}
		if !security.CheckPasswordHash(u.PasswordHash, tc.password) {
			t.Errorf("legacy user %q cannot log in with their existing password", tc.login)
		}
		if u.CreatedAt.IsZero() {
			t.Errorf("legacy user %q has a zero created_at", tc.login)
		}
	}

	u, err := db.UserByAPIKey(ctx, "11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatalf("UserByAPIKey: %v", err)
	}
	if u.Username != "alice" {
		t.Errorf("API key resolved to %q, want alice", u.Username)
	}
}

func TestReadsLegacyURLFields(t *testing.T) {
	ctx := context.Background()
	db := openLegacyFixture(t)

	u, err := db.URLByShortCode(ctx, "ABC123")
	if err != nil {
		t.Fatalf("URLByShortCode: %v", err)
	}

	if u.LongURL != "https://example.com/a/very/long/path?with=query" {
		t.Errorf("LongURL = %q", u.LongURL)
	}
	if u.ClicksCount != 42 {
		t.Errorf("ClicksCount = %d, want 42 (from the legacy `clicks` column)", u.ClicksCount)
	}
	want := []string{"https://alt1.example.com", "https://alt2.example.com"}
	if len(u.RotateTargets) != len(want) {
		t.Fatalf("RotateTargets = %v, want %v", u.RotateTargets, want)
	}
	for i := range want {
		if u.RotateTargets[i] != want[i] {
			t.Errorf("RotateTargets[%d] = %q, want %q", i, u.RotateTargets[i], want[i])
		}
	}
	if u.IOSTargetURL != "https://apps.apple.com/app/id123" {
		t.Errorf("IOSTargetURL = %q", u.IOSTargetURL)
	}
	if !u.PreviewMode || !u.StatsEnabled || !u.IsEnabled {
		t.Errorf("legacy boolean columns misread: preview=%v stats=%v enabled=%v",
			u.PreviewMode, u.StatsEnabled, u.IsEnabled)
	}
	if !security.CheckPasswordHash(u.PasswordHash, "link-secret") {
		t.Error("legacy link password does not verify")
	}

	// Microsecond precision must survive the text round-trip.
	wantExpiry := time.Date(2030, 6, 1, 12, 30, 45, 123456000, time.UTC)
	if u.ExpiresAt == nil || !u.ExpiresAt.Equal(wantExpiry) {
		t.Errorf("ExpiresAt = %v, want %v", u.ExpiresAt, wantExpiry)
	}
	if u.StartAt == nil || !u.StartAt.Equal(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("StartAt = %v", u.StartAt)
	}
	if u.UserID == nil {
		t.Error("owned link lost its user_id")
	}

	anon, err := db.URLByShortCode(ctx, "ANON01")
	if err != nil {
		t.Fatalf("URLByShortCode(ANON01): %v", err)
	}
	if anon.UserID != nil {
		t.Error("anonymous link should have a NULL user_id")
	}
	if anon.PreviewMode || anon.StatsEnabled || anon.IsEnabled {
		t.Error("false booleans misread as true")
	}
	if len(anon.RotateTargets) != 0 {
		t.Errorf("NULL rotate_targets should decode to empty, got %v", anon.RotateTargets)
	}
	if anon.ExpiresAt != nil {
		t.Errorf("NULL expires_at should decode to nil, got %v", anon.ExpiresAt)
	}
	if anon.IsActive() {
		t.Error("disabled link reported as active")
	}
}

// TestNewRowsMatchLegacyTimestampFormat is the guard against silently splitting
// the table into two incomparable timestamp encodings: SQL range filters and
// ORDER BY on these TEXT columns are lexicographic, so a row written by this
// service must use the very same layout as the rows Python left behind.
func TestNewRowsMatchLegacyTimestampFormat(t *testing.T) {
	ctx := context.Background()
	db := openLegacyFixture(t)

	link := &URL{ShortCode: "GONEW1", LongURL: "https://new.example.com/", IsEnabled: true}
	if err := db.CreateURL(ctx, link); err != nil {
		t.Fatalf("CreateURL: %v", err)
	}

	var (
		typeName string
		raw      string
	)
	// `|| ''` strips the column's declared DATETIME type, which otherwise makes
	// the driver parse and reformat the value before this test can see it.
	if err := db.QueryRow(ctx,
		"SELECT typeof(created_at), created_at || '' FROM urls WHERE id = ?", link.ID,
	).Scan(&typeName, &raw); err != nil {
		t.Fatalf("read back created_at: %v", err)
	}
	if typeName != "text" {
		t.Errorf("created_at stored as %q, but legacy rows are text", typeName)
	}
	if _, err := time.Parse(pyDatetimeLayout, raw); err != nil {
		t.Errorf("created_at %q does not match SQLAlchemy's layout %q", raw, pyDatetimeLayout)
	}

	// A range filter must see the new row alongside the legacy ones.
	var n int
	if err := db.QueryRow(ctx,
		"SELECT COUNT(*) FROM urls WHERE created_at >= ?",
		NewTime(db.Dialect(), time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)),
	).Scan(&n); err != nil {
		t.Fatalf("range query: %v", err)
	}
	if n != 4 {
		t.Errorf("range query matched %d rows, want 4 — new and legacy timestamps do not compare", n)
	}
}

func TestLegacyClickAnalytics(t *testing.T) {
	ctx := context.Background()
	db := openLegacyFixture(t)

	link, err := db.URLByShortCode(ctx, "ABC123")
	if err != nil {
		t.Fatalf("URLByShortCode: %v", err)
	}

	recent, err := db.RecentClicks(ctx, link.ID, 10)
	if err != nil {
		t.Fatalf("RecentClicks: %v", err)
	}
	if len(recent) != 5 {
		t.Fatalf("RecentClicks returned %d rows, want 5", len(recent))
	}
	if recent[0].Timestamp.Before(recent[len(recent)-1].Timestamp) {
		t.Error("RecentClicks is not ordered newest first")
	}
	if recent[0].Country == "" {
		t.Error("legacy click country did not load")
	}

	cutoff := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	countries, err := db.ClicksGroupedBy(ctx, link.ID, cutoff, "country")
	if err != nil {
		t.Fatalf("ClicksGroupedBy: %v", err)
	}
	total := int64(0)
	for _, b := range countries {
		total += b.Count
	}
	if total != 5 {
		t.Errorf("country buckets total %d, want 5", total)
	}

	daily, err := db.ClicksByTimeBucket(ctx, link.ID, cutoff, false)
	if err != nil {
		t.Fatalf("ClicksByTimeBucket: %v", err)
	}
	if len(daily) != 1 || daily[0].Label != "2026-05-04" || daily[0].Count != 5 {
		t.Errorf("daily buckets = %v, want one bucket 2026-05-04 with 5", daily)
	}
}

func TestRecordClickUpdatesCounterAndLog(t *testing.T) {
	ctx := context.Background()
	db := openLegacyFixture(t)

	link, err := db.URLByShortCode(ctx, "ABC123")
	if err != nil {
		t.Fatalf("URLByShortCode: %v", err)
	}

	if err := db.RecordClick(ctx, &Click{
		URLID: link.ID, IPAddress: "198.51.xxx.xxx", Country: "France",
		Browser: "Safari", Platform: "iOS", Referrer: "Direct",
	}); err != nil {
		t.Fatalf("RecordClick: %v", err)
	}

	after, err := db.URLByShortCode(ctx, "ABC123")
	if err != nil {
		t.Fatalf("re-read link: %v", err)
	}
	if after.ClicksCount != 43 {
		t.Errorf("ClicksCount = %d after one click, want 43", after.ClicksCount)
	}

	recent, err := db.RecentClicks(ctx, link.ID, 1)
	if err != nil {
		t.Fatalf("RecentClicks: %v", err)
	}
	if len(recent) != 1 || recent[0].Country != "France" {
		t.Errorf("newest click = %+v, want the France row just written", recent)
	}
}

func TestDashboardStatsOverLegacyData(t *testing.T) {
	ctx := context.Background()
	db := openLegacyFixture(t)

	alice, err := db.UserByLogin(ctx, "alice")
	if err != nil {
		t.Fatalf("UserByLogin: %v", err)
	}
	s, err := db.DashboardStats(ctx, alice.ID)
	if err != nil {
		t.Fatalf("DashboardStats: %v", err)
	}
	if s.TotalLinks != 1 {
		t.Errorf("TotalLinks = %d, want 1", s.TotalLinks)
	}
	if s.TotalClicks != 42 {
		t.Errorf("TotalClicks = %d, want 42", s.TotalClicks)
	}
	if s.ActiveLinks != 1 {
		t.Errorf("ActiveLinks = %d, want 1", s.ActiveLinks)
	}
	if s.TopPerformer == nil || s.TopPerformer.ShortCode != "ABC123" {
		t.Errorf("TopPerformer = %v, want ABC123", s.TopPerformer)
	}
}

func TestMigrateOnEmptyDatabaseCreatesSchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "fresh.db")

	db, err := Open(ctx, "sqlite:///"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// Running twice must be a no-op, since it runs on every boot.
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	u := &User{Username: "carol", Email: "carol@example.com", PasswordHash: "x", APIKey: "k"}
	if err := db.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID == 0 {
		t.Error("CreateUser did not populate the generated id")
	}
}
