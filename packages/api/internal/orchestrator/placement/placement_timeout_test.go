package placement

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/e2b-dev/infra/packages/api/internal/api"
	"github.com/e2b-dev/infra/packages/api/internal/orchestrator/nodemanager"
	"github.com/e2b-dev/infra/packages/shared/pkg/grpc/orchestrator"
)

// stubAlgorithm is a placement Algorithm whose chooseNode behavior is injected.
type stubAlgorithm struct {
	choose func(nodesExcluded map[string]struct{}) (*nodemanager.Node, error)
}

func (s stubAlgorithm) chooseNode(
	_ context.Context,
	_ []*nodemanager.Node,
	nodesExcluded map[string]struct{},
	_ nodemanager.SandboxResources,
	_ CPURequirement,
	_ FeatureRequirement,
	_ bool,
	_ []string,
) (*nodemanager.Node, error) {
	return s.choose(nodesExcluded)
}

func testSbxRequest(id string) *orchestrator.SandboxCreateRequest {
	return &orchestrator.SandboxCreateRequest{
		Sandbox: &orchestrator.SandboxConfig{
			SandboxId: id,
			Vcpu:      2,
			RamMb:     512,
		},
	}
}

// failIfCalled is an algorithm that fails the test if chooseNode is invoked.
func failIfCalled(t *testing.T) stubAlgorithm {
	t.Helper()

	return stubAlgorithm{
		choose: func(map[string]struct{}) (*nodemanager.Node, error) {
			t.Fatal("chooseNode should not be called")

			return nil, nil
		},
	}
}

// erroringClient returns err from SandboxCreate, optionally cancelling the
// request context first to simulate the deadline being hit mid-create.
func erroringClient(cancel context.CancelFunc, err error) *nodemanager.MockSandboxClientCustom {
	return &nodemanager.MockSandboxClientCustom{
		CreateFunc: func() error {
			if cancel != nil {
				cancel()
			}

			return err
		},
	}
}

// A cancelled request pins the warming node for both old orchestrators returning
// Internal and newer ones preserving the timeout/cancellation status.
func TestPlaceSandbox_TimeoutPinsFirstTriedNode(t *testing.T) {
	t.Parallel()

	for _, code := range []codes.Code{codes.Internal, codes.DeadlineExceeded, codes.Canceled} {
		t.Run(code.String(), func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			node := nodemanager.NewTestNode("node-test", api.NodeStatusReady, 0, 8)
			node.SetSandboxClient(erroringClient(cancel, status.Error(code, "failed to create sandbox: request timed out")))

			result, err := PlaceSandbox(
				ctx,
				failIfCalled(t),
				[]*nodemanager.Node{node},
				node,
				testSbxRequest("sbx-1"),
				CPURequirement{},
				false,
				nil,
			)

			require.Error(t, err)
			assert.True(t, result.TimedOut, "expected the failure to be reported as a timeout")
			require.NotNil(t, result.WarmedNode)
			assert.Equal(t, node.ID, result.WarmedNode.ID)
		})
	}
}

// TestPlaceSandbox_PinsFirstTriedNodeNotLater verifies that across multiple
// attempts the FIRST node tried is the one surfaced, not a later one.
func TestPlaceSandbox_PinsFirstTriedNodeNotLater(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())

	first := nodemanager.NewTestNode("node-first", api.NodeStatusReady, 0, 8)
	// First node fails without cancelling - a long pull that the orchestrator
	// gave up on while the request budget was still alive.
	first.SetSandboxClient(erroringClient(nil, status.Error(codes.Internal, "boom")))

	second := nodemanager.NewTestNode("node-second", api.NodeStatusReady, 0, 8)
	// Second node fails and the request budget runs out here.
	second.SetSandboxClient(erroringClient(cancel, status.Error(codes.Internal, "boom")))

	algo := stubAlgorithm{choose: func(map[string]struct{}) (*nodemanager.Node, error) {
		return second, nil
	}}

	result, err := PlaceSandbox(
		ctx,
		algo,
		[]*nodemanager.Node{first, second},
		first,
		testSbxRequest("sbx-2"),
		CPURequirement{},
		false,
		nil,
	)

	require.Error(t, err)
	assert.True(t, result.TimedOut)
	require.NotNil(t, result.WarmedNode)
	assert.Equal(t, first.ID, result.WarmedNode.ID, "must pin the first node tried, not a later one")
}

