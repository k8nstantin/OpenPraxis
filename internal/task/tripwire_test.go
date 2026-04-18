package task

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestTripwire_NoHardcodedKnobDefaults guards against regressing the M4
// migration from hardcoded knob defaults to the resolver. Every pattern below
// represents a literal that M4-T11 / M4-T12 / M4-T13 removed; reintroducing
// one in runner/scheduler/serve code flips this test red.
//
// Scope: non-test Go files under internal/task/ and cmd/. Test files are
// intentionally excluded — tests legitimately seed literal values (e.g.
// setProductKnob(t, …, 237)) and must not be policed by this guard.
//
// Comment handling: the matcher strips inline `//` trailing comments before
// testing the pattern so `// maxTurns := 50 was removed in M4` in a doc block
// does not trip the wire. Block comments (`/* … */`) and multi-line doc
// comments are rarely used in this codebase for knob doc text and are not
// handled — if one ever wraps a forbidden literal, add it to the allowlist.
//
// Files explicitly not scanned: HTTP server ReadTimeout/WriteTimeout literals
// in cmd/serve.go — those are server-lifecycle knobs, not task-execution
// knobs, and are out of scope for the M4 migration. The 30*time.Minute
// regex below would not match them anyway (they use time.Second), but the
// comment is here so future edits do not accidentally narrow the scope.
func TestTripwire_NoHardcodedKnobDefaults(t *testing.T) {
	patterns := map[string]*regexp.Regexp{
		"maxParallel = 3":                regexp.MustCompile(`\bmaxParallel\s*=\s*3\b`),
		"maxTurns := 50":                 regexp.MustCompile(`\bmaxTurns\s*:?=\s*50\b`),
		"hardcoded 30*time.Minute":       regexp.MustCompile(`30\s*\*\s*time\.Minute\b`),
		"hardcoded temperature":          regexp.MustCompile(`\btemperature\s*:?=\s*0\.\d+\b`),
		"Task.MaxTurns field reference":  regexp.MustCompile(`\bTask\.MaxTurns\b`),
		"t.MaxTurns field reference":     regexp.MustCompile(`\bt\.MaxTurns\b`),
	}

	// Directories to walk, relative to the repo root. internal/task runs in
	// this package's directory, so we walk "." for it and "../../cmd" for
	// serve.go's neighborhood. Paths are joined lazily so the test remains
	// invariant under go test -run from any subdirectory.
	roots := []string{".", filepath.Join("..", "..", "cmd")}

	// allowlist holds file:line coordinates that have been reviewed and are
	// known-safe. Empty after M4-T13: the 30-min belt is gone, the
	// maxParallel=3 literal is gone (M4-T11), and the maxTurns := 50
	// fallback is gone (M4-T12). If a legitimate literal is ever needed
	// again (e.g. a test-only integration fixture), add the exact
	// "relative/path.go:<line>" here with a comment explaining why.
	allowlist := map[string]bool{}

	var violations []string

	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			lines := strings.Split(string(data), "\n")
			for i, raw := range lines {
				// Strip inline // comments so trailing commentary does not
				// trip the regex. We do not try to detect "//" inside
				// strings — no knob literal lives inside a string in this
				// codebase and a mis-strip on a string is harmless (the
				// regex still decides the final verdict).
				code := raw
				if idx := strings.Index(code, "//"); idx >= 0 {
					code = code[:idx]
				}
				// Trim to skip lines that were entirely comment.
				if strings.TrimSpace(code) == "" {
					continue
				}
				for name, re := range patterns {
					if !re.MatchString(code) {
						continue
					}
					coord := path + ":" + itoa(i+1)
					if allowlist[coord] {
						continue
					}
					violations = append(violations,
						coord+" — matched ["+name+"]: "+strings.TrimSpace(raw))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}

	if len(violations) > 0 {
		t.Fatalf("tripwire caught %d hardcoded knob default(s); these should flow from the resolver:\n  %s",
			len(violations), strings.Join(violations, "\n  "))
	}
}

// itoa avoids importing strconv for a single use. Small, fast, inlinable.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
