package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mustGit runs one git command in dir and fails the test on a non-zero exit.
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// initRepo creates a hermetic git repository in t.TempDir() with a local identity.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "init", "-q")
	mustGit(t, dir, "config", "user.email", "test@example.com")
	mustGit(t, dir, "config", "user.name", "test")
	mustGit(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

// writeFile writes one file of the temp repo.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// dataRow renders one valid-looking waiver data row; growth counts content, not validity.
func dataRow(marker string) string {
	return "f.go\tsym\trange\t" + marker + "\twgpu\tP2-API-034\tpermanent"
}

// TestGrowth proves the exact growth rule: an added data row fails (added > removed), a removed one passes,
// an unchanged file, a header edit and a limit change are neutral, and a swap passes.
func TestGrowth(t *testing.T) {
	const header = "# waivers\nlimit: 9\n"
	cases := []struct {
		name        string
		base, head  string
		wantAdded   int
		wantRemoved int
	}{
		{"entry added", header, header + dataRow("added") + "\n", 1, 0},
		{"entry removed", header + dataRow("removed") + "\n", header, 0, 1},
		{"unchanged", header + dataRow("same") + "\n", header + dataRow("same") + "\n", 0, 0},
		{"header comment edit", "# a\nlimit: 9\n", "# b\nlimit: 9\n", 0, 0},
		{"limit change", header, "# waivers\nlimit: 10\n", 0, 0},
		{"swap", header + dataRow("one") + "\n", header + dataRow("two") + "\n", 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := initRepo(t)
			writeFile(t, dir, "allow.txt", tc.base)
			mustGit(t, dir, "add", "-A")
			mustGit(t, dir, "commit", "-q", "--allow-empty", "-m", "base")
			writeFile(t, dir, "allow.txt", tc.head)
			mustGit(t, dir, "add", "-A")
			mustGit(t, dir, "commit", "-q", "--allow-empty", "-m", "head")

			added, removed, established, err := growth(dir, "allow.txt", "HEAD~1")
			if err != nil {
				t.Fatalf("growth: %v", err)
			}
			if !established {
				t.Fatalf("the base carries a limit header, want established=true")
			}
			if added != tc.wantAdded || removed != tc.wantRemoved {
				t.Fatalf("growth = (%d added, %d removed), want (%d, %d)", added, removed, tc.wantAdded, tc.wantRemoved)
			}
			if grew, wantGrew := added > removed, tc.wantAdded > tc.wantRemoved; grew != wantGrew {
				t.Fatalf("growth fails=%v, want fails=%v", grew, wantGrew)
			}
		})
	}
}

// TestGrowthMissingFile proves a git failure is surfaced, never read as "no growth".
func TestGrowthMissingFile(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "allow.txt", "limit: 1\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-q", "-m", "base")
	if _, _, _, err := growth(dir, "allow.txt", "no-such-ref"); err == nil {
		t.Fatal("growth accepted an unresolvable base ref")
	}
}

// TestGrowthEstablishesSchema proves the amendment rule: a base without a `limit:` header is the pre-schema
// list, so the change that introduces the schema and its entries is not growth of a reviewed list.
func TestGrowthEstablishesSchema(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "allow.txt", "old.go:10\tthe retired path:line form\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-q", "-m", "base")
	writeFile(t, dir, "allow.txt", "limit: 2\n"+dataRow("a")+"\n"+dataRow("b")+"\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-q", "-m", "head")

	added, removed, established, err := growth(dir, "allow.txt", "HEAD~1")
	if err != nil {
		t.Fatalf("growth: %v", err)
	}
	if established {
		t.Fatal("a base without a limit header was treated as an established list")
	}
	if added <= removed {
		t.Fatalf("fixture is not a growth: %d added, %d removed", added, removed)
	}
}

// rowWithExpiry renders a data row with the given expires and a fixed owner/reason.
func rowWithExpiry(expires string) string {
	return "f.go\tsym\trange\tbecause it is the boundary\twgpu\tCARD\t" + expires
}

// TestExpire proves the date boundary: past fails, today passes, future passes, permanent never expires, and
// a malformed expiry fails. The failure names the owner and the reason.
func TestExpire(t *testing.T) {
	today := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name          string
		expires       string
		wantViolation bool
	}{
		{"past", "2026-11-16", true},
		{"today", "2026-11-17", false},
		{"future", "2026-11-18", false},
		{"permanent", "permanent", false},
		{"malformed date", "2026-13-40", true},
		{"not a date", "soon", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content := "limit: 1\n" + rowWithExpiry(tc.expires) + "\n"
			violations := expiredWaivers(content, today)
			if got := len(violations) > 0; got != tc.wantViolation {
				t.Fatalf("expiredWaivers(%q) = %v, want violation=%v", tc.expires, violations, tc.wantViolation)
			}
			if tc.wantViolation && len(violations) > 0 && strings.Contains(violations[0], "expired ") {
				if !strings.Contains(violations[0], "wgpu") || !strings.Contains(violations[0], "because it is the boundary") {
					t.Fatalf("the expiry does not name the owner and reason: %q", violations[0])
				}
			}
		})
	}
}

// TestExpireMalformedRow proves a row without seven columns is a failure, not ignored.
func TestExpireMalformedRow(t *testing.T) {
	content := "limit: 1\nf.go\tsym\trange\tpermanent\n"
	if v := expiredWaivers(content, time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)); len(v) != 1 {
		t.Fatalf("a malformed row produced %v, want one violation", v)
	}
}

// TestParseToday proves the -today injection and the system-date default.
func TestParseToday(t *testing.T) {
	got, err := parseToday("2026-11-17")
	if err != nil || !got.Equal(time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("parseToday(2026-11-17) = %v, %v", got, err)
	}
	if _, err := parseToday("17-11-2026"); err == nil {
		t.Fatal("parseToday accepted a non-ISO date")
	}
	def, err := parseToday("")
	if err != nil || !def.Equal(dateOnly(time.Now())) {
		t.Fatalf("parseToday(\"\") = %v, %v; want the system date", def, err)
	}
}
