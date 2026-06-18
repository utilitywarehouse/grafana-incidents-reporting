// Package git commits a single file into a remote git repository over SSH,
// using a deploy key. It clones as little as possible — shallow (one commit),
// partial (blobs fetched on demand), and sparse (only the target path) — so it
// stays cheap to run repeatedly, e.g. from a Kubernetes CronJob.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// Default identity for the commit when none is supplied. Deploy keys aren't
// tied to a user, so we attribute commits to the tool itself.
const (
	defaultAuthorName  = "grafana-incidents-reporting"
	defaultAuthorEmail = "grafana-incidents-reporting@users.noreply.github.com"
)

// Options configures a push.
type Options struct {
	// RepoURL is either an SSH remote ("git@github.com:org/repo.git",
	// "ssh://...") or a bare "owner/repo" shorthand, which is expanded to a
	// github.com SSH URL.
	RepoURL string
	// Branch to commit to; empty means the remote's default branch.
	Branch string
	// SSHKey is the path to the deploy key's private key. Empty uses ambient
	// SSH (or a caller-set GIT_SSH_COMMAND), which is mainly useful in tests.
	SSHKey string
	// AuthorName/AuthorEmail override the commit identity; empty uses the
	// defaults above.
	AuthorName  string
	AuthorEmail string
}

// Push writes content to relPath within the repo and pushes a commit with the
// given message, creating or overwriting the file. It returns the short SHA of
// the new commit. committed is false (with a nil error) when relPath already
// holds exactly this content, so there is nothing to commit.
func Push(ctx context.Context, o Options, relPath string, content []byte, message string) (sha string, committed bool, err error) {
	remote, err := normalizeRemote(o.RepoURL)
	if err != nil {
		return "", false, err
	}

	tmp, err := os.MkdirTemp("", "incidents-git-")
	if err != nil {
		return "", false, fmt.Errorf("create work dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if o.SSHKey != "" {
		env = append(env, "GIT_SSH_COMMAND="+sshCommand(o.SSHKey, filepath.Join(tmp, "known_hosts")))
	}

	repoDir := filepath.Join(tmp, "repo")
	clone := []string{"clone", "--depth", "1", "--filter=blob:none", "--sparse"}
	if o.Branch != "" {
		clone = append(clone, "--branch", o.Branch)
	}
	clone = append(clone, remote, repoDir)
	if _, err = run(ctx, tmp, env, clone...); err != nil {
		return "", false, err
	}

	// Limit the working tree to the file's directory; root-level files are
	// already present from the sparse clone's cone.
	if dir := path.Dir(relPath); dir != "." && dir != "" {
		if _, err = run(ctx, repoDir, env, "sparse-checkout", "set", dir); err != nil {
			return "", false, err
		}
	}

	full := filepath.Join(repoDir, filepath.FromSlash(relPath))
	if err = os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", false, fmt.Errorf("create path: %w", err)
	}
	if err = os.WriteFile(full, content, 0o644); err != nil {
		return "", false, fmt.Errorf("write file: %w", err)
	}
	if _, err = run(ctx, repoDir, env, "add", "--", relPath); err != nil {
		return "", false, err
	}

	status, err := run(ctx, repoDir, env, "status", "--porcelain")
	if err != nil {
		return "", false, err
	}
	if strings.TrimSpace(status) == "" {
		return "", false, nil // identical content already in the repo
	}

	commit := []string{
		"-c", "user.name=" + orDefault(o.AuthorName, defaultAuthorName),
		"-c", "user.email=" + orDefault(o.AuthorEmail, defaultAuthorEmail),
		"commit", "--quiet", "-m", message,
	}
	if _, err = run(ctx, repoDir, env, commit...); err != nil {
		return "", false, err
	}

	pushRef := "HEAD"
	if o.Branch != "" {
		pushRef = "HEAD:" + o.Branch
	}
	if _, err = run(ctx, repoDir, env, "push", "--quiet", "origin", pushRef); err != nil {
		return "", false, err
	}

	sha, err = run(ctx, repoDir, env, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", false, err
	}
	return sha, true, nil
}

// normalizeRemote expands an "owner/repo" shorthand to a github.com SSH URL and
// passes anything that already looks like a URL through unchanged.
func normalizeRemote(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("missing repo")
	}
	if strings.Contains(s, "://") || strings.Contains(s, "@") || strings.HasSuffix(s, ".git") {
		return s, nil
	}
	owner, repo, ok := strings.Cut(s, "/")
	if !ok || owner == "" || repo == "" {
		return "", fmt.Errorf("invalid repo %q (want owner/repo or an SSH URL)", s)
	}
	return fmt.Sprintf("git@github.com:%s/%s.git", owner, repo), nil
}

// sshCommand builds the GIT_SSH_COMMAND that forces use of the deploy key only
// and records unknown host keys on first use, so a fresh container neither
// offers the wrong identity nor blocks on an interactive prompt.
func sshCommand(keyPath, knownHosts string) string {
	return strings.Join([]string{
		"ssh", "-i", keyPath,
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=" + knownHosts,
	}, " ")
}

// run executes git in dir with env and returns trimmed stdout, wrapping
// failures with the command and stderr for context.
func run(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
