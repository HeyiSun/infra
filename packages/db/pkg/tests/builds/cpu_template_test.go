package builds

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/e2b-dev/infra/packages/db/pkg/testutils"
	"github.com/e2b-dev/infra/packages/db/pkg/types"
	"github.com/e2b-dev/infra/packages/db/queries"
)

func TestFinishTemplateBuild_CPUTemplate(t *testing.T) {
	t.Parallel()
	db := testutils.SetupDatabase(t)
	teamID := testutils.CreateTestTeam(t, db)
	const settings = `{"cpuid_modifiers":[{"leaf":"0x7","subleaf":"0x0","modifiers":[{"register":"ebx","bitmap":"0b0x"}]}],"kvm_capabilities":["!122","!121"],"x86_tsc_khz":3200000}`

	for _, tt := range []struct {
		name       string
		template   json.RawMessage
		failed     bool
		want       string
		wantStatus types.BuildStatus
	}{
		{name: "complete nested configuration", template: json.RawMessage(settings), want: settings, wantStatus: types.BuildStatusUploaded},
		{name: "no reported configuration", wantStatus: types.BuildStatusUploaded},
		{name: "late completion of failed build", template: json.RawMessage(settings), failed: true, wantStatus: types.BuildStatusFailed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			templateID := testutils.CreateTestTemplate(t, db, teamID)
			buildID := testutils.CreateTestBuild(t, ctx, db, templateID, "building")
			testutils.CreateTestBuildAssignment(t, ctx, db, templateID, buildID, "cpu-template")
			if tt.failed {
				require.True(t, failBuild(t, ctx, db, buildID, "build timed out"))
			}

			err := db.SqlcClient.FinishTemplateBuild(ctx, queries.FinishTemplateBuildParams{
				BuildID:     buildID,
				Status:      types.BuildStatusUploaded,
				CpuTemplate: tt.template,
			})
			require.NoError(t, err)

			build, err := db.SqlcClient.GetTemplateBuildWithTemplate(ctx, queries.GetTemplateBuildWithTemplateParams{
				TemplateID: templateID,
				BuildID:    buildID,
			})
			require.NoError(t, err)
			if tt.want == "" {
				assert.Nil(t, build.EnvBuild.CpuTemplate)
			} else {
				assert.JSONEq(t, tt.want, string(build.EnvBuild.CpuTemplate))
			}
			assert.Equal(t, tt.wantStatus, build.EnvBuild.Status)
		})
	}
}
