package detachedmutationcancellation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"cleanroom-monitor-release/internal/application"
	"cleanroom-monitor-release/internal/domain/monitoring"
)

type cancellationRepository struct{}

func (cancellationRepository) Run(ctx context.Context, _ application.Mutation) (*monitoring.Campaign, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	return &monitoring.Campaign{}, false, nil
}

func (cancellationRepository) Get(context.Context, string) (*monitoring.Campaign, error) {
	return nil, errors.New("unexpected Get")
}

func (cancellationRepository) Timeline(context.Context, string) ([]monitoring.AuditEvent, error) {
	return nil, errors.New("unexpected Timeline")
}

func (cancellationRepository) Close() error { return nil }

type manifestStub struct{}

func (manifestStub) Hash(*monitoring.Campaign) (string, error) { return "hash", nil }

func (manifestStub) Issue(*monitoring.Campaign, string, time.Time) (monitoring.ReleaseCredential, error) {
	return monitoring.ReleaseCredential{}, nil
}

func (manifestStub) VerifyDetailed(*monitoring.Campaign) monitoring.CredentialVerificationReport {
	return monitoring.CredentialVerificationReport{}
}

func TestCanceledMutationContextsReachRepository(t *testing.T) {
	service := application.NewService(cancellationRepository{}, manifestStub{}, func() time.Time {
		return time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	engineer := application.CommandMeta{ExpectedVersion: 1, IdempotencyKey: "engineer-key", Actor: "engineer", Role: application.RoleEngineer}
	reviewer := application.CommandMeta{ExpectedVersion: 1, IdempotencyKey: "reviewer-key", Actor: "reviewer", Role: application.RoleReviewer}
	compliance := application.CommandMeta{ExpectedVersion: 1, IdempotencyKey: "compliance-key", Actor: "compliance", Role: application.RoleCompliance}
	calls := []struct {
		name string
		run  func() error
	}{
		{name: "CreateCampaign", run: func() error {
			meta := engineer
			meta.ExpectedVersion = 0
			_, _, err := service.CreateCampaign(ctx, application.CreateCampaignCommand{CommandMeta: meta, ID: "campaign-create"})
			return err
		}},
		{name: "RegisterInstruments", run: func() error {
			_, _, err := service.RegisterInstruments(ctx, application.RegisterInstrumentsCommand{CommandMeta: engineer, CampaignID: "campaign-1"})
			return err
		}},
		{name: "AddRound", run: func() error {
			_, _, err := service.AddRound(ctx, application.AddRoundCommand{CommandMeta: engineer, CampaignID: "campaign-1"})
			return err
		}},
		{name: "SubmitReview", run: func() error {
			_, _, err := service.SubmitReview(ctx, application.SubmitReviewCommand{CommandMeta: engineer, CampaignID: "campaign-1"})
			return err
		}},
		{name: "DecideFinding", run: func() error {
			_, _, err := service.DecideFinding(ctx, application.DecideFindingCommand{CommandMeta: reviewer, CampaignID: "campaign-1"})
			return err
		}},
		{name: "Freeze", run: func() error {
			_, _, err := service.Freeze(ctx, application.FreezeCommand{CommandMeta: reviewer, CampaignID: "campaign-1", CandidateVersion: 1, ManifestHash: "hash"})
			return err
		}},
		{name: "Issue", run: func() error {
			_, _, err := service.Issue(ctx, application.IssueCommand{CommandMeta: compliance, CampaignID: "campaign-1", IssuedBy: "compliance"})
			return err
		}},
	}

	for _, call := range calls {
		if err := call.run(); !errors.Is(err, context.Canceled) {
			t.Errorf("%s detached the caller cancellation: got %v, want context.Canceled", call.name, err)
		}
	}
}
