package application

import (
	"context"
	"time"

	"cleanroom-monitor-release/internal/domain/monitoring"
)

type Mutation struct {
	CampaignID      string
	ExpectedVersion int64
	IdempotencyKey  string
	RequestHash     string
	Create          bool
	EventType       string
	Actor           string
	Details         string
	OccurredAt      time.Time
	Change          func(*monitoring.Campaign) error
}

type Repository interface {
	Run(context.Context, Mutation) (*monitoring.Campaign, bool, error)
	Get(context.Context, string) (*monitoring.Campaign, error)
	Timeline(context.Context, string) ([]monitoring.AuditEvent, error)
	Close() error
}

type ManifestService interface {
	Hash(*monitoring.Campaign) (string, error)
	Issue(*monitoring.Campaign, string, time.Time) (monitoring.ReleaseCredential, error)
	VerifyDetailed(*monitoring.Campaign) monitoring.CredentialVerificationReport
}
