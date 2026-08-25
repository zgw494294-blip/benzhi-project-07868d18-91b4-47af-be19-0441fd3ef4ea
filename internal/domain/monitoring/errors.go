package monitoring

import "errors"

type RuleError struct {
	Code    string
	Message string
}

func (e *RuleError) Error() string { return e.Message }

func NewRuleError(code, message string) error { return &RuleError{Code: code, Message: message} }

func ErrorCode(err error) string {
	var target *RuleError
	if errors.As(err, &target) {
		return target.Code
	}
	return "internal_error"
}
