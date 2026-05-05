package tui

import (
	"context"
	"fmt"

	"github.com/allensu/loki-profile-manager/internal/app"
)

type Client interface {
	Status(context.Context) (app.StatusResult, error)
	ProfileCatalog(context.Context) (app.ProfileCatalogResult, error)
	Doctor(context.Context) (app.DoctorResult, error)
	MachineStatus(context.Context) (app.MachineStatusResult, error)
	SecretsStatus(context.Context) (app.SecretsStatusResult, error)
	ListSnapshots(context.Context) (app.SnapshotListResult, error)
}

type ServiceClient struct {
	Service   *app.Service
	StorePath string
}

func NewServiceClient(service *app.Service, storePath string) ServiceClient {
	return ServiceClient{Service: service, StorePath: storePath}
}

func (c ServiceClient) Status(ctx context.Context) (app.StatusResult, error) {
	if c.Service == nil {
		return app.StatusResult{}, fmt.Errorf("tui client: service is nil")
	}
	return c.Service.Status(ctx, app.StatusRequest{})
}

func (c ServiceClient) ProfileCatalog(ctx context.Context) (app.ProfileCatalogResult, error) {
	if c.Service == nil {
		return app.ProfileCatalogResult{}, fmt.Errorf("tui client: service is nil")
	}
	return c.Service.ProfileCatalog(ctx, app.ProfileCatalogRequest{StorePath: c.StorePath})
}

func (c ServiceClient) Doctor(ctx context.Context) (app.DoctorResult, error) {
	if c.Service == nil {
		return app.DoctorResult{}, fmt.Errorf("tui client: service is nil")
	}
	return c.Service.Doctor(ctx, app.DoctorRequest{StorePath: c.StorePath})
}

func (c ServiceClient) MachineStatus(ctx context.Context) (app.MachineStatusResult, error) {
	if c.Service == nil {
		return app.MachineStatusResult{}, fmt.Errorf("tui client: service is nil")
	}
	return c.Service.MachineStatus(ctx, app.MachineStatusRequest{StorePath: c.StorePath})
}

func (c ServiceClient) SecretsStatus(ctx context.Context) (app.SecretsStatusResult, error) {
	if c.Service == nil {
		return app.SecretsStatusResult{}, fmt.Errorf("tui client: service is nil")
	}
	return c.Service.SecretsStatus(ctx, app.SecretsStatusRequest{})
}

func (c ServiceClient) ListSnapshots(ctx context.Context) (app.SnapshotListResult, error) {
	if c.Service == nil {
		return app.SnapshotListResult{}, fmt.Errorf("tui client: service is nil")
	}
	return c.Service.ListSnapshots(ctx, app.SnapshotListRequest{})
}
