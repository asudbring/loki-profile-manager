package app

import (
	"context"
	"fmt"
	"sort"

	"github.com/allensu/loki-profile-manager/internal/secrets"
)

type SecretsStatusRequest struct{}

type SecretsStatusResult = secrets.Status

type SecretsLoginRequest struct {
	Domain string
}

type SecretsCheckRequest struct {
	Names []string
}

type SecretsCheckResult struct {
	Provider  secrets.ProviderID `json:"provider"`
	Checked   []string           `json:"checked"`
	Available []string           `json:"available"`
	Missing   []string           `json:"missing"`
	Ready     bool               `json:"ready"`
}

func (s *Service) SecretsStatus(ctx context.Context, req SecretsStatusRequest) (SecretsStatusResult, error) {
	if s == nil {
		return SecretsStatusResult{}, fmt.Errorf("secrets status: service is nil")
	}
	return s.secretStatusChecker.CheckStatus(ctx), nil
}

func (s *Service) SecretsLogin(ctx context.Context, req SecretsLoginRequest) error {
	if s == nil {
		return fmt.Errorf("secrets login: service is nil")
	}
	return s.secretLoginRunner.Login(ctx, secrets.LoginRequest{Domain: req.Domain})
}

func (s *Service) SecretsCheck(ctx context.Context, req SecretsCheckRequest) (SecretsCheckResult, error) {
	if s == nil {
		return SecretsCheckResult{}, fmt.Errorf("secrets check: service is nil")
	}
	names, err := secrets.NormalizeNames(req.Names)
	if err != nil {
		return SecretsCheckResult{}, err
	}
	result := SecretsCheckResult{Provider: secrets.ProviderInfisical, Checked: names, Available: []string{}, Missing: []string{}}
	values, err := s.secretProvider.GetSecrets(ctx, names)
	for _, name := range names {
		if _, ok := values[name]; ok {
			result.Available = append(result.Available, name)
		} else {
			result.Missing = append(result.Missing, name)
		}
	}
	sort.Strings(result.Available)
	sort.Strings(result.Missing)
	result.Ready = err == nil && len(result.Missing) == 0
	return result, err
}
