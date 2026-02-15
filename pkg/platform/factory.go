package platform

import (
	"errors"
	"fmt"

	"github.com/sgaunet/auto-mr/pkg/config"
	"github.com/sgaunet/auto-mr/pkg/git"
	ghclient "github.com/sgaunet/auto-mr/pkg/github"
	"github.com/sgaunet/auto-mr/pkg/gitlab"
	"github.com/sgaunet/bullets"
)

// errUnsupportedPlatform is returned when the detected platform is not supported.
var errUnsupportedPlatform = errors.New("unsupported platform")

// NewProvider creates the appropriate Provider implementation based on the detected platform.
//
//nolint:ireturn // Factory function must return interface to enable platform abstraction.
func NewProvider(p git.Platform, cfg *config.Config, logger *bullets.Logger) (Provider, error) {
	switch p {
	case git.PlatformGitLab:
		client, err := gitlab.NewClient()
		if err != nil {
			return nil, fmt.Errorf("failed to create GitLab client: %w", err)
		}
		client.SetLogger(logger)
		return NewGitLabAdapter(client, cfg.GitLab, logger), nil

	case git.PlatformGitHub:
		client, err := ghclient.NewClient()
		if err != nil {
			return nil, fmt.Errorf("failed to create GitHub client: %w", err)
		}
		client.SetLogger(logger)
		return NewGitHubAdapter(client, cfg.GitHub, logger), nil

	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedPlatform, p)
	}
}
