//go:build linux

package build

import (
	"github.com/e2b-dev/infra/packages/orchestrator/pkg/sandbox/fc/cputemplate"
	"github.com/e2b-dev/infra/packages/orchestrator/pkg/template/build/buildcontext"
	"github.com/e2b-dev/infra/packages/orchestrator/pkg/template/build/builderrors"
	"github.com/e2b-dev/infra/packages/orchestrator/pkg/template/build/metrics"
	"github.com/e2b-dev/infra/packages/orchestrator/pkg/template/metadata"
	orchestratorgrpc "github.com/e2b-dev/infra/packages/shared/pkg/grpc/orchestrator"
	"github.com/e2b-dev/infra/packages/shared/pkg/units"
)

func newResult(bc buildcontext.BuildContext, finalMetadata metadata.Template, rootfsSize uint64, scheduling *orchestratorgrpc.SchedulingMetadata) *Result {
	return &Result{
		EnvdVersion:        bc.EnvdVersion,
		KernelVersion:      bc.Config.KernelVersion,
		FirecrackerVersion: bc.Config.FirecrackerVersion,
		// Record the final artifact's settings, not the requested flag or its ancestor's settings.
		CPUTemplate:        cputemplate.AppliedJSON(finalMetadata.CPUTemplate),
		RootfsSizeMB:       units.BytesToMB(int64(rootfsSize)),
		SchedulingMetadata: scheduling,
	}
}

// ClassifyBuildResult maps the outcome of Builder.Build to the value used both
// as the `result` label on template_build_result_total and as the build.result
// span attribute. It is the single place the user-vs-internal decision is made.
//
//   - success: no error and a result was produced.
//   - user_error: the error chain contains a PhaseBuildError (a failed user
//     command, script, image pull, or a build-level cancellation/timeout).
//   - internal_error: anything else.
func ClassifyBuildResult(r *Result, err error) metrics.BuildResultType {
	if err == nil && r != nil {
		return metrics.BuildResultSuccess
	}

	if builderrors.IsUserError(err) {
		return metrics.BuildResultUserError
	}

	return metrics.BuildResultInternalError
}
