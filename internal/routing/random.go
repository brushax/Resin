package routing

import (
	"errors"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/Resinat/Resin/internal/node"
	"github.com/Resinat/Resin/internal/platform"
)

var ErrNoAvailableNodes = errors.New("no available nodes")

var randomRouteRNGPool = sync.Pool{
	New: func() any {
		return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	},
}

// randomRoute selects a routable node using P2C with latency/load scoring.
// It intentionally trusts Platform.View as the routable source of truth and
// does not do extra pool scans/availability validation on the hot path.
// Post-pick race handling (node removed right after selection) is handled by
// the caller in RouteRequest.
func randomRoute(
	plat *platform.Platform,
	stats *IPLoadStats,
	pool PoolAccessor,
	targetDomain string,
	authorities []string,
	p2cWindow time.Duration,
) (node.Hash, error) {
	view := plat.View()
	size := view.Size()
	if size == 0 {
		return node.Zero, ErrNoAvailableNodes
	}

	if plat.AllocationPolicy == platform.AllocationPolicyRoundRobin {
		h, ok := view.RoundRobinPick()
		if !ok {
			return node.Zero, ErrNoAvailableNodes
		}
		return h, nil
	}

	rng := randomRouteRNGPool.Get().(*rand.Rand)
	defer randomRouteRNGPool.Put(rng)

	pick := func() (node.Hash, bool) {
		return view.RandomPick(rng)
	}

	// Pick 1st candidate.
	h1, ok1 := pick()
	if !ok1 {
		return node.Zero, ErrNoAvailableNodes
	}

	// If view has one node, use it directly.
	if size == 1 {
		return h1, nil
	}

	// Pick 2nd candidate; best-effort to make it distinct.
	h2, ok2 := pick()
	if !ok2 {
		return h1, nil
	}
	if h2 == h1 {
		for i := 0; i < 3; i++ {
			candidate, ok := pick()
			if !ok {
				break
			}
			if candidate != h1 {
				h2 = candidate
				break
			}
		}
		if h2 == h1 {
			return h1, nil
		}
	}

	// Determine effective latency for comparison.
	lat1, lat2 := compareLatencies(h1, h2, pool, targetDomain, authorities, p2cWindow)

	// Calculate scores.
	s1 := calculateScore(h1, lat1, plat, stats, pool)
	s2 := calculateScore(h2, lat2, plat, stats, pool)

	// Lower score is better.
	selected := h2 // favor h2 on tie
	if s1 < s2 {
		selected = h1
	}
	return selected, nil
}

// compareLatencies determines the latency values for h1 and h2.
// Implements the 3-level comparison logic:
// 1. Target domain present in both and recent.
// 2. Common authority domains present in both and recent.
// 3. Fallback to 0 (empty) for both.
func compareLatencies(
	h1, h2 node.Hash,
	pool PoolAccessor,
	target string,
	authorities []string,
	window time.Duration,
) (time.Duration, time.Duration) {
	e1, ok1 := pool.GetEntry(h1)
	e2, ok2 := pool.GetEntry(h2)
	if !ok1 || !ok2 || e1.LatencyTable == nil || e2.LatencyTable == nil {
		return 0, 0
	}

	now := time.Now()

	// 1. Target domain check.
	// target can be empty if extracted domain is invalid/empty, handle gracefully.
	lat1, ok1 := lookupRecentDomainLatency(e1, target, now, window)
	lat2, ok2 := lookupRecentDomainLatency(e2, target, now, window)
	if ok1 && ok2 {
		return lat1, lat2
	}

	// 2. Authority intersection check.
	lat1, lat2, ok := averageComparableAuthorityLatencies(e1, e2, authorities, now, window)
	if ok {
		return lat1, lat2
	}

	// 3. Fallback.
	return 0, 0
}

func isRecent(t time.Time, now time.Time, window time.Duration) bool {
	return now.Sub(t) <= window
}

// calculateScore computes the score for a node based on platform allocation policy.
// Lower is better.
func calculateScore(
	h node.Hash,
	latency time.Duration,
	plat *platform.Platform,
	stats *IPLoadStats,
	pool PoolAccessor,
) float64 {
	entry, _ := pool.GetEntry(h)
	// If entry is nil (race), treat as high load/latency?
	// But we hold ref via pool, only deletion removes it.
	// Assuming existence since we just picked it from view.

	// Lease count and active in-flight connection load.
	var leaseCount int64
	var activeConns int64
	var throughput uint64
	if entry != nil {
		ip := entry.GetEgressIP()
		if ip.IsValid() && stats != nil {
			leaseCount = stats.Get(ip)
		}
		activeConns = entry.ActiveConns.Load()
		throughput = entry.GetThroughputBytesPerSec()
	}
	totalLoad := leaseCount + activeConns

	// If latency is 0 (empty/incompatible), score = totalLoad strictly.
	if latency <= 0 {
		return float64(totalLoad)
	}

	// Policy-based scoring.
	switch plat.AllocationPolicy {
	case platform.AllocationPolicyPreferThroughput:
		// Higher throughput is better -> lower score is better.
		// Scale by activeConns+1 to distribute concurrent requests across fast nodes.
		if throughput > 0 {
			mbps := float64(throughput) / (1024 * 1024)
			return (float64(activeConns+1) / (mbps + 0.1)) * 1e6
		}
		// Fallback to latency when throughput is unmeasured yet.
		return float64(activeConns+1) * float64(latency)
	case platform.AllocationPolicyPreferLowLatency:
		// Penalize nodes with active connections slightly (10% per active connection)
		// to prevent single-node hotspotting during concurrent bursts.
		penalty := 1.0 + 0.1*float64(activeConns)
		return float64(latency) * penalty
	case platform.AllocationPolicyPreferIdleIP:
		return float64(leaseCount)
	case platform.AllocationPolicyBalanced:
		fallthrough
	default:
		// (totalLoad + 1) * Latency
		return float64(totalLoad+1) * float64(latency)
	}
}
