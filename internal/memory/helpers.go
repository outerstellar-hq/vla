package memory

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"sync/atomic"
	"time"
)

// jsonMarshal / jsonUnmarshal are thin wrappers so the types.go file reads
// clean without importing encoding/json there.
func jsonMarshal(m *Memory) ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

func jsonUnmarshal(data []byte, m *Memory) error {
	return json.Unmarshal(data, m)
}

// generateID produces a unique ID: timestamp + random suffix. The random
// suffix prevents collisions when multiple memories are saved in the same
// nanosecond (which happens in tests and fast batch saves).
var idCounter atomic.Uint64

func generateID() string {
	ts := time.Now().UTC().Format("20060102T150405.000000000Z")
	n := idCounter.Add(1)
	return fmt.Sprintf("%s-%06d", ts, n%1000000)
}

// cosineSim computes the cosine similarity between two float32 vectors.
// Returns 0 if either is empty or lengths don't match.
func cosineSim(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, magA, magB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		magA += float64(a[i]) * float64(a[i])
		magB += float64(b[i]) * float64(b[i])
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}

// normalizeScores performs min-max normalization on the given score field,
// calling get to read and set to write back. If all values are equal, they
// all normalize to 1.0 (so they aren't zeroed out).
type scored struct {
	mem    *Memory
	vScore float64
	kScore float64
}

func normalizeScores(hits []scored, get func(scored) float64, set func(scored, float64) scored) {
	if len(hits) == 0 {
		return
	}
	min, max := get(hits[0]), get(hits[0])
	for _, h := range hits[1:] {
		v := get(h)
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	rangeVal := max - min
	for i, h := range hits {
		v := get(h)
		var norm float64
		if rangeVal == 0 {
			norm = 1.0 // all equal → don't zero out
		} else {
			norm = (v - min) / rangeVal
		}
		hits[i] = set(h, norm)
	}
}

func sortByTimestampDesc(ms []*Memory) {
	sort.Slice(ms, func(i, j int) bool {
		return ms[i].Timestamp.After(ms[j].Timestamp)
	})
}

func sortResultsByScoreDesc(rs []*SearchResult) {
	sort.Slice(rs, func(i, j int) bool {
		return rs[i].Score > rs[j].Score
	})
}
