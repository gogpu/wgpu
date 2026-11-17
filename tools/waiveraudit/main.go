// Command waiveraudit enforces the two properties of the public-layer waiver file
// (internal/layoutcheck/publiclayer_allow.txt) that a hermetic unit test cannot: growth over time and
// expiry against the wall clock. The checker in internal/layoutcheck owns the format, the stale/unused
// failure and the count limit; this tool owns the clock and the history, which is why the unit test stays
// green on the same tree next month (PRACTICES-CONSENSUS O1, Amendment 2.2).
//
// Subcommands:
//
//	growth -file <path> -base <ref>
//	    Runs `git diff --unified=0 <base>..HEAD -- <file>` and fails when the number of ADDED data-row
//	    lines exceeds the number of REMOVED data-row lines.
//
//	    The exact rule. With --unified=0 there are no context lines. A diff line is a DATA row when its
//	    body (after the leading + or -) is non-blank and is neither a comment (`#...`) nor the `limit:`
//	    header. The `diff --git`/`index` lines, the `---`/`+++` file headers and the `@@` hunk headers are
//	    never data rows, even though they start with a diff prefix. The list grows when
//	    added > removed: adding a reviewed entry fails, removing one passes, and a swap (one added, one
//	    removed) passes here and is caught instead by the checker's "every entry fires" plus the
//	    owner/reason diff in review (count-only ratchets are rejected: remove-one-add-one would swap a
//	    reviewed site for an unreviewed one).
//
//	    The one exception is the documented escape hatch: a lead-approved change is a dated amendment,
//	    never a silent edit. Its mechanical form is the base copy of the file carrying no `limit:` header
//	    (the list does not exist yet, or still has the retired path:line schema). That change establishes
//	    the reviewed list and is the amendment; the tool reports the counts and passes. Every later change
//	    is measured against an established list and may not grow.
//
//	expire -file <path> [-today YYYY-MM-DD]
//	    Fails on every dated waiver whose date is before today (the default is the system date; -today is
//	    injectable for tests), naming the entry's owner, card and reason. `permanent` never expires. A
//	    malformed row or a malformed date is a failure here as well.
//
// The tool reads the same tab-separated schema as the checker and lives in the tools module so the root
// module's build and tests never depend on git or the wall clock.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// waiverColumns is the data-row column count: path, symbol, construct, reason, owner, card, expires.
const waiverColumns = 7

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "growth":
		err = runGrowthCLI(os.Args[2:])
	case "expire":
		err = runExpireCLI(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "waiveraudit:", err)
		os.Exit(1)
	}
}

// usage prints the subcommand summary.
func usage() {
	fmt.Fprintln(os.Stderr, "usage: waiveraudit growth -file <path> -base <ref>")
	fmt.Fprintln(os.Stderr, "       waiveraudit expire -file <path> [-today YYYY-MM-DD]")
}

// runGrowthCLI runs the growth check against the working tree in the current directory.
func runGrowthCLI(args []string) error {
	fs := flag.NewFlagSet("growth", flag.ContinueOnError)
	file := fs.String("file", "", "waiver file, repo-relative")
	base := fs.String("base", "", "git ref to diff against (the PR base)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" || *base == "" {
		return errors.New("growth: -file and -base are required")
	}
	added, removed, established, err := growth(".", *file, *base)
	if err != nil {
		return err
	}
	if !established {
		fmt.Printf("waiver growth not enforced: %d added, %d removed (%s..HEAD); the base carries no `limit:` header, so this change establishes the schema (the dated amendment)\n", added, removed, *base)
		return nil
	}
	if added > removed {
		return fmt.Errorf("waiver list grew: %d data rows added, %d removed (%s..HEAD); a lead-approved change is a dated amendment, not a silent edit", added, removed, *base)
	}
	fmt.Printf("waiver growth ok: %d added, %d removed (%s..HEAD)\n", added, removed, *base)
	return nil
}

// growth returns the added and removed data-row counts of the waiver file's diff between base and HEAD,
// and whether the base already carries the schema (`limit:` header). A change whose base has no schema is
// the documented amendment that establishes the list, not growth of a reviewed list.
func growth(dir, file, base string) (added, removed int, established bool, err error) {
	cmd := exec.Command("git", "-C", dir, "diff", "--unified=0", base+"..HEAD", "--", file)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, false, fmt.Errorf("git diff %s..HEAD -- %s: %w: %s", base, file, err, strings.TrimSpace(stderr.String()))
	}
	added, removed = countDataRows(string(out))
	return added, removed, baseHasLimit(dir, file, base), nil
}

