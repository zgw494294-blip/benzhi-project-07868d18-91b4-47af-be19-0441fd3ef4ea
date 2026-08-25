package monitoring

import "fmt"

type Status string

const (
	StatusDraft         Status = "Draft"
	StatusReady         Status = "Ready"
	StatusExecuting     Status = "Executing"
	StatusReviewPending Status = "ReviewPending"
	StatusRemediation   Status = "Remediation"
	StatusFrozen        Status = "Frozen"
	StatusCertified     Status = "Certified"
)

func (s Status) Valid() bool {
	switch s {
	case StatusDraft, StatusReady, StatusExecuting, StatusReviewPending, StatusRemediation, StatusFrozen, StatusCertified:
		return true
	default:
		return false
	}
}

func RequireStatus(actual Status, allowed ...Status) error {
	for _, candidate := range allowed {
		if actual == candidate {
			return nil
		}
	}
	return NewRuleError("invalid_state", fmt.Sprintf("状态 %s 不允许执行此操作", actual))
}

func IsFrozen(s Status) bool { return s == StatusFrozen || s == StatusCertified }
