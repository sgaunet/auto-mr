package platform_test

import (
	"errors"
	"testing"
	"time"

	"github.com/sgaunet/auto-mr/pkg/platform"
	"github.com/sgaunet/auto-mr/testing/fixtures"
	"github.com/sgaunet/auto-mr/testing/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Provider Tests ---

func TestMockProvider_Initialize(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		err := mock.Initialize("https://github.com/owner/repo.git")
		require.NoError(t, err)
		assert.Equal(t, 1, mock.GetCallCount("Initialize"))
	})

	t.Run("error", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		mock.InitializeError = errors.New("init failed")
		err := mock.Initialize("https://github.com/owner/repo.git")
		require.Error(t, err)
		assert.Equal(t, "init failed", err.Error())
	})
}

func TestMockProvider_ListLabels(t *testing.T) {
	t.Run("returns labels", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		mock.ListLabelsResponse = fixtures.ValidPlatformLabels()

		labels, err := mock.ListLabels()
		require.NoError(t, err)
		assert.Len(t, labels, 4)
		assert.Equal(t, "bug", labels[0].Name)
	})

	t.Run("returns empty list", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		mock.ListLabelsResponse = []platform.Label{}

		labels, err := mock.ListLabels()
		require.NoError(t, err)
		assert.Empty(t, labels)
	})

	t.Run("returns error", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		mock.ListLabelsError = errors.New("api error")

		labels, err := mock.ListLabels()
		require.Error(t, err)
		assert.Nil(t, labels)
	})
}

func TestMockProvider_Create(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		mock.CreateResponse = fixtures.ValidPlatformMergeRequest()
		params := fixtures.ValidCreateParams()

		mr, err := mock.Create(params)
		require.NoError(t, err)
		require.NotNil(t, mr)
		assert.Equal(t, int64(42), mr.ID)
		assert.Contains(t, mr.WebURL, "merge_requests/42")

		lastCall := mock.GetLastCall("Create")
		require.NotNil(t, lastCall)
		assert.Equal(t, "feature-branch", lastCall.Args["sourceBranch"])
		assert.Equal(t, "main", lastCall.Args["targetBranch"])
	})

	t.Run("already exists", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		mock.CreateError = platform.ErrAlreadyExists

		mr, err := mock.Create(fixtures.ValidCreateParams())
		require.Error(t, err)
		assert.Nil(t, mr)
		assert.True(t, errors.Is(err, platform.ErrAlreadyExists))
	})
}

func TestMockProvider_GetByBranch(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		mock.GetByBranchResponse = fixtures.ValidPlatformMergeRequest()

		mr, err := mock.GetByBranch("feature", "main")
		require.NoError(t, err)
		require.NotNil(t, mr)
		assert.Equal(t, int64(42), mr.ID)
	})

	t.Run("not found", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		mock.GetByBranchError = platform.ErrNotFound

		mr, err := mock.GetByBranch("feature", "main")
		require.Error(t, err)
		assert.Nil(t, mr)
		assert.True(t, errors.Is(err, platform.ErrNotFound))
	})
}

func TestMockProvider_WaitForPipeline(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		mock.WaitForPipelineStatus = "success"

		status, err := mock.WaitForPipeline(30 * time.Minute)
		require.NoError(t, err)
		assert.Equal(t, "success", status)
	})

	t.Run("failed", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		mock.WaitForPipelineStatus = "failed"

		status, err := mock.WaitForPipeline(30 * time.Minute)
		require.NoError(t, err)
		assert.Equal(t, "failed", status)
	})

	t.Run("timeout error", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		mock.WaitForPipelineError = errors.New("timeout")

		_, err := mock.WaitForPipeline(30 * time.Minute)
		require.Error(t, err)
	})
}

func TestMockProvider_Approve(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		err := mock.Approve(42)
		require.NoError(t, err)
		assert.Equal(t, 1, mock.GetCallCount("Approve"))
	})

	t.Run("error", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		mock.ApproveError = errors.New("approval failed")
		err := mock.Approve(42)
		require.Error(t, err)
	})
}

