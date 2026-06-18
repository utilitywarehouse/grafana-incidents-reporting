package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitT runs a git command for test setup/inspection, failing the test on error.
func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// seedBareRepo creates a bare repo with one commit on main and returns a
// file:// URL for it. allowFilter lets partial (blob:none) clones work over the
// local smart transport.
func seedBareRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	gitT(t, root, "init", "--bare", "--initial-branch=main", bare)
	gitT(t, bare, "config", "uploadpack.allowFilter", "true")
	gitT(t, bare, "config", "uploadpack.allowAnySHA1InWant", "true")

	work := filepath.Join(root, "seed")
	gitT(t, root, "init", "--initial-branch=main", work)
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, work, "add", ".")
	gitT(t, work, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", "seed")
	gitT(t, work, "remote", "add", "origin", bare)
	gitT(t, work, "push", "origin", "main")

	return "file://" + bare
}

// showFile returns the content of path on main in the bare repo at url.
func showFile(t *testing.T, url, path string) string {
	t.Helper()
	bare := strings.TrimPrefix(url, "file://")
	return gitT(t, bare, "show", "main:"+path)
}

func TestPushCreatesUpdatesAndSkips(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	url := seedBareRepo(t)
	ctx := context.Background()
	opts := Options{RepoURL: url, Branch: "main"}
	const relPath = "reports/incidents-2026-05.md"

	// Create.
	sha, committed, err := Push(ctx, opts, relPath, []byte("first\n"), "add report")
	if err != nil {
		t.Fatalf("create push: %v", err)
	}
	if !committed || sha == "" {
		t.Fatalf("want committed with sha, got committed=%v sha=%q", committed, sha)
	}
	if got := showFile(t, url, relPath); got != "first" {
		t.Errorf("content after create = %q, want %q", got, "first")
	}

	// Overwrite with new content.
	_, committed, err = Push(ctx, opts, relPath, []byte("second\n"), "update report")
	if err != nil {
		t.Fatalf("update push: %v", err)
	}
	if !committed {
		t.Fatal("want committed on changed content")
	}
	if got := showFile(t, url, relPath); got != "second" {
		t.Errorf("content after update = %q, want %q", got, "second")
	}

	// Identical content is a no-op.
	_, committed, err = Push(ctx, opts, relPath, []byte("second\n"), "noop")
	if err != nil {
		t.Fatalf("noop push: %v", err)
	}
	if committed {
		t.Error("want committed=false for identical content")
	}
}

func TestNormalizeRemote(t *testing.T) {
	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{in: "org/repo", want: "git@github.com:org/repo.git"},
		{in: "git@github.com:org/repo.git", want: "git@github.com:org/repo.git"},
		{in: "ssh://git@host/org/repo", want: "ssh://git@host/org/repo"},
		{in: "https://example.com/x.git", want: "https://example.com/x.git"},
		{in: "", wantErr: true},
		{in: "noslash", wantErr: true},
	}
	for _, tc := range tests {
		got, err := normalizeRemote(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("normalizeRemote(%q) = %q, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeRemote(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("normalizeRemote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
