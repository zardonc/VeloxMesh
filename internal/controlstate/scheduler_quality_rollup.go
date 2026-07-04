package controlstate

func NormalizeSchedulerQualityRollup(in *SchedulerQualityRollup) *SchedulerQualityRollup {
	out := *in
	out.MAPEAvg = average(out.MAPESum, out.SampleCount)
	out.WaitMSAvg = average(out.WaitMSSum, out.SampleCount)
	out.SchedulerCallLatencyMSAvg = average(out.SchedulerCallLatencyMSSum, out.SampleCount)
	out.ConfidenceAvg = average(out.ConfidenceSum, out.SampleCount)
	out.SafeSampleIDs = append([]string(nil), in.SafeSampleIDs...)
	return &out
}

func MergeSchedulerQualityRollups(current, incoming *SchedulerQualityRollup) *SchedulerQualityRollup {
	out := *current
	out.SampleCount += incoming.SampleCount
	out.MAPESum += incoming.MAPESum
	out.WaitMSSum += incoming.WaitMSSum
	out.SchedulerCallLatencyMSSum += incoming.SchedulerCallLatencyMSSum
	out.ErrorCount += incoming.ErrorCount
	out.ConfidenceSum += incoming.ConfidenceSum
	out.SafeSampleIDs = appendUnique(out.SafeSampleIDs, incoming.SafeSampleIDs)
	return NormalizeSchedulerQualityRollup(&out)
}

func average(sum float64, count int64) float64 {
	if count <= 0 {
		return 0
	}
	return sum / float64(count)
}

func appendUnique(existing []string, incoming []string) []string {
	seen := make(map[string]bool, len(existing)+len(incoming))
	out := append([]string(nil), existing...)
	for _, value := range existing {
		seen[value] = true
	}
	for _, value := range incoming {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
