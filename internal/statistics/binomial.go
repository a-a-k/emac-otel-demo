package statistics

import (
	"fmt"
	"math"
)

type Interval struct {
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
}

// ClopperPearson returns the equal-tailed exact binomial confidence interval.
func ClopperPearson(successes, trials int, alpha float64) (Interval, error) {
	if trials < 0 || successes < 0 || successes > trials {
		return Interval{}, fmt.Errorf("invalid counts")
	}
	if !(alpha > 0 && alpha < 1) {
		return Interval{}, fmt.Errorf("alpha must be in (0,1)")
	}
	if trials == 0 {
		return Interval{0, 1}, nil
	}
	lo, hi := 0.0, 1.0
	if successes > 0 {
		lo = inverseRegularizedBeta(alpha/2, float64(successes), float64(trials-successes+1))
	}
	if successes < trials {
		hi = inverseRegularizedBeta(1-alpha/2, float64(successes+1), float64(trials-successes))
	}
	return Interval{lo, hi}, nil
}

func inverseRegularizedBeta(p, a, b float64) float64 {
	lo, hi := 0.0, 1.0
	for i := 0; i < 100; i++ {
		mid := (lo + hi) / 2
		if regularizedBeta(mid, a, b) < p {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

func regularizedBeta(x, a, b float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	lga, _ := math.Lgamma(a)
	lgb, _ := math.Lgamma(b)
	lgab, _ := math.Lgamma(a + b)
	bt := math.Exp(lgab - lga - lgb + a*math.Log(x) + b*math.Log1p(-x))
	if x < (a+1)/(a+b+2) {
		return bt * betaCF(a, b, x) / a
	}
	return 1 - bt*betaCF(b, a, 1-x)/b
}

func betaCF(a, b, x float64) float64 {
	const maxIter = 200
	const eps = 3e-14
	const fpmin = 1e-300
	qab, qap, qam := a+b, a+1, a-1
	c := 1.0
	d := 1 - qab*x/qap
	if math.Abs(d) < fpmin {
		d = fpmin
	}
	d = 1 / d
	h := d
	for m := 1; m <= maxIter; m++ {
		m2 := 2 * m
		aa := float64(m) * (b - float64(m)) * x / ((qam + float64(m2)) * (a + float64(m2)))
		d = 1 + aa*d
		if math.Abs(d) < fpmin {
			d = fpmin
		}
		c = 1 + aa/c
		if math.Abs(c) < fpmin {
			c = fpmin
		}
		d = 1 / d
		h *= d * c
		aa = -(a + float64(m)) * (qab + float64(m)) * x / ((a + float64(m2)) * (qap + float64(m2)))
		d = 1 + aa*d
		if math.Abs(d) < fpmin {
			d = fpmin
		}
		c = 1 + aa/c
		if math.Abs(c) < fpmin {
			c = fpmin
		}
		d = 1 / d
		delta := d * c
		h *= delta
		if math.Abs(delta-1) < eps {
			break
		}
	}
	return h
}
