package tui

import (
	"context"
	"fmt"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/machine"
)

type Client interface {
	Status(context.Context) (app.StatusResult, error)
	StoreStatus(context.Context) (app.StoreStatusResult, error)
	DiscoverStores(context.Context, app.DiscoverStoresRequest) (app.DiscoverStoresResult, error)
	UseStore(context.Context, app.UseStoreRequest) (app.EnsureStoreResult, error)
	EnsureStore(context.Context, app.EnsureStoreRequest) (app.EnsureStoreResult, error)
	ForgetStore(context.Context, app.ForgetStoreRequest) (app.StoreStatusResult, error)
	ProfileCatalog(context.Context) (app.ProfileCatalogResult, error)
	Doctor(context.Context) (app.DoctorResult, error)
	MachineStatus(context.Context) (app.MachineStatusResult, error)
	RegisterMachine(context.Context, app.RegisterMachineRequest) (machine.Record, error)
	SecretsStatus(context.Context) (app.SecretsStatusResult, error)
	SecretsConfigureInfisical(context.Context, app.SecretsConfigureInfisicalRequest) (app.SecretsConfigureInfisicalResult, error)
	ListSnapshots(context.Context) (app.SnapshotListResult, error)
	ShowSnapshot(context.Context, app.SnapshotShowRequest) (app.SnapshotShowResult, error)
	RestoreSnapshotDryRun(context.Context, app.SnapshotRestoreDryRunRequest) (app.SnapshotRestoreDryRunResult, error)
	Switch(context.Context, app.SwitchRequest) (app.SwitchResult, error)
	Sync(context.Context, app.SyncRequest) (app.SyncResult, error)
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

func (c ServiceClient) StoreStatus(ctx context.Context) (app.StoreStatusResult, error) {
	if c.Service == nil {
		return app.StoreStatusResult{}, fmt.Errorf("tui client: service is nil")
	}
	return c.Service.StoreStatus(ctx, app.StoreStatusRequest{})
}

func (c ServiceClient) DiscoverStores(ctx context.Context, req app.DiscoverStoresRequest) (app.DiscoverStoresResult, error) {
	if c.Service == nil {
		return app.DiscoverStoresResult{}, fmt.Errorf("tui client: service is nil")
	}
	return c.Service.DiscoverStores(ctx, req)
}

func (c ServiceClient) UseStore(ctx context.Context, req app.UseStoreRequest) (app.EnsureStoreResult, error) {
	if c.Service == nil {
		return app.EnsureStoreResult{}, fmt.Errorf("tui client: service is nil")
	}
	return c.Service.UseStore(ctx, req)
}

func (c ServiceClient) EnsureStore(ctx context.Context, req app.EnsureStoreRequest) (app.EnsureStoreResult, error) {
	if c.Service == nil {
		return app.EnsureStoreResult{}, fmt.Errorf("tui client: service is nil")
	}
	return c.Service.EnsureStore(ctx, req)
}

func (c ServiceClient) ForgetStore(ctx context.Context, req app.ForgetStoreRequest) (app.StoreStatusResult, error) {
	if c.Service == nil {
		return app.StoreStatusResult{}, fmt.Errorf("tui client: service is nil")
	}
	return c.Service.ForgetStore(ctx, req)
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

func (c ServiceClient) RegisterMachine(ctx context.Context, req app.RegisterMachineRequest) (machine.Record, error) {
	if c.Service == nil {
		return machine.Record{}, fmt.Errorf("tui client: service is nil")
	}
	if req.StorePath == "" {
		req.StorePath = c.StorePath
	}
	return c.Service.RegisterMachine(ctx, req)
}

func (c ServiceClient) SecretsStatus(ctx context.Context) (app.SecretsStatusResult, error) {
	if c.Service == nil {
		return app.SecretsStatusResult{}, fmt.Errorf("tui client: service is nil")
	}
	return c.Service.SecretsStatus(ctx, app.SecretsStatusRequest{})
}

func (c ServiceClient) SecretsConfigureInfisical(ctx context.Context, req app.SecretsConfigureInfisicalRequest) (app.SecretsConfigureInfisicalResult, error) {
	if c.Service == nil {
		return app.SecretsConfigureInfisicalResult{}, fmt.Errorf("tui client: service is nil")
	}
	return c.Service.SecretsConfigureInfisical(ctx, req)
}

func (c ServiceClient) ListSnapshots(ctx context.Context) (app.SnapshotListResult, error) {
	if c.Service == nil {
		return app.SnapshotListResult{}, fmt.Errorf("tui client: service is nil")
	}
	return c.Service.ListSnapshots(ctx, app.SnapshotListRequest{})
}

func (c ServiceClient) ShowSnapshot(ctx context.Context, req app.SnapshotShowRequest) (app.SnapshotShowResult, error) {
	if c.Service == nil {
		return app.SnapshotShowResult{}, fmt.Errorf("tui client: service is nil")
	}
	return c.Service.ShowSnapshot(ctx, req)
}

func (c ServiceClient) RestoreSnapshotDryRun(ctx context.Context, req app.SnapshotRestoreDryRunRequest) (app.SnapshotRestoreDryRunResult, error) {
	if c.Service == nil {
		return app.SnapshotRestoreDryRunResult{}, fmt.Errorf("tui client: service is nil")
	}
	req.DryRun = true
	return c.Service.RestoreSnapshotDryRun(ctx, req)
}

func (c ServiceClient) Switch(ctx context.Context, req app.SwitchRequest) (app.SwitchResult, error) {
	if c.Service == nil {
		return app.SwitchResult{}, fmt.Errorf("tui client: service is nil")
	}
	if req.StorePath == "" {
		req.StorePath = c.StorePath
	}
	return c.Service.Switch(ctx, req)
}

func (c ServiceClient) Sync(ctx context.Context, req app.SyncRequest) (app.SyncResult, error) {
	if c.Service == nil {
		return app.SyncResult{}, fmt.Errorf("tui client: service is nil")
	}
	if req.StorePath == "" {
		req.StorePath = c.StorePath
	}
	return c.Service.Sync(ctx, req)
}
