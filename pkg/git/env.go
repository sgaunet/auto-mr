package git

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

// repoLocalEnvVars are the environment variables git treats as belonging to one
// specific repository. The list is what `git rev-parse --local-env-vars` reports;
// git clears exactly these before running a command against a different
// repository, and auto-mr must do the same.
//
// Git exports GIT_DIR, GIT_INDEX_FILE and friends into the environment of every
// hook it runs, relative to the hooked repository. An auto-mr launched from a hook
// — or from a shell where the user exported GIT_DIR — would otherwise aim its
// native git calls at that repository even though cmd.Dir names the right one.
// It also keeps the two halves of the hybrid consistent: go-git resolves the
// repository purely from the path it is given and never consults these variables.
//
// This is deliberately a denylist rather than "drop everything named GIT_*":
// GIT_SSH_COMMAND, GIT_SSH, GIT_ASKPASS and GIT_TERMINAL_PROMPT carry the user's
// transport and credential configuration, and the native git push and ls-remote
// paths exist precisely to honour them. Do not widen this into a blanket strip.
var repoLocalEnvVars = map[string]struct{}{
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": {},
	"GIT_COMMON_DIR":                   {},
	"GIT_CONFIG":                       {},
	"GIT_CONFIG_COUNT":                 {},
	"GIT_CONFIG_PARAMETERS":            {},
	"GIT_DIR":                          {},
	"GIT_GRAFT_FILE":                   {},
	"GIT_IMPLICIT_WORK_TREE":           {},
	"GIT_INDEX_FILE":                   {},
	"GIT_NO_REPLACE_OBJECTS":           {},
	"GIT_OBJECT_DIRECTORY":             {},
	"GIT_PREFIX":                       {},
	"GIT_REPLACE_REF_BASE":             {},
	"GIT_SHALLOW_FILE":                 {},
	"GIT_WORK_TREE":                    {},
}

// sanitizedEnv returns the process environment with every repository-local git
// variable removed, so a native git call is governed by its working directory alone.
func sanitizedEnv() []string {
	parent := os.Environ()
	env := make([]string, 0, len(parent))
	for _, kv := range parent {
		name, _, found := strings.Cut(kv, "=")
		if found {
			if _, isRepoLocal := repoLocalEnvVars[name]; isRepoLocal {
				continue
			}
		}
		env = append(env, kv)
	}
	return env
}

// gitCommand builds a native git command rooted at the repository root, with any
// inherited repository-local git variables stripped from its environment.
func (r *Repository) gitCommand(ctx context.Context, args ...string) *exec.Cmd {
	// #nosec G204 - args are built from constants and values that come from git itself
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.gitRoot
	cmd.Env = sanitizedEnv()
	return cmd
}
