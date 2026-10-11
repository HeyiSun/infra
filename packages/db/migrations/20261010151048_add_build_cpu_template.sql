-- +goose Up
ALTER TABLE public.env_builds ADD COLUMN cpu_template JSONB
    CHECK (jsonb_typeof(cpu_template) = 'object');

COMMENT ON COLUMN public.env_builds.cpu_template IS
    'Complete applied custom CPU template JSON; NULL when no configuration is reported. Does not certify CPU compatibility.';

-- +goose Down
ALTER TABLE public.env_builds DROP COLUMN cpu_template;
