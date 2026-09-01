package main

// This is the only white-box test file in the repository, and necessarily so:
// package main has no import path, so no external test package can import it. The
// helpers below are pure and cheap to test, and their edge cases -- label limits,
// timeout precedence -- are exactly the kind that regress silently.

import (
	"errors"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/sgaunet/auto-mr/internal/logger"
	"github.com/sgaunet/auto-mr/pkg/config"
	"github.com/sgaunet/auto-mr/pkg/platform"
)

func TestParseLabels(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "single", input: "bug", want: []string{"bug"}},
		{name: "multiple", input: "bug,enhancement", want: []string{"bug", "enhancement"}},
		{name: "surrounding_whitespace_trimmed", input: " bug , enhancement ", want: []string{"bug", "enhancement"}},
		{name: "empty_entries_dropped", input: "bug,,enhancement", want: []string{"bug", "enhancement"}},
		{name: "trailing_comma", input: "bug,", want: []string{"bug"}},
		{name: "only_separators", input: ",,,", want: nil},
		{name: "empty", input: "", want: nil},
		{name: "whitespace_only", input: "   ", want: nil},
		// Label names legitimately contain spaces, so only the edges are trimmed.
		{name: "inner_spaces_preserved", input: "good first issue", want: []string{"good first issue"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLabels(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseLabels(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseLabels(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestValidateManualLabels(t *testing.T) {
	available := []platform.Label{
		{Name: "bug"},
		{Name: "enhancement"},
		{Name: "documentation"},
		{Name: "good first issue"},
	}

	tests := []struct {
		name      string
		requested string
		wantLen   int
		wantErr   error
	}{
		{name: "single_known_label", requested: "bug", wantLen: 1},
		{name: "several_known_labels", requested: "bug,enhancement", wantLen: 2},
		{name: "whitespace_is_tolerated", requested: " bug , enhancement ", wantLen: 2},
		{name: "label_with_spaces", requested: "good first issue", wantLen: 1},
		// An empty value means "no labels", which is different from an invalid one.
		{name: "empty_means_no_labels", requested: "", wantLen: 0},
		{name: "unknown_label_is_rejected", requested: "nonexistent", wantErr: errLabelNotFound},
		{name: "one_unknown_among_known", requested: "bug,nonexistent", wantErr: errLabelNotFound},
		{name: "case_sensitive", requested: "Bug", wantErr: errLabelNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateManualLabels(available, tt.requested)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Errorf("got %d labels (%v), want %d", len(got), got, tt.wantLen)
			}
		})
	}
}

// TestValidateManualLabels_TooMany covers the selection cap. The requested labels are
// all valid, so only the count can be responsible for the rejection.
func TestValidateManualLabels_TooMany(t *testing.T) {
	available := make([]platform.Label, 0, maxLabelsToSelect+1)
	requested := ""
	for i := 0; i <= maxLabelsToSelect; i++ {
		name := string(rune('a' + i))
		available = append(available, platform.Label{Name: name})
		if requested != "" {
			requested += ","
		}
		requested += name
	}

	_, err := validateManualLabels(available, requested)
	if !errors.Is(err, errTooManyLabels) {
		t.Errorf("got %v, want errTooManyLabels for %d labels", err, maxLabelsToSelect+1)
	}
}

func TestGetStatusIcon(t *testing.T) {
	tests := []struct {
		name      string
		completed bool
		err       error
		want      string
	}{
		{name: "completed", completed: true, want: "✓"},
		{name: "not_attempted", completed: false, want: "—"},
		{name: "failed", completed: false, err: errors.New("boom"), want: "✗"},
		// An error wins over the completed flag: a step that errored did not succeed,
		// however far it got.
		{name: "error_outranks_completed", completed: true, err: errors.New("boom"), want: "✗"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getStatusIcon(tt.completed, tt.err); got != tt.want {
				t.Errorf("getStatusIcon(%v, %v) = %q, want %q", tt.completed, tt.err, got, tt.want)
			}
		})
	}
}

// newTimeoutCmd builds a command carrying just the pipeline-timeout flag.
//
// The shared rootCmd is deliberately not reused: its flag state would persist between
// cases and leak across tests.
func newTimeoutCmd(t *testing.T, flagValue string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringVar(&pipelineTimeout, "pipeline-timeout", "", "")

	if flagValue != "" {
		if err := cmd.Flags().Set("pipeline-timeout", flagValue); err != nil {
			t.Fatalf("set flag: %v", err)
		}
	}
	t.Cleanup(func() { pipelineTimeout = "" })
	return cmd
}

func TestGetPipelineTimeout(t *testing.T) {
	// The config fallback path logs a warning through the package-level logger, which
	// is nil until runAutoMR assigns it.
	log = logger.NoLogger()

	tests := []struct {
		name         string
		flag         string
		configValue  string
		want         time.Duration
		wantErr      bool
		wantSentinel error
	}{
		// Priority 1: the flag wins over everything.
		{name: "flag_beats_config", flag: "10m", configValue: "45m", want: 10 * time.Minute},
		{name: "flag_alone", flag: "2h", want: 2 * time.Hour},
		// Priority 2: config applies when no flag was given.
		{name: "config_when_no_flag", configValue: "45m", want: 45 * time.Minute},
		// Priority 3: the built-in default.
		{name: "default_when_neither", want: defaultPipelineTimeout},
		// An unparseable config value must not abort the run; it falls back rather
		// than failing, because Validate should already have rejected it.
		{name: "unparseable_config_falls_back", configValue: "not-a-duration", want: defaultPipelineTimeout},
		// A bad flag is the user's immediate input, so it is reported.
		{name: "unparseable_flag_is_an_error", flag: "not-a-duration", wantErr: true},
		{name: "flag_below_minimum", flag: "1s", wantErr: true, wantSentinel: config.ErrInvalidTimeout},
		{name: "flag_above_maximum", flag: "9h", wantErr: true, wantSentinel: config.ErrInvalidTimeout},
		{name: "flag_at_minimum", flag: "1m", want: config.MinPipelineTimeout},
		{name: "flag_at_maximum", flag: "8h", want: config.MaxPipelineTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newTimeoutCmd(t, tt.flag)

			got, err := getPipelineTimeout(cmd, tt.configValue)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				if tt.wantSentinel != nil && !errors.Is(err, tt.wantSentinel) {
					t.Errorf("got %v, want it to wrap %v", err, tt.wantSentinel)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFormatConfigError checks the guidance shown when configuration is unusable.
// These messages are the user's only cue about what to fix, so they must name the
// missing key rather than surface a bare wrapped error.
func TestFormatConfigError(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "config_not_found", err: config.ErrConfigNotFound},
		{name: "gitlab_assignee_empty", err: config.ErrGitLabAssigneeEmpty},
		{name: "github_reviewer_empty", err: config.ErrGitHubReviewerEmpty},
		{name: "gitlab_assignee_invalid", err: config.ErrGitLabAssigneeInvalid},
		{name: "forgejo_url_invalid", err: config.ErrForgejoURLInvalid},
		{name: "invalid_timeout", err: config.ErrInvalidTimeout},
		{name: "timeout_too_small", err: config.ErrTimeoutTooSmall},
		{name: "unrelated_error", err: errors.New("something else")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatConfigError(tt.err)
			if got == nil {
				t.Fatal("formatConfigError returned nil for a non-nil error")
			}
			if got.Error() == "" {
				t.Error("formatConfigError produced an empty message")
			}
		})
	}
}
