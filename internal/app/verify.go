package app

import (
	"context"
	"fmt"

	"github.com/allensu/loki-profile-manager/internal/verify"
)

type VerifyRequest struct {
	StorePath     string
	ParentProfile string
	Buckets       []string
	MachineID     string
}

type VerifyResult = verify.Report

func (s *Service) Verify(ctx context.Context, req VerifyRequest) (VerifyResult, error) {
	if s == nil {
		return VerifyResult{}, fmt.Errorf("verify: service is nil")
	}
	storePath, err := s.effectiveStorePath(ctx, req.StorePath)
	if err != nil {
		return verify.Run(ctx, verify.Request{
			StorePath:     "",
			ParentProfile: req.ParentProfile,
			Buckets:       req.Buckets,
			MachineID:     req.MachineID,
			MachineIDPath: s.paths.MachineIDPath,
			Resolver:      s.resolver,
		}), nil
	}
	return verify.Run(ctx, verify.Request{
		StorePath:     storePath,
		ParentProfile: req.ParentProfile,
		Buckets:       req.Buckets,
		MachineID:     req.MachineID,
		MachineIDPath: s.paths.MachineIDPath,
		Resolver:      s.resolver,
	}), nil
}
