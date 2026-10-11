package template_manager

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestNormalizeCPUTemplate(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "no template"},
		{name: "whitespace", raw: " \n\t"},
		{name: "null", raw: " null "},
		{name: "empty object", raw: " { } "},
		{
			name: "object formatting",
			raw:  "{\n \"x86_tsc_khz\": 3200000, \"kvm_capabilities\": [\"!121\"]\n}",
			want: `{"kvm_capabilities":["!121"],"x86_tsc_khz":3200000}`,
		},
		{
			name: "nested keys and modifier order",
			raw:  `{"cpuid_modifiers":[{"subleaf":"0x0","modifiers":[{"register":"ebx","bitmap":"0b0x"},{"register":"eax","bitmap":"0b1x"}],"leaf":"0x7"}]}`,
			want: `{"cpuid_modifiers":[{"leaf":"0x7","modifiers":[{"bitmap":"0b0x","register":"ebx"},{"bitmap":"0b1x","register":"eax"}],"subleaf":"0x0"}]}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeCPUTemplate(tt.raw)
			require.NoError(t, err)
			if tt.want == "" {
				require.Nil(t, got, "absence must become SQL NULL, not JSON null")
			} else {
				require.Equal(t, tt.want, string(got))
			}
			again, err := normalizeCPUTemplate(string(got))
			require.NoError(t, err)
			require.Equal(t, got, again, "normalization must be idempotent")
		})
	}
}

func TestSetFinishedRejectsInvalidCPUTemplateBeforeDatabaseWrite(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{`{`, `[]`, `"not an object"`, `42`, `true`, `{} {}`, `{} trailing`} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			// A nil DB also proves validation precedes the terminal status write.
			tm := &TemplateManager{}
			err := tm.SetFinished(t.Context(), uuid.New(), 0, "", "", "", raw)
			require.ErrorContains(t, err, "invalid CPU template JSON")
		})
	}
}
