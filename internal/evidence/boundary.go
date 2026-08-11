package evidence

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/a-a-k/emac-otel-demo/internal/ledger"
)

type ExternalRoot struct {
	RequestID  string  `json:"request_id"`
	Phase      string  `json:"phase"`
	Branch     string  `json:"branch"`
	Correct    bool    `json:"correct"`
	DurationMS float64 `json:"duration_ms"`
}

type BoundaryReconciliation struct {
	Valid                bool     `json:"valid"`
	PolicyRoots          int      `json:"policy_roots"`
	ExternalRoots        int      `json:"external_roots"`
	Matched              int      `json:"matched"`
	RequestMatchRate     float64  `json:"request_match_rate"`
	CorrectnessAgreement float64  `json:"correctness_agreement"`
	P95AbsoluteDeltaMS   float64  `json:"p95_absolute_delta_ms"`
	TemporalChecked      bool     `json:"temporal_checked"`
	LabelAgreement       float64  `json:"label_agreement,omitempty"`
	ToleranceMS          float64  `json:"tolerance_ms,omitempty"`
	Conflicts            []string `json:"conflicts,omitempty"`
}

func ReconcileBoundary(ledgerPath, k6LogPath string, deadlineMS float64) (BoundaryReconciliation, error) {
	result := BoundaryReconciliation{}
	policyRoots, err := loadPolicyRoots(ledgerPath)
	if err != nil {
		return result, err
	}
	externalRoots, err := loadExternalRoots(k6LogPath)
	if err != nil {
		return result, err
	}
	result.PolicyRoots, result.ExternalRoots = len(policyRoots), len(externalRoots)
	denominator := len(policyRoots)
	if len(externalRoots) > denominator {
		denominator = len(externalRoots)
	}
	var correctMatches, labelMatches int
	deltas := make([]float64, 0, denominator)
	for requestID, policyRoot := range policyRoots {
		externalRoot, ok := externalRoots[requestID]
		if !ok {
			continue
		}
		result.Matched++
		if policyRoot.RootCorrect == externalRoot.Correct {
			correctMatches++
		}
		policyMS := float64(policyRoot.Root) / 1e6
		deltas = append(deltas, math.Abs(externalRoot.DurationMS-policyMS))
		if deadlineMS > 0 {
			policyY := policyRoot.RootCorrect && policyMS <= deadlineMS
			externalY := externalRoot.Correct && externalRoot.DurationMS <= deadlineMS
			if policyY == externalY {
				labelMatches++
			}
		}
	}
	if denominator > 0 {
		result.RequestMatchRate = float64(result.Matched) / float64(denominator)
	}
	if result.Matched > 0 {
		result.CorrectnessAgreement = float64(correctMatches) / float64(result.Matched)
		result.P95AbsoluteDeltaMS = percentile(deltas, .95)
	}
	result.TemporalChecked = deadlineMS > 0
	if result.TemporalChecked && result.Matched > 0 {
		result.LabelAgreement = float64(labelMatches) / float64(result.Matched)
		result.ToleranceMS = math.Max(10, .1*deadlineMS)
	}
	if result.RequestMatchRate < .999 {
		result.Conflicts = append(result.Conflicts, fmt.Sprintf("request-ID match %.6f < 0.999", result.RequestMatchRate))
	}
	if result.CorrectnessAgreement < .99 {
		result.Conflicts = append(result.Conflicts, fmt.Sprintf("correctness agreement %.6f < 0.99", result.CorrectnessAgreement))
	}
	if result.TemporalChecked && result.P95AbsoluteDeltaMS > result.ToleranceMS {
		result.Conflicts = append(result.Conflicts, fmt.Sprintf("p95 absolute duration delta %.3fms > %.3fms", result.P95AbsoluteDeltaMS, result.ToleranceMS))
	}
	result.Valid = len(result.Conflicts) == 0
	return result, nil
}

func loadPolicyRoots(path string) (map[string]ledger.Request, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	roots := map[string]ledger.Request{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var root ledger.Request
		if err := json.Unmarshal(scanner.Bytes(), &root); err != nil {
			return nil, err
		}
		if root.Phase != "measured" && root.Phase != "oracle" {
			continue
		}
		if _, duplicate := roots[root.RequestID]; duplicate {
			return nil, fmt.Errorf("duplicate policy request_id %s", root.RequestID)
		}
		roots[root.RequestID] = root
	}
	return roots, scanner.Err()
}

func loadExternalRoots(path string) (map[string]ExternalRoot, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	roots := map[string]ExternalRoot{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		index := strings.Index(line, "EMAC_ORACLE ")
		if index < 0 {
			continue
		}
		var root ExternalRoot
		if err := json.Unmarshal([]byte(line[index+len("EMAC_ORACLE "):]), &root); err != nil {
			return nil, fmt.Errorf("decode k6 oracle line: %w", err)
		}
		if _, duplicate := roots[root.RequestID]; duplicate {
			return nil, fmt.Errorf("duplicate external request_id %s", root.RequestID)
		}
		roots[root.RequestID] = root
	}
	return roots, scanner.Err()
}

func percentile(values []float64, probability float64) float64 {
	if len(values) == 0 {
		return 0
	}
	copyOfValues := append([]float64(nil), values...)
	sort.Float64s(copyOfValues)
	index := int(math.Ceil(probability*float64(len(copyOfValues)))) - 1
	if index < 0 {
		index = 0
	}
	return copyOfValues[index]
}
