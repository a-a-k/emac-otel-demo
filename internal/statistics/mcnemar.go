package statistics

import (
	"fmt"
	"math"
	"sort"
)

func ExactMcNemarTwoSided(b, c int) (float64, error) {
	if b < 0 || c < 0 {
		return 0, fmt.Errorf("negative discordant count")
	}
	n := b + c
	if n == 0 {
		return 1, nil
	}
	k := b
	if c < k {
		k = c
	}
	tail := 0.0
	for i := 0; i <= k; i++ {
		tail += math.Exp(logChoose(n, i) - float64(n)*math.Ln2)
	}
	p := 2 * tail
	if p > 1 {
		p = 1
	}
	return p, nil
}
func logChoose(n, k int) float64 {
	a, _ := math.Lgamma(float64(n + 1))
	b, _ := math.Lgamma(float64(k + 1))
	c, _ := math.Lgamma(float64(n - k + 1))
	return a - b - c
}

type HolmResult struct {
	Name      string  `json:"name"`
	P         float64 `json:"p"`
	AdjustedP float64 `json:"adjusted_p"`
	Reject    bool    `json:"reject"`
}

func Holm(p map[string]float64, alpha float64) ([]HolmResult, error) {
	out := make([]HolmResult, 0, len(p))
	for name, v := range p {
		if v < 0 || v > 1 {
			return nil, fmt.Errorf("invalid p for %s", name)
		}
		out = append(out, HolmResult{Name: name, P: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].P < out[j].P })
	running := 0.0
	still := true
	m := len(out)
	for i := range out {
		adj := float64(m-i) * out[i].P
		if adj < running {
			adj = running
		}
		if adj > 1 {
			adj = 1
		}
		running = adj
		out[i].AdjustedP = adj
		out[i].Reject = still && out[i].P <= alpha/float64(m-i)
		if !out[i].Reject {
			still = false
		}
	}
	return out, nil
}
