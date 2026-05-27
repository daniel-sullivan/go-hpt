//go:build cgo

package hpt

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The cgo ticker threads write a byte to a pipe to signal each tick. On Linux
// with glibc and _FORTIFY_SOURCE, write() is declared with
// __attribute__((warn_unused_result)), so ignoring its return value triggers:
//
//	warning: ignoring return value of 'write' declared with attribute
//	'warn_unused_result' [-Wunused-result]
//
// These tests reproduce that compiler diagnostic and guard against the pattern
// reappearing in the platform cgo sources.

func findCC(t *testing.T) string {
	t.Helper()
	cc := os.Getenv("CC")
	if cc == "" {
		cc = "cc"
	}
	path, err := exec.LookPath(cc)
	if err != nil {
		t.Skipf("no C compiler available (%q): %v", cc, err)
	}
	return path
}

// compileSnippet writes src to a temp .c file and compiles it (no link) with
// -Werror=unused-result, returning the combined compiler output and error.
func compileSnippet(t *testing.T, cc, src string) ([]byte, error) {
	t.Helper()
	dir := t.TempDir()
	cf := filepath.Join(dir, "snippet.c")
	if err := os.WriteFile(cf, []byte(src), 0o644); err != nil {
		t.Fatalf("write snippet: %v", err)
	}
	cmd := exec.Command(cc, "-Werror=unused-result", "-c", cf, "-o", filepath.Join(dir, "snippet.o"))
	out, err := cmd.CombinedOutput()
	return out, err
}

// TestCgoWriteUnusedResultReproducesError confirms that ignoring the return
// value of a warn_unused_result write (as glibc declares it) fails to compile
// under -Werror=unused-result -- i.e. it replicates the reported error.
func TestCgoWriteUnusedResultReproducesError(t *testing.T) {
	cc := findCC(t)

	const buggy = `
#include <unistd.h>
// Mirror glibc's fortified declaration so the diagnostic fires on any platform.
extern ssize_t hpt_write(int, const void *, size_t) __attribute__((warn_unused_result));
void ticker(int fd) {
	char b = 1;
	hpt_write(fd, &b, 1); // ignores return value -> -Wunused-result
}
`
	out, err := compileSnippet(t, cc, buggy)
	if err == nil {
		t.Fatalf("expected compile to fail on ignored warn_unused_result, but it succeeded")
	}
	if !strings.Contains(string(out), "unused-result") {
		t.Fatalf("expected unused-result diagnostic, got:\n%s", out)
	}
}

// TestCgoWriteFixCompiles confirms the fix idiom (capture the result and
// explicitly discard it) compiles cleanly under -Werror=unused-result.
func TestCgoWriteFixCompiles(t *testing.T) {
	cc := findCC(t)

	const fixed = `
#include <unistd.h>
extern ssize_t hpt_write(int, const void *, size_t) __attribute__((warn_unused_result));
void ticker(int fd) {
	char b = 1;
	ssize_t wr = hpt_write(fd, &b, 1);
	(void)wr;
}
`
	out, err := compileSnippet(t, cc, fixed)
	if err != nil {
		t.Fatalf("fix idiom failed to compile: %v\n%s", err, out)
	}
}

// TestPlatformCgoSourcesHandleWriteResult guards the actual platform cgo files:
// any write() to the ticker pipe must consume its return value so the
// warn_unused_result diagnostic never reappears.
func TestPlatformCgoSourcesHandleWriteResult(t *testing.T) {
	// A bare statement-level write(...) whose result is discarded.
	bareWrite := regexp.MustCompile(`(?m)^\s*write\(`)

	for _, name := range []string{"platform_linux_cgo.go", "platform_darwin_cgo.go"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if loc := bareWrite.FindIndex(data); loc != nil {
			line := strings.Count(string(data[:loc[0]]), "\n") + 1
			t.Errorf("%s:%d has a write() call whose return value is ignored; "+
				"capture it (e.g. `ssize_t wr = write(...); (void)wr;`)", name, line)
		}
	}
}
