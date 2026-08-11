package experiment

import (
	"encoding/binary"
	"encoding/hex"
	"hash/fnv"
	"strings"
)

const CollectorHashSeed uint32 = 11096231

// Sample reproduces the pinned Collector v0.157.0 hash_seed mode: FNV-1a over
// little-endian seed || raw TraceID, followed by its 14-bit threshold.
func Sample(traceID string, proportion float64) bool {
	if proportion >= 1 {
		return true
	}
	if proportion <= 0 {
		return false
	}
	raw, err := hex.DecodeString(strings.ReplaceAll(traceID, "-", ""))
	if err != nil || len(raw) != 16 {
		return false
	}
	h := fnv.New32a()
	var seed [4]byte
	binary.LittleEndian.PutUint32(seed[:], CollectorHashSeed)
	_, _ = h.Write(seed[:])
	_, _ = h.Write(raw)
	threshold := uint32(float32(proportion*100) * float32(0x4000/100.0))
	return h.Sum32()&0x3fff < threshold
}
