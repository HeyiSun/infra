package template_manager

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	templatemanagergrpc "github.com/e2b-dev/infra/packages/shared/pkg/grpc/template-manager"
	"github.com/e2b-dev/infra/packages/shared/pkg/logger"
)

func TestPollBuildStatus_ForwardsCPUTemplate(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		template string
	}{
		{name: "reported template", template: `{"kvm_capabilities":["!121"],"x86_tsc_khz":3200000}`},
		{name: "no template"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var reported string
			called := false
			client := &fakeTemplateManagerClient{
				onSetFinished: func(template string) {
					called = true
					reported = template
				},
			}
			checker := &PollBuildStatus{client: client, logger: logger.NewNopLogger(), buildID: uuid.New()}
			completed, err := checker.dispatchBasedOnStatus(t.Context(), &templatemanagergrpc.TemplateBuildStatusResponse{
				Status: templatemanagergrpc.TemplateBuildState_Completed,
				Metadata: &templatemanagergrpc.TemplateBuildMetadata{
					RootfsSizeKey:  1024,
					EnvdVersionKey: "v1.0.0",
					CpuTemplate:    tt.template,
				},
			})
			require.NoError(t, err)
			require.True(t, completed)
			require.True(t, called)
			require.Equal(t, tt.template, reported)
		})
	}
}
