package routing

import (
	"math"
	"sort"
	"time"

	"veloxmesh/internal/errors"
	"veloxmesh/internal/health"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
)

type CompositeConfig struct {
	LatencyWeight     float64
	LoadWeight        float64
	ErrorRateWeight   float64
	HealthWeight      float64
	DegradedPenalty   float64
	WarmUpSuccesses   int
	ScoreThreshold    float64
	NearTieThreshold  float64
	StaleMetricWindow time.Duration
	CostOverrides     map[string]float64
}

func DefaultCompositeConfig() CompositeConfig {
	return CompositeConfig{
		LatencyWeight:     0.4,
		LoadWeight:        0.2,
		ErrorRateWeight:   0.3,
		HealthWeight:      0.1,
		DegradedPenalty:   0.5,
		WarmUpSuccesses:   5,
		ScoreThreshold:    -2.0,
		NearTieThreshold:  0.1,
		StaleMetricWindow: 2 * time.Minute,
	}
}

type CompositeScoreSummary struct {
	ProviderID  string
	Model       string
	Score       float64
	IsWarmUp    bool
	CostApplied bool
}

type candidateData struct {
	adapter    providers.ProviderAdapter
	latency    float64
	load       float64
	errorRate  float64
	degraded   bool
	cost       float64
	stale      bool
	warmUp     bool
	finalScore float64
}

func SelectComposite(
	candidates []providers.ProviderAdapter,
	healthStore health.Store,
	req *llm.LLMRequest,
	config CompositeConfig,
) (providers.ProviderAdapter, CompositeScoreSummary, error) {

	if len(candidates) == 0 {
		return nil, CompositeScoreSummary{}, errors.ErrNoHealthyProvider
	}

	now := time.Now()
	var data []candidateData
	var needsWarmUp []candidateData

	for _, adapter := range candidates {
		snap := healthStore.Snapshot(adapter.ID())
		modelSnap := healthStore.ModelSnapshot(adapter.ID(), req.Model)

		cd := candidateData{
			adapter:  adapter,
			latency:  float64(snap.EWMALatency),
			load:     float64(snap.PendingRequests),
			cost:     0.0,
			degraded: snap.Status == health.StatusDegraded,
		}

		if config.CostOverrides != nil {
			if cost, ok := config.CostOverrides[adapter.ID()+":"+req.Model]; ok {
				cd.cost = cost
			}
		}

		if snap.TotalSuccesses+snap.TotalFailures > 0 {
			cd.errorRate = float64(snap.TotalFailures) / float64(snap.TotalSuccesses+snap.TotalFailures)
		}

		if now.Sub(snap.LastUpdated) > config.StaleMetricWindow {
			cd.stale = true
		}

		if modelSnap.TotalSuccesses < config.WarmUpSuccesses {
			cd.warmUp = true
			needsWarmUp = append(needsWarmUp, cd)
		}

		data = append(data, cd)
	}

	// D-05: Round-robin warm-up
	if len(needsWarmUp) > 0 {
		sort.Slice(needsWarmUp, func(i, j int) bool {
			idI := needsWarmUp[i].adapter.ID()
			idJ := needsWarmUp[j].adapter.ID()
			snapI := healthStore.ModelSnapshot(idI, req.Model)
			snapJ := healthStore.ModelSnapshot(idJ, req.Model)
			if snapI.TotalSuccesses == snapJ.TotalSuccesses {
				return idI < idJ
			}
			return snapI.TotalSuccesses < snapJ.TotalSuccesses
		})

		best := needsWarmUp[0]
		return best.adapter, CompositeScoreSummary{
			ProviderID:  best.adapter.ID(),
			Model:       req.Model,
			Score:       0,
			IsWarmUp:    true,
			CostApplied: false,
		}, nil
	}

	// Normalize metrics
	normalizeAndScore(data, config)

	// Sort by final score descending
	sort.Slice(data, func(i, j int) bool {
		diff := math.Abs(data[i].finalScore - data[j].finalScore)
		if diff <= config.NearTieThreshold {
			if data[i].cost != data[j].cost {
				return data[i].cost < data[j].cost
			}
			return data[i].adapter.ID() < data[j].adapter.ID()
		}
		return data[i].finalScore > data[j].finalScore
	})

	best := data[0]

	if best.finalScore < config.ScoreThreshold {
		return best.adapter, CompositeScoreSummary{
			ProviderID:  best.adapter.ID(),
			Model:       req.Model,
			Score:       best.finalScore,
			IsWarmUp:    false,
			CostApplied: false,
		}, errors.ErrCompositeScoreBelowThreshold
	}

	costApplied := false
	if len(data) > 1 {
		diff := math.Abs(data[0].finalScore - data[1].finalScore)
		if diff <= config.NearTieThreshold && data[0].cost != data[1].cost {
			costApplied = true
		}
	}

	return best.adapter, CompositeScoreSummary{
		ProviderID:  best.adapter.ID(),
		Model:       req.Model,
		Score:       best.finalScore,
		IsWarmUp:    false,
		CostApplied: costApplied,
	}, nil
}

func normalizeAndScore(data []candidateData, config CompositeConfig) {
	n := float64(len(data))
	if n == 0 {
		return
	}

	var sumLat, sumLoad, sumErr float64
	var count float64
	for _, d := range data {
		if !d.stale {
			sumLat += d.latency
			sumLoad += d.load
			sumErr += d.errorRate
			count++
		}
	}

	if count == 0 {
		// All stale, just apply degraded penalty
		for i := range data {
			score := 0.0
			if data[i].degraded {
				score -= config.DegradedPenalty
			}
			data[i].finalScore = score
		}
		return
	}

	meanLat := sumLat / count
	meanLoad := sumLoad / count
	meanErr := sumErr / count

	var sqLat, sqLoad, sqErr float64
	for _, d := range data {
		if !d.stale {
			sqLat += (d.latency - meanLat) * (d.latency - meanLat)
			sqLoad += (d.load - meanLoad) * (d.load - meanLoad)
			sqErr += (d.errorRate - meanErr) * (d.errorRate - meanErr)
		}
	}

	stdLat := math.Sqrt(sqLat / count)
	stdLoad := math.Sqrt(sqLoad / count)
	stdErr := math.Sqrt(sqErr / count)

	for i := range data {
		var zLat, zLoad, zErr float64
		if !data[i].stale {
			if stdLat > 0 {
				zLat = (data[i].latency - meanLat) / stdLat
			}
			if stdLoad > 0 {
				zLoad = (data[i].load - meanLoad) / stdLoad
			}
			if stdErr > 0 {
				zErr = (data[i].errorRate - meanErr) / stdErr
			}
		}

		score := -(zLat*config.LatencyWeight + zLoad*config.LoadWeight + zErr*config.ErrorRateWeight)
		if data[i].degraded {
			score -= config.DegradedPenalty
		}

		data[i].finalScore = score
	}
}