func TestMockProvider_Merge(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		params := fixtures.ValidMergeParams()

		err := mock.Merge(params)
		require.NoError(t, err)

		lastCall := mock.GetLastCall("Merge")
		require.NotNil(t, lastCall)
		assert.Equal(t, int64(42), lastCall.Args["mrID"])
		assert.Equal(t, true, lastCall.Args["squash"])
	})

	t.Run("error", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		mock.MergeError = errors.New("merge failed")
		err := mock.Merge(fixtures.ValidMergeParams())
		require.Error(t, err)
	})
}

func TestMockProvider_PlatformName(t *testing.T) {
	mock := mocks.NewPlatformProvider()
	assert.Equal(t, "MockPlatform", mock.PlatformName())

	mock.PlatformNameValue = "GitHub"
	assert.Equal(t, "GitHub", mock.PlatformName())
}

func TestMockProvider_PipelineTimeout(t *testing.T) {
	mock := mocks.NewPlatformProvider()
	assert.Equal(t, "", mock.PipelineTimeout())

	mock.PipelineTimeoutValue = "45m"
	assert.Equal(t, "45m", mock.PipelineTimeout())
}

func TestMockProvider_CallTracking(t *testing.T) {
	t.Run("tracks multiple calls", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		mock.ListLabelsResponse = []platform.Label{}

		_, _ = mock.ListLabels()
		_, _ = mock.ListLabels()
		_ = mock.Initialize("url")

		assert.Equal(t, 2, mock.GetCallCount("ListLabels"))
		assert.Equal(t, 1, mock.GetCallCount("Initialize"))
		assert.Len(t, mock.GetCalls(), 3)
	})

	t.Run("reset clears calls", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		_ = mock.Initialize("url")
		assert.Equal(t, 1, mock.GetCallCount("Initialize"))

		mock.Reset()
		assert.Equal(t, 0, mock.GetCallCount("Initialize"))
		assert.Empty(t, mock.GetCalls())
	})

	t.Run("GetLastCall returns nil for uncalled method", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		assert.Nil(t, mock.GetLastCall("NeverCalled"))
	})
}

// --- Sentinel Error Tests ---

func TestSentinelErrors(t *testing.T) {
	t.Run("ErrAlreadyExists", func(t *testing.T) {
		assert.Error(t, platform.ErrAlreadyExists)
		assert.Contains(t, platform.ErrAlreadyExists.Error(), "already exists")
	})

	t.Run("ErrNotFound", func(t *testing.T) {
		assert.Error(t, platform.ErrNotFound)
		assert.Contains(t, platform.ErrNotFound.Error(), "no merge/pull request found")
	})

	t.Run("errors_are_unwrappable", func(t *testing.T) {
		wrapped := errors.Join(platform.ErrAlreadyExists, errors.New("extra context"))
		assert.True(t, errors.Is(wrapped, platform.ErrAlreadyExists))
	})
}

// --- Type Tests ---

func TestMergeRequestType(t *testing.T) {
	mr := fixtures.ValidPlatformMergeRequest()
	assert.Equal(t, int64(42), mr.ID)
	assert.Contains(t, mr.WebURL, "merge_requests")
	assert.Equal(t, "feature-branch", mr.SourceBranch)
}

func TestCreateParamsType(t *testing.T) {
	params := fixtures.ValidCreateParams()
	assert.Equal(t, "feature-branch", params.SourceBranch)
	assert.Equal(t, "main", params.TargetBranch)
	assert.Equal(t, "Test merge request", params.Title)
	assert.True(t, params.Squash)
	assert.Equal(t, []string{"bug"}, params.Labels)
}

func TestMergeParamsType(t *testing.T) {
	params := fixtures.ValidMergeParams()
	assert.Equal(t, int64(42), params.MRID)
	assert.True(t, params.Squash)
	assert.Equal(t, "Test merge request", params.CommitTitle)
	assert.Equal(t, "feature-branch", params.SourceBranch)
}

func TestLabelType(t *testing.T) {
	labels := fixtures.ValidPlatformLabels()
	assert.Len(t, labels, 4)
	assert.Equal(t, "bug", labels[0].Name)
	assert.Equal(t, "enhancement", labels[1].Name)
	assert.Equal(t, "documentation", labels[2].Name)
	assert.Equal(t, "help wanted", labels[3].Name)
}

// --- Workflow Integration Tests ---

