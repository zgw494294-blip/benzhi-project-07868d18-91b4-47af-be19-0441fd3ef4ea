package application

import (
	"context"
	"fmt"
	"time"

	"cleanroom-monitor-release/internal/domain/monitoring"
)

type Clock func() time.Time

type Service struct {
	repo      Repository
	manifests ManifestService
	clock     Clock
}

type FreezePreflight struct {
	CampaignID       string                     `json:"campaignId"`
	CandidateVersion int64                      `json:"candidateVersion"`
	Eligible         bool                       `json:"eligible"`
	Blockers         []monitoring.FreezeBlocker `json:"blockers"`
	ManifestHash     string                     `json:"manifestHash,omitempty"`
	EvidenceCounts   monitoring.EvidenceCounts  `json:"evidenceCounts"`
}

func NewService(repo Repository, manifests ManifestService, clock Clock) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{repo: repo, manifests: manifests, clock: clock}
}

func (s *Service) CreateCampaign(ctx context.Context, cmd CreateCampaignCommand) (*monitoring.Campaign, bool, error) {
	if err := validateMeta(cmd.CommandMeta, false); err != nil {
		return nil, false, err
	}
	if err := requireRole(cmd.Role, RoleEngineer); err != nil {
		return nil, false, err
	}
	now := s.clock().UTC()
	mutation := Mutation{CampaignID: cmd.ID, ExpectedVersion: 0, IdempotencyKey: cmd.IdempotencyKey, RequestHash: requestHash(cmd), Create: true, EventType: "CampaignCreated", Actor: cmd.Actor, Details: "创建洁净室监测活动", OccurredAt: now}
	mutation.Change = func(current *monitoring.Campaign) error {
		created, err := monitoring.NewCampaign(monitoring.CreateSpec{ID: cmd.ID, FacilityName: cmd.FacilityName, RoomCode: cmd.RoomCode, CleanlinessClass: cmd.CleanlinessClass, PlannedDate: cmd.PlannedDate, Points: copyPoints(cmd.Points), Now: now})
		if err != nil {
			return err
		}
		*current = *created
		return nil
	}
	return s.repo.Run(context.WithoutCancel(ctx), mutation)
}

func (s *Service) RegisterInstruments(ctx context.Context, cmd RegisterInstrumentsCommand) (*monitoring.Campaign, bool, error) {
	if err := validateMeta(cmd.CommandMeta, true); err != nil {
		return nil, false, err
	}
	if err := requireRole(cmd.Role, RoleEngineer); err != nil {
		return nil, false, err
	}
	now := s.clock().UTC()
	return s.repo.Run(context.WithoutCancel(ctx), Mutation{CampaignID: cmd.CampaignID, ExpectedVersion: cmd.ExpectedVersion, IdempotencyKey: cmd.IdempotencyKey, RequestHash: requestHash(cmd), EventType: "InstrumentsRegistered", Actor: cmd.Actor, Details: fmt.Sprintf("登记 %d 项校准证据", len(cmd.Instruments)), OccurredAt: now, Change: func(c *monitoring.Campaign) error {
		return c.RegisterInstruments(copyInstruments(cmd.Instruments), cmd.Actor, now)
	}})
}

func (s *Service) AddRound(ctx context.Context, cmd AddRoundCommand) (*monitoring.Campaign, bool, error) {
	if err := validateMeta(cmd.CommandMeta, true); err != nil {
		return nil, false, err
	}
	if err := requireRole(cmd.Role, RoleEngineer); err != nil {
		return nil, false, err
	}
	now := s.clock().UTC()
	return s.repo.Run(context.WithoutCancel(ctx), Mutation{CampaignID: cmd.CampaignID, ExpectedVersion: cmd.ExpectedVersion, IdempotencyKey: cmd.IdempotencyKey, RequestHash: requestHash(cmd), EventType: "MeasurementRoundRecorded", Actor: cmd.Actor, Details: "提交测量轮次", OccurredAt: now, Change: func(c *monitoring.Campaign) error {
		round := copyRound(cmd.Round)
		if err := c.AddRoundByActor(round, cmd.Actor, now); err != nil {
			return err
		}
		return nil
	}})
}

func (s *Service) SubmitReview(ctx context.Context, cmd SubmitReviewCommand) (*monitoring.Campaign, bool, error) {
	if err := validateMeta(cmd.CommandMeta, true); err != nil {
		return nil, false, err
	}
	if err := requireRole(cmd.Role, RoleEngineer); err != nil {
		return nil, false, err
	}
	now := s.clock().UTC()
	return s.repo.Run(context.WithoutCancel(ctx), Mutation{CampaignID: cmd.CampaignID, ExpectedVersion: cmd.ExpectedVersion, IdempotencyKey: cmd.IdempotencyKey, RequestHash: requestHash(cmd), EventType: "ReviewSubmitted", Actor: cmd.Actor, Details: "完成自动检查并提交复核", OccurredAt: now, Change: func(c *monitoring.Campaign) error { return c.SubmitForReview(now) }})
}

