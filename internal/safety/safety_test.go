package safety

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
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
	domains, _, err := parseDomainList(strings.NewReader(list))
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

func TestHostsRecordKeepsEveryHostname(t *testing.T) {
	domains, unsupported, err := parseDomainList(strings.NewReader(
		"0.0.0.0 first.example second.example third.example # note\nnot-an-ip a.example b.example\n"))
	if err != nil {
		t.Fatalf("parseDomainList: %v", err)
	}
	for _, d := range []string{"first.example", "second.example", "third.example"} {
		if _, ok := domains[d]; !ok {
			t.Errorf("%s from a multi-host record was dropped", d)
		}
	}
	if unsupported != 1 {
		t.Errorf("unsupported = %d, want 1", unsupported)
	}
	for _, d := range []string{"a.example", "b.example"} {
		if _, ok := domains[d]; ok {
			t.Errorf("%s from an invalid-address record was stored", d)
		}
	}
}

func TestUnsupportedCountsLinesOnce(t *testing.T) {
	_, unsupported, err := parseDomainList(strings.NewReader("0.0.0.0 a..example b..example ok.example\n"))
	if err != nil {
		t.Fatalf("parseDomainList: %v", err)
	}
	if unsupported != 1 {
		t.Errorf("unsupported = %d, want 1 for a single line", unsupported)
	}
}

func TestIPLiteralsAreCanonicalized(t *testing.T) {
	c := New(Options{ManualDomains: []string{"2001:db8::1"}})
	if c.IsSafeURL("https://[2001:0db8:0:0:0:0:0:1]/") {
		t.Error("an alternate spelling of a blocked IPv6 address was allowed")
	}
}

func TestRefreshKeepsListWhenFeedHasNoDomains(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("# nothing but comments\n"))
	}))
	defer srv.Close()
	path := writeList(t, "evil.com\n")
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("age list: %v", err)
	}
	c := New(Options{Enabled: true, BlockedListPath: path, FeedURLs: []string{srv.URL}, RefreshInterval: time.Hour})
	if err := c.Refresh(t.Context()); err == nil {
		t.Fatal("Refresh installed a feed with no usable domains")
	}
	if c.IsSafeURL("https://evil.com/") {
		t.Error("the existing list was replaced")
	}
}

func TestIDNAEquivalentHostsAreBlocked(t *testing.T) {
	path := writeList(t, "evil.com\n")
	c := New(Options{Enabled: true, BlockedListPath: path, ManualDomains: []string{"blocked.test"}})

	for _, target := range []string{
		"https://ｅｖｉｌ.com/",       // fullwidth letters
		"https://evil。com/",       // ideographic full stop
		"https://login.EVIL．com/", // fullwidth full stop
		"https://ｂｌｏｃｋｅｄ.test/",   // manual list
	} {
		if c.IsSafeURL(target) {
			t.Errorf("IsSafeURL(%q) = true, want blocked", target)
		}
	}
	if !c.IsSafeURL("https://bücher.example/") {
		t.Error("an unrelated IDN was blocked")
	}
}

func TestUnicodeBlocklistEntriesAreCanonicalized(t *testing.T) {
	path := writeList(t, "bücher.example\n")
	c := New(Options{Enabled: true, BlockedListPath: path})
	if c.IsSafeURL("https://xn--bcher-kva.example/") {
		t.Error("punycode form of a Unicode blocklist entry was allowed")
	}
}

func TestRejectsOversizedHostnames(t *testing.T) {
	c := New(Options{})
	long := strings.Repeat("a.", 200) + "example"
	if c.IsSafeURL("https://" + long + "/") {
		t.Error("a hostname over the DNS length limit was accepted")
	}
	if c.IsSafeURL("https://" + strings.Repeat("a", 64) + ".example/") {
		t.Error("a label over 63 bytes was accepted")
	}
	if !c.IsSafeURL("https://" + strings.Repeat("a", 63) + ".example/") {
		t.Error("a 63-byte label was rejected")
	}
	label := strings.Repeat("a", 63)
	at := label + "." + label + "." + label + "." + strings.Repeat("b", 61)
	if len(at) != 253 || !c.IsSafeURL("https://"+at+"/") {
		t.Errorf("a %d-byte hostname was rejected", len(at))
	}
	if c.IsSafeURL("https://" + at + "b/") {
		t.Error("a 254-byte hostname was accepted")
	}
}

func TestFetchRejectsOversizedFeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chunk := bytes.Repeat([]byte("a.example\n"), 1<<16)
		for written := 0; written <= maxFeedBytes; written += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	_, err := fetch(context.Background(), srv.Client(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "feed exceeds") {
		t.Fatalf("fetch of an oversized feed = %v, want the size-cap error", err)
	}
}

func TestFetchAcceptsFeedAtCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("a"), maxFeedBytes))
	}))
	defer srv.Close()

	body, err := fetch(context.Background(), srv.Client(), srv.URL)
	if err != nil || len(body) != maxFeedBytes {
		t.Fatalf("fetch at the cap = %d bytes, %v; want %d bytes, nil", len(body), err, maxFeedBytes)
	}
}