// A node timeout or cancellation does not mean the placement budget has expired.
func TestPlaceSandbox_LiveContextFailureNotPinned(t *testing.T) {
	t.Parallel()

	for _, code := range []codes.Code{codes.Internal, codes.DeadlineExceeded, codes.Canceled} {
		t.Run(code.String(), func(t *testing.T) {
			t.Parallel()

			node := nodemanager.NewTestNode("node-failed", api.NodeStatusReady, 0, 8)
			createErr := status.Error(code, "failed to create sandbox")
			node.SetSandboxClient(erroringClient(nil, createErr))

			result, err := PlaceSandbox(
				t.Context(),
				failIfCalled(t),
				[]*nodemanager.Node{node},
				node,
				testSbxRequest("sbx-3"),
				CPURequirement{},
				false,
				nil,
			)

			require.ErrorIs(t, err, createErr)
			assert.False(t, result.TimedOut, "a live-context failure must not be reported as a timeout")
			assert.Nil(t, result.WarmedNode, "a live-context failure must not surface a warming node")
		})
	}
}

func TestPlaceSandbox_NodeDeadlineRetriesWhileContextLive(t *testing.T) {
	t.Parallel()

	first := nodemanager.NewTestNode("node-timed-out", api.NodeStatusReady, 0, 8)
	first.SetSandboxClient(erroringClient(nil, status.Error(codes.DeadlineExceeded, "request timed out")))
	second := nodemanager.NewTestNode("node-ok", api.NodeStatusReady, 0, 8)

	algo := stubAlgorithm{choose: func(map[string]struct{}) (*nodemanager.Node, error) {
		return second, nil
	}}

	result, err := PlaceSandbox(
		t.Context(),
		algo,
		[]*nodemanager.Node{first, second},
		first,
		testSbxRequest("sbx-node-deadline"),
		CPURequirement{},
		false,
		nil,
	)

	require.NoError(t, err)
	require.NotNil(t, result.Node)
	assert.Equal(t, second.ID, result.Node.ID)
	assert.False(t, result.TimedOut)
	assert.Nil(t, result.WarmedNode)
}

// TestPlaceSandbox_ResourceExhaustedNotPinned verifies that a node which refused
// fast with ResourceExhausted is not pinned even if the request then times out.
func TestPlaceSandbox_ResourceExhaustedNotPinned(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())

	node := nodemanager.NewTestNode("node-exhausted", api.NodeStatusReady, 0, 8)
	node.SetSandboxClient(erroringClient(cancel, status.Error(codes.ResourceExhausted, "no capacity")))

	result, err := PlaceSandbox(
		ctx,
		failIfCalled(t),
		[]*nodemanager.Node{node},
		node,
		testSbxRequest("sbx-4"),
		CPURequirement{},
		false,
		nil,
	)

	require.Error(t, err)
	assert.Nil(t, result.WarmedNode, "a node that refused fast must not be pinned")
}

// TestPlaceSandbox_TimeoutBeforeAnyAttemptNotWrapped verifies that when the
// deadline fires before any node was tried there is nothing to pin to.
func TestPlaceSandbox_TimeoutBeforeAnyAttemptNotWrapped(t *testing.T) {
	t.Parallel()

	node := nodemanager.NewTestNode("node-ctx", api.NodeStatusReady, 0, 8)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // already past deadline before the first attempt

	result, err := PlaceSandbox(
		ctx,
		failIfCalled(t),
		[]*nodemanager.Node{node},
		node,
		testSbxRequest("sbx-5"),
		CPURequirement{},
		false,
		nil,
	)

	require.Error(t, err)
	assert.Nil(t, result.WarmedNode, "no node was tried, so there is nothing to pin")
}

func TestPlaceSandbox_SuccessReturnsNode(t *testing.T) {
	t.Parallel()

	node := nodemanager.NewTestNode("node-ok", api.NodeStatusReady, 0, 8)

	result, err := PlaceSandbox(
		t.Context(),
		failIfCalled(t),
		[]*nodemanager.Node{node},
		node,
		testSbxRequest("sbx-6"),
		CPURequirement{},
		false,
		nil,
	)

	require.NoError(t, err)
	require.NotNil(t, result.Node)
	assert.Equal(t, node.ID, result.Node.ID)
}
