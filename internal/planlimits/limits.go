package planlimits

import (
	"strconv"
	"strings"
)

const (
	PlanFree = "free"
	PlanPro  = "pro"

	FeatureDatasources = "datasources"
	FeatureDevices     = "devices"
	FeatureRiskRules   = "risk_rules"

	ErrPrefix = "plan_limit_exceeded"
)

func Normalize(plan string) string {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case PlanPro:
		return PlanPro
	default:
		return PlanFree
	}
}

func DatasourceLimit(plan string) int {
	if Normalize(plan) == PlanFree {
		return 3
	}
	return 0
}

func DeviceLimit(plan string) int {
	if Normalize(plan) == PlanPro {
		return 3
	}
	return 1
}

func CustomRiskRulesAllowed(plan string) bool {
	return PolicyManagementAllowed(plan)
}

func PolicyManagementAllowed(plan string) bool {
	return Normalize(plan) == PlanPro
}

func PlanForDeviceLimit(limit int) string {
	switch limit {
	case 3:
		return PlanPro
	case 1:
		return PlanFree
	default:
		return ""
	}
}

func EncodeLimitError(feature, plan string, limit int) string {
	normalizedPlan := Normalize(plan)
	if limit < 0 {
		limit = 0
	}
	return ErrPrefix + ":" + strings.TrimSpace(feature) + ":" + normalizedPlan + ":" + strconv.Itoa(limit)
}

func DatasourceLimitError(plan string) string {
	return EncodeLimitError(FeatureDatasources, plan, DatasourceLimit(plan))
}

func DeviceLimitError(plan string, limit int) string {
	nextLimit := limit
	if nextLimit <= 0 {
		nextLimit = DeviceLimit(plan)
	}
	nextPlan := strings.TrimSpace(plan)
	if nextPlan == "" {
		nextPlan = PlanForDeviceLimit(nextLimit)
	}
	return EncodeLimitError(FeatureDevices, nextPlan, nextLimit)
}

func CustomRiskRulesError(plan string) string {
	return EncodeLimitError(FeatureRiskRules, plan, 0)
}
