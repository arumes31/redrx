package safety

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestParseDomainListStripsInlineComments guards the hosts-format parsing: a
// trailing "# note" must not be stored as the domain, and neither the address
// prefix nor a full-line comment should leak in.
func TestParseDomainListStripsInlineComments(t *testing.T) {
	const list = `
# a full-line comment
0.0.0.0 evil.com. # inline note
bad.example
127.0.0.1 phish.test#tight-comment
   spaced.example   # trailing
localhost
`
	domains, err := parseDomainList(strings.NewReader(list))
	if err != nil {
		t.Fatalf("parseDomainList: %v", err)
	}

	want := map[string]bool{
		"evil.com": true, "bad.example": true,
		"phish.test": true, "spaced.example": true,
	}
	for d := range want {
		if _, ok := domains[d]; !ok {
			t.Errorf("expected %q in the parsed list", d)
		}
	}
	for _, unwanted := range []string{"note", "inline", "comment", "tight-comment", "localhost", "0.0.0.0"} {
		if _, ok := domains[unwanted]; ok {
			t.Errorf("%q should not have been stored as a domain", unwanted)
		}
	}
	if len(domains) != len(want) {
		t.Errorf("parsed %d domains, want %d: %v", len(domains), len(want), domains)
	}
}

func writeList(t *testing.T, lines string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "blocked_domains.txt")
	if err := os.WriteFile(path, []byte(lines), 0o600); err != nil {
		t.Fatalf("write list: %v", err)
	}
	return path
}

func TestRejectsNonHTTPSchemes(t *testing.T) {
	c := New(Options{Enabled: false})
	for _, target := range []string{
		"javascript:alert(1)",
		"data:text/html;base64,PHNjcmlwdD4=",
		"file:///etc/passwd",
		"ftp://example.com/x",
		"/relative/path",
		"example.com",
		"",
	} {
		if c.IsSafeURL(target) {
			t.Errorf("IsSafeURL(%q) = true, want false", target)
		}
	}
}

func TestAllowsOrdinaryURLsWhenCheckDisabled(t *testing.T) {
	c := New(Options{Enabled: false})
	for _, target := range []string{
		"https://example.com/",
		"http://example.com/path?q=1",
		"https://sub.example.co.uk/a/b",
	} {
		if !c.IsSafeURL(target) {
			t.Errorf("IsSafeURL(%q) = false, want true", target)
		}
	}
}

// TestManualBlocklistMatchesSubdomains covers canonical and absolute DNS names
// supplied through configuration.
func TestManualBlocklistMatchesSubdomains(t *testing.T) {
	c := New(Options{Enabled: false, ManualDomains: []string{"evil.com.", "Bad.Example."}})

	blocked := []string{
		"https://evil.com/",
		"https://login.evil.com/steal",
		"https://deep.sub.evil.com/",
		"https://bad.example/",
		"https://x.bad.example/",
		"https://EVIL.COM/upper",
		"https://evil.com./root-dot",
		"https://LOGIN.EVIL.COM./mixed-case-root-dot",
		"https://evil.com:8443/with-port",
	}
	for _, target := range blocked {
		if c.IsSafeURL(target) {
			t.Errorf("IsSafeURL(%q) = true, want false", target)
		}
	}

	// A domain that merely ends with the same letters must not be caught.
	for _, target := range []string{"https://notevil.com/", "https://evil.com.example.org/"} {
		if !c.IsSafeURL(target) {
			t.Errorf("IsSafeURL(%q) = false, want true", target)
		}
	}
}

// TestPhishingListBlocksDomainAndSubdomains covers canonical and absolute DNS
// names loaded from a phishing feed.
func TestPhishingListBlocksDomainAndSubdomains(t *testing.T) {
	path := writeList(t, "phish.example.\n# a comment\n\nOTHER-PHISH.example.\n")
	c := New(Options{Enabled: true, BlockedListPath: path})

	for _, target := range []string{
		"https://phish.example/",
		"https://www.phish.example/login",
		"https://other-phish.example/",
		"https://PHISH.EXAMPLE./root-dot",
		"https://www.phish.example./subdomain-root-dot",
	} {
		if c.IsSafeURL(target) {
			t.Errorf("IsSafeURL(%q) = true, want false", target)
		}
	}
	if !c.IsSafeURL("https://legitimate.example/") {
		t.Error("a domain absent from the list was blocked")
	}
}

func TestParsesHostsFileFormat(t *testing.T) {
	path := writeList(t, "0.0.0.0 malware.example\n127.0.0.1 localhost\n127.0.0.1 tracker.example\n")
	c := New(Options{Enabled: true, BlockedListPath: path})

	if c.IsSafeURL("https://malware.example/") {
		t.Error("a hosts-file entry was not blocked")
	}
	if c.IsSafeURL("https://tracker.example/") {
		t.Error("a hosts-file entry was not blocked")
	}
	if !c.IsSafeURL("https://example.org/") {
		t.Error("an unrelated domain was blocked")
	}
}

// TestFailsClosedWhenListMissing is the important one: with checking enabled
// and no readable list, nothing may be judged safe. Failing open here would
// silently disable phishing protection.
func TestFailsClosedWhenListMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.txt")
	c := New(Options{Enabled: true, BlockedListPath: missing})

	if c.IsSafeURL("https://example.com/") {
		t.Error("URL judged safe even though the blocklist could not be read")
	}

	ok, err := c.CheckURL("https://example.com/")
	if err == nil {
		t.Error("CheckURL returned no error for an unreadable blocklist")
	}
	if ok {
		t.Error("CheckURL reported safe for an unreadable blocklist")
	}
}

// TestServesCachedListWhenFileDisappears keeps a transient filesystem problem
// from taking the whole service down once a good list has been loaded.
func TestServesCachedListWhenFileDisappears(t *testing.T) {
	path := writeList(t, "phish.example\n")
	c := New(Options{Enabled: true, BlockedListPath: path})

	if c.IsSafeURL("https://phish.example/") {
		t.Fatal("list was not applied on first load")
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove list: %v", err)
	}

	if c.IsSafeURL("https://phish.example/") {
		t.Error("cached blocklist stopped applying after the file vanished")
	}
	if !c.IsSafeURL("https://legitimate.example/") {
		t.Error("unrelated URLs should stay usable while serving the cached list")
	}
}

func TestReloadsWhenFileChanges(t *testing.T) {
	path := writeList(t, "first.example\n")
	c := New(Options{Enabled: true, BlockedListPath: path})

	if c.IsSafeURL("https://first.example/") {
		t.Fatal("initial list not applied")
	}
	if !c.IsSafeURL("https://second.example/") {
		t.Fatal("second.example should not be blocked yet")
	}

	// Ensure the modification time actually differs on coarse-grained clocks.
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("second.example\n"), 0o600); err != nil {
		t.Fatalf("rewrite list: %v", err)
	}

	if c.IsSafeURL("https://second.example/") {
		t.Error("the updated list was not picked up")
	}
	if !c.IsSafeURL("https://first.example/") {
		t.Error("the replaced entry is still being blocked")
	}
}

func TestRefreshIsNoOpWhenDisabled(t *testing.T) {
	c := New(Options{Enabled: false, BlockedListPath: filepath.Join(t.TempDir(), "x.txt")})
	if err := c.Refresh(t.Context()); err != nil {
		t.Errorf("Refresh with checking disabled returned %v, want nil", err)
	}
}