func (s *Service) DecideFinding(ctx context.Context, cmd DecideFindingCommand) (*monitoring.Campaign, bool, error) {
	if err := validateMeta(cmd.CommandMeta, true); err != nil {
		return nil, false, err
	}
	if err := requireRole(cmd.Role, RoleReviewer); err != nil {
		return nil, false, err
	}
	now := s.clock().UTC()
	return s.repo.Run(context.WithoutCancel(ctx), Mutation{CampaignID: cmd.CampaignID, ExpectedVersion: cmd.ExpectedVersion, IdempotencyKey: cmd.IdempotencyKey, RequestHash: requestHash(cmd), EventType: "FindingDecided", Actor: cmd.Actor, Details: string(cmd.Decision), OccurredAt: now, Change: func(c *monitoring.Campaign) error {
		return c.DecideFinding(cmd.FindingID, cmd.Decision, cmd.Actor, cmd.Note, cmd.RemediationNote, now)
	}})
}

func (s *Service) Freeze(ctx context.Context, cmd FreezeCommand) (*monitoring.Campaign, bool, error) {
	if err := validateMeta(cmd.CommandMeta, true); err != nil {
		return nil, false, err
	}
	if err := requireRole(cmd.Role, RoleReviewer); err != nil {
		return nil, false, err
	}
	if cmd.CandidateVersion < 1 || cmd.ManifestHash == "" {
		return nil, false, &ValidationError{Message: "candidateVersion 和 manifestHash 不能为空"}
	}
	if cmd.CandidateVersion != cmd.ExpectedVersion {
		return nil, false, ErrVersionConflict
	}
	now := s.clock().UTC()
	return s.repo.Run(context.WithoutCancel(ctx), Mutation{CampaignID: cmd.CampaignID, ExpectedVersion: cmd.ExpectedVersion, IdempotencyKey: cmd.IdempotencyKey, RequestHash: requestHash(cmd), EventType: "CampaignFrozen", Actor: cmd.Actor, Details: "冻结证据清单", OccurredAt: now, Change: func(c *monitoring.Campaign) error {
		if err := c.CanFreeze(); err != nil {
			return err
		}
		hash, err := s.manifests.Hash(c)
		if err != nil {
			return err
		}
		if hash != cmd.ManifestHash {
			return monitoring.NewRuleError("manifest_candidate_mismatch", "预检清单哈希与事务内重算结果不一致")
		}
		return c.Freeze(hash, now)
	}})
}

func (s *Service) FreezePreflight(ctx context.Context, id string, candidateVersion int64) (FreezePreflight, error) {
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return FreezePreflight{}, err
	}
	if candidateVersion > 0 && candidateVersion != c.Version {
		return FreezePreflight{}, ErrVersionConflict
	}
	report := FreezePreflight{CampaignID: c.ID, CandidateVersion: c.Version, Blockers: c.FreezeBlockers(), EvidenceCounts: c.EvidenceCounts()}
	report.Eligible = len(report.Blockers) == 0
	if report.Eligible {
		report.ManifestHash, err = s.manifests.Hash(c)
		if err != nil {
			return FreezePreflight{}, err
		}
	}
	return report, nil
}

func (s *Service) Issue(ctx context.Context, cmd IssueCommand) (*monitoring.Campaign, bool, error) {
	if err := validateMeta(cmd.CommandMeta, true); err != nil {
		return nil, false, err
	}
	if err := requireRole(cmd.Role, RoleCompliance); err != nil {
		return nil, false, err
	}
	if cmd.IssuedBy == "" || cmd.IssuedBy != cmd.Actor {
		return nil, false, &ValidationError{Message: "issuedBy 必须与 actor 一致"}
	}
	now := s.clock().UTC()
	return s.repo.Run(context.WithoutCancel(ctx), Mutation{CampaignID: cmd.CampaignID, ExpectedVersion: cmd.ExpectedVersion, IdempotencyKey: cmd.IdempotencyKey, RequestHash: requestHash(cmd), EventType: "CredentialIssued", Actor: cmd.Actor, Details: "签发放行凭据", OccurredAt: now, Change: func(c *monitoring.Campaign) error {
		credential, err := s.manifests.Issue(c, cmd.IssuedBy, now)
		if err != nil {
			return err
		}
		return c.Certify(credential, now)
	}})
}

func (s *Service) GetCampaign(ctx context.Context, id string) (*monitoring.Campaign, error) {
	return s.repo.Get(ctx, id)
}
func (s *Service) Timeline(ctx context.Context, id string) ([]monitoring.AuditEvent, error) {
	return s.repo.Timeline(ctx, id)
}
func (s *Service) VerifyCredential(ctx context.Context, id string) (monitoring.CredentialVerificationReport, error) {
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return monitoring.CredentialVerificationReport{}, err
	}
	return s.manifests.VerifyDetailed(c), nil
}
