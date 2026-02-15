package urlutil_test

import (
	"testing"

	"github.com/sgaunet/auto-mr/internal/urlutil"
)

func TestExtractPathComponents(t *testing.T) {
	tests := []struct {
		name           string
		url            string
		componentCount int
		want           string
	}{
		// GitHub HTTPS URLs (caller has already trimmed .git)
		{
			name:           "github_https",
			url:            "https://github.com/owner/repo",
			componentCount: 2,
			want:           "owner/repo",
		},
		{
			name:           "github_https_with_www",
			url:            "https://www.github.com/owner/repo",
			componentCount: 2,
			want:           "owner/repo",
		},

		// GitHub SSH URLs (caller has already trimmed .git)
		{
			name:           "github_ssh_colon",
			url:            "git@github.com:owner/repo",
			componentCount: 2,
			want:           "owner/repo",
		},
		{
			name:           "github_ssh_protocol",
			url:            "ssh://git@github.com/owner/repo",
			componentCount: 2,
			want:           "owner/repo",
		},

		// GitLab HTTPS URLs (2 components)
		{
			name:           "gitlab_https_2_components",
			url:            "https://gitlab.com/group/project",
			componentCount: 2,
			want:           "group/project",
		},

		// GitLab HTTPS URLs (3 components - nested groups)
		{
			name:           "gitlab_https_3_components",
			url:            "https://gitlab.com/group/subgroup/project",
			componentCount: 3,
			want:           "group/subgroup/project",
		},
		{
			name:           "gitlab_https_3_components_extract_2",
			url:            "https://gitlab.com/group/subgroup/project",
			componentCount: 2,
			want:           "subgroup/project",
		},

		// GitLab HTTPS URLs (4 components - deeply nested)
		{
			name:           "gitlab_https_4_components",
			url:            "https://gitlab.com/group/subgroup1/subgroup2/project",
			componentCount: 4,
			want:           "group/subgroup1/subgroup2/project",
		},
		{
			name:           "gitlab_https_4_components_extract_3",
			url:            "https://gitlab.com/group/subgroup1/subgroup2/project",
			componentCount: 3,
			want:           "subgroup1/subgroup2/project",
		},

		// GitLab SSH URLs
		{
			name:           "gitlab_ssh_colon_2_components",
			url:            "git@gitlab.com:group/project",
			componentCount: 2,
			want:           "group/project",
		},
		{
			name:           "gitlab_ssh_colon_3_components",
			url:            "git@gitlab.com:group/subgroup/project",
			componentCount: 3,
			want:           "group/subgroup/project",
		},
		{
			name:           "gitlab_ssh_protocol_2_components",
			url:            "ssh://git@gitlab.com/group/project",
			componentCount: 2,
			want:           "group/project",
		},
		{
			name:           "gitlab_ssh_protocol_3_components",
			url:            "ssh://git@gitlab.com/group/subgroup/project",
			componentCount: 3,
			want:           "group/subgroup/project",
		},

		// Custom domain URLs
		{
			name:           "custom_domain_https",
			url:            "https://git.company.com/team/project",
			componentCount: 2,
			want:           "team/project",
		},
		{
			name:           "custom_domain_ssh_colon",
			url:            "git@git.company.com:team/project",
			componentCount: 2,
			want:           "team/project",
		},

		// Edge cases - special characters
		{
			name:           "special_chars_hyphens",
			url:            "https://github.com/my-org/my-repo",
			componentCount: 2,
			want:           "my-org/my-repo",
		},
		{
			name:           "special_chars_underscores",
			url:            "https://github.com/my_org/my_repo",
			componentCount: 2,
			want:           "my_org/my_repo",
		},
		{
			name:           "special_chars_dots",
			url:            "https://github.com/my.org/my.repo",
			componentCount: 2,
			want:           "my.org/my.repo",
		},
		{
			name:           "special_chars_numbers",
			url:            "https://github.com/org123/repo456",
			componentCount: 2,
			want:           "org123/repo456",
		},

		// Edge cases - invalid/insufficient components
		{
			name:           "insufficient_components_https",
			url:            "https://github.com/single",
			componentCount: 2,
			want:           "github.com/single", // Original behavior: extracts last 2 slash-separated parts
		},
		{
			name:           "insufficient_components_ssh",
			url:            "git@github.com:single",
			componentCount: 2,
			want:           "single",
		},
		{
			name:           "ssh_protocol_insufficient_components",
			url:            "ssh://git@github.com/single",
			componentCount: 10, // Request more components than available
			want:           "",
		},
		{
			name:           "ssh_colon_no_colon",
			url:            "git@github.com",
			componentCount: 2,
			want:           "", // No colon present
		},
		{
			name:           "empty_url",
			url:            "",
			componentCount: 2,
			want:           "",
		},
		{
			name:           "invalid_url_no_slashes",
			url:            "not-a-url",
			componentCount: 2,
			want:           "",
		},

		// Edge cases - component count variations
		{
			name:           "extract_1_component",
			url:            "https://github.com/owner/repo",
			componentCount: 1,
			want:           "repo",
		},
		{
			name:           "extract_more_than_available",
			url:            "https://github.com/owner/repo",
			componentCount: 5,
			want:           "https://github.com/owner/repo", // Original behavior: extracts all parts if componentCount >= total parts
		},

		// Real-world examples from different platforms
		{
			name:           "bitbucket_https",
			url:            "https://bitbucket.org/workspace/repo",
			componentCount: 2,
			want:           "workspace/repo",
		},
		{
			name:           "bitbucket_ssh",
			url:            "git@bitbucket.org:workspace/repo",
			componentCount: 2,
			want:           "workspace/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := urlutil.ExtractPathComponents(tt.url, tt.componentCount)
			if got != tt.want {
				t.Errorf("ExtractPathComponents(%q, %d) = %q, want %q",
					tt.url, tt.componentCount, got, tt.want)
			}
		})
	}
}

func TestExtractPathComponents_Consistency(t *testing.T) {
	// Verify that HTTPS and SSH colon formats produce identical results
	// when extracting the appropriate number of components.
	// Note: SSH colon format returns the full path after the colon (ignores componentCount),
	// so we need to match HTTPS extraction to the actual path length.
	tests := []struct {
		name           string
		https          string
		ssh            string
		componentCount int
		want           string
	}{
		{
			name:           "github_owner_repo",
			https:          "https://github.com/owner/repo",
			ssh:            "git@github.com:owner/repo",
			componentCount: 2, // Extract "owner/repo"
			want:           "owner/repo",
		},
		{
			name:           "gitlab_nested_3_levels",
			https:          "https://gitlab.com/group/subgroup/project",
			ssh:            "git@gitlab.com:group/subgroup/project",
			componentCount: 3, // Extract "group/subgroup/project"
			want:           "group/subgroup/project",
		},
		{
			name:           "custom_domain",
			https:          "https://git.company.com/team/repo",
			ssh:            "git@git.company.com:team/repo",
			componentCount: 2, // Extract "team/repo"
			want:           "team/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpsResult := urlutil.ExtractPathComponents(tt.https, tt.componentCount)
			sshResult := urlutil.ExtractPathComponents(tt.ssh, tt.componentCount)

			if httpsResult != tt.want {
				t.Errorf("HTTPS result incorrect: got %q, want %q", httpsResult, tt.want)
			}

			if sshResult != tt.want {
				t.Errorf("SSH result incorrect: got %q, want %q", sshResult, tt.want)
			}

			if httpsResult != sshResult {
				t.Errorf("HTTPS and SSH results differ: HTTPS=%q, SSH=%q",
					httpsResult, sshResult)
			}
		})
	}
}
