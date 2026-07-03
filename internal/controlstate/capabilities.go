package controlstate

type CapabilityProfile struct {
	DurableConfig           bool
	DistributedControlState bool
	LocalOnly               bool
	RedisHotState           bool
	SemanticCache           bool
	RateLimits              bool
	CostGovernance          bool
}

func PostgreSQLCapabilityProfile() CapabilityProfile {
	return CapabilityProfile{
		DurableConfig:           true,
		DistributedControlState: true,
		LocalOnly:               false,
		RedisHotState:           false, // Redis capabilities are separate
		SemanticCache:           true,
		RateLimits:              true,
		CostGovernance:          true,
	}
}

func SQLiteCapabilityProfile() CapabilityProfile {
	return CapabilityProfile{
		DurableConfig:           true,
		DistributedControlState: false,
		LocalOnly:               true,
		RedisHotState:           false,
		SemanticCache:           false,
		RateLimits:              false,
		CostGovernance:          false,
	}
}
