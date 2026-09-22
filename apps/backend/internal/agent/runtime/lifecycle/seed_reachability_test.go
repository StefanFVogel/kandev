package lifecycle

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

func seedPreparer(t *testing.T) (*WorktreePreparer, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	return &WorktreePreparer{logger: log}, logs
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-006.2, .3, .4
// copy_files always lands in the repository's own worktree root. With two or
// more repositories the agent runs one level above, so a seed meant for it is
// never read — and the session looks as though the configuration were missing.
func TestWarnsWhenCopyFilesSeedCannotReachTheAgent(t *testing.T) {
	p, logs := seedPreparer(t)

	p.warnUnreachableCopyFilesSeeds(
		[]RepoPrepareSpec{
			{RepositoryID: "repo-a", RepoName: "backend", CopyFiles: ".claude/settings.local.json"},
			{RepositoryID: "repo-b", RepoName: "frontend"},
		},
		[]RepoWorktreeResult{
			{RepositoryID: "repo-a", WorktreePath: "/tasks/t1/backend"},
			{RepositoryID: "repo-b", WorktreePath: "/tasks/t1/frontend"},
		},
		"/tasks/t1",
	)

	entries := logs.FilterMessage("copy_files seed does not reach the agent working directory").All()
	if len(entries) != 1 {
		t.Fatalf("warnings = %d, want exactly the repository that seeds", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["repository"] != "backend" {
		t.Fatalf("repository = %v, want backend", fields["repository"])
	}
	if fields["seed_destination"] != "/tasks/t1/backend" {
		t.Fatalf("seed_destination = %v, want the repository worktree root", fields["seed_destination"])
	}
	if fields["agent_working_dir"] != "/tasks/t1" {
		t.Fatalf("agent_working_dir = %v, want the task root", fields["agent_working_dir"])
	}
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-006.5
func TestNoWarningForSingleRepositoryLayout(t *testing.T) {
	p, logs := seedPreparer(t)

	p.warnUnreachableCopyFilesSeeds(
		[]RepoPrepareSpec{{RepositoryID: "repo-a", RepoName: "backend", CopyFiles: ".env"}},
		[]RepoWorktreeResult{{RepositoryID: "repo-a", WorktreePath: "/tasks/t1/backend"}},
		"/tasks/t1/backend",
	)

	if got := logs.FilterMessage("copy_files seed does not reach the agent working directory").Len(); got != 0 {
		t.Fatalf("warnings = %d, want none for a single-repository layout", got)
	}
}

// A multi-repository workspace without any seed has nothing to report.
func TestNoWarningWithoutASeed(t *testing.T) {
	p, logs := seedPreparer(t)

	p.warnUnreachableCopyFilesSeeds(
		[]RepoPrepareSpec{{RepositoryID: "repo-a"}, {RepositoryID: "repo-b"}},
		[]RepoWorktreeResult{{RepositoryID: "repo-a"}, {RepositoryID: "repo-b"}},
		"/tasks/t1",
	)

	if got := logs.FilterMessage("copy_files seed does not reach the agent working directory").Len(); got != 0 {
		t.Fatalf("warnings = %d, want none without a configured seed", got)
	}
}
