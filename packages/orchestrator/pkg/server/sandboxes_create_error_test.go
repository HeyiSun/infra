//go:build linux

package server

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	"github.com/e2b-dev/infra/packages/shared/pkg/storage"
)

func TestSandboxCreateErrorCode(t *testing.T) {
	t.Parallel()

	// Create uses a custom cause that does not wrap context.DeadlineExceeded.
	deadlineCtx, cancel := context.WithDeadlineCause(t.Context(), time.Now().Add(-time.Second), errors.New("request timed out"))
	t.Cleanup(cancel)
	require.ErrorIs(t, deadlineCtx.Err(), context.DeadlineExceeded)
	require.NotErrorIs(t, context.Cause(deadlineCtx), context.DeadlineExceeded)

	canceledCtx, cancelCause := context.WithCancelCause(t.Context())
	cancelCause(errors.New("caller canceled the request"))

	bootErr := errors.New("failed to load snapshot")
	missingFiles := fmt.Errorf("load rootfs: %w", storage.ErrObjectNotExist)

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want codes.Code
	}{
		{"hard failure", t.Context(), bootErr, codes.Internal},
		{"nested timeout with live request", t.Context(), fmt.Errorf("load snapshot: %w", context.DeadlineExceeded), codes.Internal},
		{"request deadline with custom cause", deadlineCtx, bootErr, codes.DeadlineExceeded},
		{"request canceled with custom cause", canceledCtx, bootErr, codes.Canceled},
		{"missing snapshot", t.Context(), missingFiles, codes.FailedPrecondition},
		{"missing snapshot before deadline", deadlineCtx, missingFiles, codes.FailedPrecondition},
		{"missing snapshot before cancellation", canceledCtx, missingFiles, codes.FailedPrecondition},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, sandboxCreateErrorCode(tt.ctx, tt.err))
		})
	}
}
