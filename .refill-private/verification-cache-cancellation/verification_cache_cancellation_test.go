package verification_cache_cancellation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"cleanroom-monitor-release/internal/application"
	"cleanroom-monitor-release/internal/domain/monitoring"
)

type contextRepository struct {
	campaign *monitoring.Campaign
}

func (r *contextRepository) Run(context.Context, application.Mutation) (*monitoring.Campaign, bool, error) {
	panic("Run must not be called")
}

func (r *contextRepository) Get(ctx context.Context, id string) (*monitoring.Campaign, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if id != r.campaign.ID {
		return nil, application.ErrNotFound
	}
	return r.campaign, nil
}

func (r *contextRepository) Timeline(context.Context, string) ([]monitoring.AuditEvent, error) {
	panic("Timeline must not be called")
}

func (r *contextRepository) Close() error { return nil }

type fixedManifestService struct{}

func (fixedManifestService) Hash(*monitoring.Campaign) (string, error) {
	panic("Hash must not be called")
}

func (fixedManifestService) Issue(*monitoring.Campaign, string, time.Time) (monitoring.ReleaseCredential, error) {
	panic("Issue must not be called")
}

func (fixedManifestService) VerifyDetailed(c *monitoring.Campaign) monitoring.CredentialVerificationReport {
	return monitoring.CredentialVerificationReport{
		CampaignID: c.ID,
		Valid:      true,
		ReasonCode: "valid",
		Reason:     "凭据有效",
		Checks:     []monitoring.VerificationCheck{{Name: "manifest_integrity", Passed: true, ReasonCode: "passed"}},
	}
}

func TestCanceledCredentialVerificationDoesNotReuseCachedSuccess(t *testing.T) {
	repo := &contextRepository{campaign: &monitoring.Campaign{ID: "campaign-cache-cancel"}}
	service := application.NewService(repo, fixedManifestService{}, nil)

	primed, err := service.VerifyCredential(context.Background(), repo.campaign.ID)
	if err != nil || !primed.Valid {
		t.Fatalf("failed to prime verification result: report=%+v err=%v", primed, err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := service.VerifyCredential(canceled, repo.campaign.ID)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled verification reused a cached success: report=%+v err=%v", got, err)
	}
}
