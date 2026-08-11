package experiment

type StageRequest struct {
	Index         uint64  `json:"index"`
	Phase         Phase   `json:"phase"`
	RolloutKey    string  `json:"rollout_key"`
	UserID        string  `json:"user_id"`
	RequestID     string  `json:"request_id"`
	International bool    `json:"international"`
	Country       string  `json:"country"`
	Currency      string  `json:"currency"`
	Bucket        float64 `json:"bucket"`
	Branch        Branch  `json:"branch"`
}

type StagePlan struct {
	Schema   string         `json:"schema"`
	RunID    string         `json:"run_id"`
	StageID  string         `json:"stage_id"`
	Weight   float64        `json:"weight"`
	Requests []StageRequest `json:"requests"`
}

func BuildStagePlan(runSeed []byte, runID, stageID string, weight float64, warmup, measured int, measuredPhase Phase) (StagePlan, error) {
	seeds := DeriveSeeds(runSeed)
	plan := StagePlan{Schema: "emac.stage-plan/v1", RunID: runID, StageID: stageID, Weight: weight, Requests: make([]StageRequest, 0, warmup+measured)}
	for i := 0; i < warmup+measured; i++ {
		phase := measuredPhase
		if i < warmup {
			phase = PhaseWarmup
		}
		index := uint64(i)
		rk, uid, rid := Identity(seeds, runID, stageID, index)
		intl := International(seeds.Eligibility, runID, index)
		bucket := Bucket(seeds.Rollout, rk)
		branch, err := Assign(intl, bucket, weight)
		if err != nil {
			return StagePlan{}, err
		}
		country, currency := "United States", "USD"
		if intl {
			country, currency = "Canada", "CAD"
		}
		plan.Requests = append(plan.Requests, StageRequest{index, phase, rk, uid, rid, intl, country, currency, bucket, branch})
	}
	return plan, nil
}
