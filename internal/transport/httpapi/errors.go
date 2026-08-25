package httpapi

import (
	"errors"
	"net/http"

	"cleanroom-monitor-release/internal/application"
	"cleanroom-monitor-release/internal/domain/monitoring"
)

func mapError(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}
	status, code, message := http.StatusInternalServerError, "internal_error", "服务内部错误"
	switch {
	case errors.Is(err, application.ErrNotFound):
		status, code, message = http.StatusNotFound, "not_found", "监测活动不存在"
	case errors.Is(err, application.ErrVersionConflict):
		status, code, message = http.StatusConflict, "version_conflict", "expectedVersion 与当前版本不一致"
	case errors.Is(err, application.ErrIdempotencyConflict):
		status, code, message = http.StatusConflict, "idempotency_conflict", "idempotencyKey 已用于不同请求"
	default:
		var validation *application.ValidationError
		var authorization *application.AuthorizationError
		var rule *monitoring.RuleError
		if errors.As(err, &validation) {
			status, code, message = http.StatusBadRequest, "validation_error", validation.Message
		} else if errors.As(err, &authorization) {
			status, code, message = http.StatusForbidden, "forbidden", authorization.Message
		} else if errors.As(err, &rule) {
			status, code, message = http.StatusUnprocessableEntity, rule.Code, rule.Message
		}
	}
	writeError(w, r, status, code, message)
}