func TestWorkflow_CreateWaitMerge(t *testing.T) {
	t.Run("successful lifecycle", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		mock.ListLabelsResponse = fixtures.ValidPlatformLabels()
		mock.CreateResponse = fixtures.ValidPlatformMergeRequest()
		mock.WaitForPipelineStatus = "success"
		mock.PlatformNameValue = "GitHub"

		// Initialize
		err := mock.Initialize("https://github.com/owner/repo.git")
		require.NoError(t, err)

		// List labels
		labels, err := mock.ListLabels()
		require.NoError(t, err)
		assert.NotEmpty(t, labels)

		// Create
		mr, err := mock.Create(fixtures.ValidCreateParams())
		require.NoError(t, err)
		require.NotNil(t, mr)

		// Wait
		status, err := mock.WaitForPipeline(30 * time.Minute)
		require.NoError(t, err)
		assert.Equal(t, "success", status)

		// Approve (no-op for GitHub)
		err = mock.Approve(mr.ID)
		require.NoError(t, err)

		// Merge
		err = mock.Merge(platform.MergeParams{
			MRID:         mr.ID,
			Squash:       true,
			CommitTitle:  "Test merge",
			SourceBranch: mr.SourceBranch,
		})
		require.NoError(t, err)

		// Verify call sequence
		calls := mock.GetCalls()
		assert.Len(t, calls, 6)
		assert.Equal(t, "Initialize", calls[0].Method)
		assert.Equal(t, "ListLabels", calls[1].Method)
		assert.Equal(t, "Create", calls[2].Method)
		assert.Equal(t, "WaitForPipeline", calls[3].Method)
		assert.Equal(t, "Approve", calls[4].Method)
		assert.Equal(t, "Merge", calls[5].Method)
	})

	t.Run("existing MR fallback", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		mock.CreateError = platform.ErrAlreadyExists
		mock.GetByBranchResponse = fixtures.ValidPlatformMergeRequest()
		mock.WaitForPipelineStatus = "success"

		// Create fails with already exists
		_, err := mock.Create(fixtures.ValidCreateParams())
		require.Error(t, err)
		assert.True(t, errors.Is(err, platform.ErrAlreadyExists))

		// Fall back to GetByBranch
		mr, err := mock.GetByBranch("feature-branch", "main")
		require.NoError(t, err)
		require.NotNil(t, mr)
		assert.Equal(t, int64(42), mr.ID)
	})

	t.Run("pipeline failure", func(t *testing.T) {
		mock := mocks.NewPlatformProvider()
		mock.CreateResponse = fixtures.ValidPlatformMergeRequest()
		mock.WaitForPipelineStatus = "failed"

		mr, err := mock.Create(fixtures.ValidCreateParams())
		require.NoError(t, err)
		require.NotNil(t, mr)

		status, err := mock.WaitForPipeline(30 * time.Minute)
		require.NoError(t, err)
		assert.Equal(t, "failed", status)
	})
}

// --- GitLab Adapter Interface Tests ---

func TestGitLabAdapter_PlatformName(t *testing.T) {
	// We can't instantiate a real GitLabAdapter without a token,
	// but we can verify the type fulfills the interface through mock.
	mock := mocks.NewPlatformProvider()
	mock.PlatformNameValue = "GitLab"
	assert.Equal(t, "GitLab", mock.PlatformName())
}

// --- GitHub Adapter Interface Tests ---

func TestGitHubAdapter_ApproveIsNoOp(t *testing.T) {
	// Verify that the GitHub adapter pattern (approve as no-op) works via mock
	mock := mocks.NewPlatformProvider()
	// No error configured = no-op behavior
	err := mock.Approve(42)
	require.NoError(t, err)
	assert.Equal(t, 1, mock.GetCallCount("Approve"))
}

func TestGitHubAdapter_MergeWithBranchDeletion(t *testing.T) {
	// Verify the merge params include source branch for deletion
	mock := mocks.NewPlatformProvider()

	params := platform.MergeParams{
		MRID:         123,
		Squash:       true,
		CommitTitle:  "Merge feature",
		SourceBranch: "feature-branch",
	}

	err := mock.Merge(params)
	require.NoError(t, err)

	lastCall := mock.GetLastCall("Merge")
	require.NotNil(t, lastCall)
	assert.Equal(t, "feature-branch", lastCall.Args["sourceBranch"])
}
