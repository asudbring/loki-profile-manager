package app

import (
	"context"
	"fmt"

	diagnostics "github.com/asudbring/loki-profile-manager/internal/doctor"
)

// FindSwitchBlockers detects capture-unsupported or capture-conflict changes
// that will block the next switch operation.
func (s *Service) FindSwitchBlockers(ctx context.Context) ([]diagnostics.ResolvableBlocker, error) {
	if s == nil {
		return nil, fmt.Errorf("find switch blockers: service is nil")
	}
	storePath, err := s.effectiveStorePath(ctx, "")
	if err != nil {
		return nil, err
	}
	return diagnostics.FindSwitchBlockers(ctx, s.database, storePath, s.resolver)
}

// ResolveBlocker promotes local overrides into a chosen store layer and
// repairs the managed state record to clear the switch blocker.
func (s *Service) ResolveBlocker(ctx context.Context, blocker diagnostics.ResolvableBlocker, chosen diagnostics.LayerChoice) error {
	if s == nil {
		return fmt.Errorf("resolve blocker: service is nil")
	}
	return diagnostics.ResolveBlocker(ctx, diagnostics.ResolveBlockerOptions{
		Blocker:     blocker,
		ChosenLayer: chosen,
		Database:    s.database,
	})
}