// baseHasLimit reports whether the base copy of the file carries a `limit:` header. git diff already proved
// the ref resolves, so a `git show` failure here means the file did not exist at base: not established.
func baseHasLimit(dir, file, base string) bool {
	out, err := exec.Command("git", "-C", dir, "show", base+":"+file).Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "limit:") {
			return true
		}
	}
	return false
}

// countDataRows counts the added and removed data-row lines of a unified diff. See the package comment for
// the exact rule.
func countDataRows(diff string) (int, int) {
	added, removed := 0, 0
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case isDiffMeta(line):
			continue
		case strings.HasPrefix(line, "+"):
			if isDataRow(line[1:]) {
				added++
			}
		case strings.HasPrefix(line, "-"):
			if isDataRow(line[1:]) {
				removed++
			}
		}
	}
	return added, removed
}

// isDiffMeta reports whether a diff line is a header rather than a content line.
func isDiffMeta(line string) bool {
	for _, prefix := range []string{"diff ", "index ", "@@", "--- ", "+++ "} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// isDataRow reports whether a line body is a waiver data row (not blank, a comment, or the limit header).
func isDataRow(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "limit:") {
		return false
	}
	return true
}

// runExpireCLI runs the expiry check on one waiver file.
func runExpireCLI(args []string) error {
	fs := flag.NewFlagSet("expire", flag.ContinueOnError)
	file := fs.String("file", "", "waiver file path")
	todayFlag := fs.String("today", "", "override today as YYYY-MM-DD (default: system date)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("expire: -file is required")
	}
	today, err := parseToday(*todayFlag)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(*file) //nolint:gosec // G304: the path is an operator-supplied CLI flag, not tree data
	if err != nil {
		return err
	}
	violations := expiredWaivers(string(data), today)
	if len(violations) > 0 {
		return fmt.Errorf("expired waivers:\n%s", strings.Join(violations, "\n"))
	}
	fmt.Println("waiver expiry ok")
	return nil
}

// parseToday returns the reference date: the -today flag when set, else the system date, both at UTC
// midnight so a date compares as a date and "today" never counts as expired.
func parseToday(todayFlag string) (time.Time, error) {
	if todayFlag == "" {
		return dateOnly(time.Now()), nil
	}
	d, err := time.Parse("2006-01-02", todayFlag)
	if err != nil {
		return time.Time{}, fmt.Errorf("expire: -today %q: %w", todayFlag, err)
	}
	return d, nil
}

// dateOnly collapses a time to its calendar date at UTC midnight.
func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// expiredWaivers returns one line per waiver that expired before today or is malformed.
func expiredWaivers(content string, today time.Time) []string {
	today = dateOnly(today)
	var violations []string
	for i, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "limit:") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != waiverColumns {
			violations = append(violations, fmt.Sprintf("line %d: malformed row (%d columns, want %d)", i+1, len(fields), waiverColumns))
			continue
		}
		if v := expiryViolation(i+1, fields, today); v != "" {
			violations = append(violations, v)
		}
	}
	return violations
}

// expiryViolation checks one parsed row's expires column against today.
func expiryViolation(line int, fields []string, today time.Time) string {
	expires := strings.TrimSpace(fields[6])
	if expires == "permanent" {
		return ""
	}
	d, err := time.Parse("2006-01-02", expires)
	if err != nil || !isISODate(expires) {
		return fmt.Sprintf("line %d: malformed expires %q (want YYYY-MM-DD or permanent)", line, expires)
	}
	if d.Before(today) {
		return fmt.Sprintf("line %d: expired %s (owner %s, card %s, reason: %s)",
			line, expires, strings.TrimSpace(fields[4]), strings.TrimSpace(fields[5]), strings.TrimSpace(fields[3]))
	}
	return ""
}

// isISODate reports whether s is exactly YYYY-MM-DD.
func isISODate(s string) bool {
	if len(s) != len("2006-01-02") {
		return false
	}
	for i := 0; i < len(s); i++ {
		if i == 4 || i == 7 {
			if s[i] != '-' {
				return false
			}
			continue
		}
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
