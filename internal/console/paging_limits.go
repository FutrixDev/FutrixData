package console

import "futrixdata/platform/internal/console/window"

func resolveTotalLimit(found bool, limit int64, policy window.LimitPolicy) int64 {
	if found {
		if limit < 0 {
			return policy.Decide(nil).Effective
		}
		return limit
	}
	return policy.Decide(nil).Effective
}
