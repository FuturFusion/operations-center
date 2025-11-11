package terraform

import "context"

func WithTerraformInitFunc(initFunc func(ctx context.Context, configDir string) error) Option {
	return func(t *terraform) {
		t.terraformInitFunc = initFunc
	}
}

func WithTerraformApplyFunc(applyFunc func(ctx context.Context, configDir string) error) Option {
	return func(t *terraform) {
		t.terraformApplyFunc = applyFunc
	}
}
