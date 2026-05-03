package activation

import (
	"context"
	"fmt"
	"os"

	"github.com/allensu/loki-profile-manager/internal/infisical"
)

type SecretProvider interface {
	GetSecrets(ctx context.Context, names []string) (map[string]string, error)
}

func RenderToFile(ctx context.Context, provider SecretProvider, source, target string, declaredSecrets []string) error {
	if provider == nil {
		return fmt.Errorf("render %s: secret provider is required", source)
	}
	template, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read template %s: %w", source, err)
	}
	required := infisical.RequiredSecrets(template, declaredSecrets)
	if err := infisical.ValidateSecretNames(required); err != nil {
		return err
	}
	values, err := provider.GetSecrets(ctx, required)
	if err != nil {
		return err
	}
	rendered, err := infisical.RenderTemplate(template, values, required)
	if err != nil {
		return err
	}
	return writeFileAtomic(target, rendered, 0o600)
}
