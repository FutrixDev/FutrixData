package window

const DefaultLimit int64 = 2000

type Decision struct {
	Effective int64
	Fetch     int64
	Enforced  bool
}

type LimitPolicy struct {
	Max int64
}

func (p LimitPolicy) Decide(requested *int64) Decision {
	max := p.Max
	if max <= 0 {
		max = DefaultLimit
	}
	if requested == nil {
		return Decision{Effective: max, Fetch: max + 1, Enforced: true}
	}
	limit := *requested
	if limit < 0 {
		return Decision{Effective: max, Fetch: max + 1, Enforced: true}
	}
	return Decision{Effective: limit, Fetch: limit, Enforced: false}
}
