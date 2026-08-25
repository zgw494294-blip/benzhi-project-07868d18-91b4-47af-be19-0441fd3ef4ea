package application

const (
	RoleEngineer   = "monitoring_engineer"
	RoleReviewer   = "quality_reviewer"
	RoleCompliance = "facility_compliance"
)

func requireRole(actual string, accepted ...string) error {
	for _, role := range accepted {
		if actual == role {
			return nil
		}
	}
	return &AuthorizationError{Message: "当前角色无权执行此操作"}
}
