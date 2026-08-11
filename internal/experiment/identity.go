package experiment

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

type Phase string

const (
	PhaseSetup    Phase = "setup"
	PhaseWarmup   Phase = "warmup"
	PhaseMeasured Phase = "measured"
	PhaseOracle   Phase = "oracle"
)

type Branch string

const (
	Candidate           Branch = "candidate"
	StableInternational Branch = "stable_international"
	StableDomestic      Branch = "stable_domestic"
)

// Seeds are domain-separated from a single registered run seed.
type Seeds struct {
	Identity    [32]byte
	Eligibility [32]byte
	Rollout     [32]byte
}

func DeriveSeeds(runSeed []byte) Seeds {
	return Seeds{
		Identity:    hkdf(runSeed, "identity"),
		Eligibility: hkdf(runSeed, "eligibility"),
		Rollout:     hkdf(runSeed, "rollout"),
	}
}

// Identity returns a stage-independent rollout key and stage-specific UUIDv5
// user/request identities. Repeating a rollout key at later stages cannot
// re-randomize its bucket.
func Identity(seeds Seeds, _ string, stageID string, requestIndex uint64) (rolloutKey, userID, requestID string) {
	rolloutKey = uuidV5(namespaceRollout, fmt.Sprintf("%x:%d", seeds.Rollout, requestIndex))
	userID = uuidV5(namespaceUser, fmt.Sprintf("%x:%s:%d", seeds.Identity, stageID, requestIndex))
	requestID = uuidV5(namespaceRequest, fmt.Sprintf("%x:%s:%d:checkout", seeds.Identity, stageID, requestIndex))
	return
}

// International implements the registered exact 6/4 workload. Within each
// ten-request block, six positions selected by a seeded permutation are
// international; the remaining four are domestic.
func International(seed [32]byte, runID string, requestIndex uint64) bool {
	block := requestIndex / 10
	position := int(requestIndex % 10)
	type ranked struct {
		position int
		score    uint64
	}
	ranks := make([]ranked, 10)
	for i := range ranks {
		ranks[i] = ranked{i, keyedUint64(seed[:], fmt.Sprintf("persona:%s:%d:%d", runID, block, i))}
	}
	sort.Slice(ranks, func(i, j int) bool {
		if ranks[i].score == ranks[j].score {
			return ranks[i].position < ranks[j].position
		}
		return ranks[i].score < ranks[j].score
	})
	for i := 0; i < 6; i++ {
		if ranks[i].position == position {
			return true
		}
	}
	return false
}

func Bucket(seed [32]byte, rolloutKey string) float64 {
	u := keyedUint64(seed[:], "bucket:"+rolloutKey)
	return float64(u>>11) / float64(uint64(1)<<53)
}

func Assign(international bool, bucket, weight float64) (Branch, error) {
	if math.IsNaN(weight) || weight < 0 || weight > 1 {
		return "", fmt.Errorf("weight must be in [0,1]")
	}
	if !international {
		return StableDomestic, nil
	}
	if bucket < weight {
		return Candidate, nil
	}
	return StableInternational, nil
}

func hkdf(input []byte, info string) [32]byte {
	// RFC 5869 extract with the all-zero SHA-256 salt, then one expand block.
	extract := hmac.New(sha256.New, make([]byte, sha256.Size))
	_, _ = extract.Write(input)
	prk := extract.Sum(nil)
	expand := hmac.New(sha256.New, prk)
	_, _ = expand.Write([]byte(info))
	_, _ = expand.Write([]byte{1})
	var out [32]byte
	copy(out[:], expand.Sum(nil))
	return out
}

func keyedUint64(key []byte, text string) uint64 {
	m := hmac.New(sha256.New, key)
	_, _ = m.Write([]byte(text))
	return binary.BigEndian.Uint64(m.Sum(nil)[:8])
}

var (
	namespaceRollout = [16]byte{0x10, 0x96, 0x2b, 0x31, 0xd2, 0xf9, 0x54, 0xf9, 0xb5, 0x1c, 0xd1, 0xef, 0x71, 0xc2, 0xe4, 0x02}
	namespaceUser    = [16]byte{0xa2, 0x45, 0x1f, 0x2d, 0x2a, 0xf5, 0x50, 0x01, 0x9a, 0x31, 0x0c, 0x3c, 0x52, 0xa8, 0x70, 0x11}
	namespaceRequest = [16]byte{0x47, 0xc2, 0x95, 0xe8, 0x4c, 0x91, 0x54, 0x52, 0xb1, 0xb7, 0x4d, 0xc7, 0x18, 0x22, 0x40, 0xa9}
)

func uuidV5(namespace [16]byte, name string) string {
	h := sha1.New()
	_, _ = h.Write(namespace[:])
	_, _ = h.Write([]byte(name))
	b := h.Sum(nil)[:16]
	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
