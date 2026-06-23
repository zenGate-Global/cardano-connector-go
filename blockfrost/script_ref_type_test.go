package blockfrost

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/tj/assert"
	"github.com/zenGate-Global/cardano-connector-go/tests"
)

func mustBlake2b224(t *testing.T, hashHex string) common.Blake2b224 {
	t.Helper()
	b, err := hex.DecodeString(hashHex)
	assert.NoError(t, err)
	assert.Len(t, b, common.Blake2b224Size)
	var h common.Blake2b224
	copy(h[:], b)
	return h
}

// TestScriptRefFromHashDetectsPlutusVersion uses a real preprod reference script
// to confirm the language is detected by hash match and the resulting ScriptRef
// wraps the raw script bytes unchanged.
func TestScriptRefFromHashDetectsPlutusVersion(t *testing.T) {
	scriptCbor, err := hex.DecodeString(tests.ExpectedScriptCbor)
	assert.NoError(t, err)

	hash := mustBlake2b224(t, tests.ScriptHashToQuery)
	ref, err := scriptRefFromHash(hash, scriptCbor)
	assert.NoError(t, err)
	assert.NotNil(t, ref)
	assert.Equal(t, uint(common.ScriptRefTypePlutusV3), ref.Type, "expected PlutusV3 detection")
	assert.Equal(t, scriptCbor, ref.Script.RawScriptBytes(), "must wrap the raw script bytes unchanged")
}

// TestScriptRefFromHashErrorsOnUnknownHash confirms that a script whose hash
// matches no known language is rejected rather than mislabelled.
func TestScriptRefFromHashErrorsOnUnknownHash(t *testing.T) {
	scriptCbor, err := hex.DecodeString(tests.ExpectedScriptCbor)
	assert.NoError(t, err)

	wrongHash := mustBlake2b224(t, "00000000000000000000000000000000000000000000000000000000")
	_, err = scriptRefFromHash(wrongHash, scriptCbor)
	assert.Error(t, err, "unmatched hash should error")
}

// TestScriptRefFromHashDetectsAllPlutusVersions checks v1/v2/v3 detection by
// using each version's own computed hash.
func TestScriptRefFromHashDetectsAllPlutusVersions(t *testing.T) {
	script := []byte{0x46, 0x01, 0x00, 0x00, 0x22, 0x00, 0x11}

	for _, tc := range []struct {
		name string
		hash common.Blake2b224
		want uint
	}{
		{"v1", common.PlutusV1Script(script).Hash(), common.ScriptRefTypePlutusV1},
		{"v2", common.PlutusV2Script(script).Hash(), common.ScriptRefTypePlutusV2},
		{"v3", common.PlutusV3Script(script).Hash(), common.ScriptRefTypePlutusV3},
	} {
		ref, err := scriptRefFromHash(tc.hash, script)
		assert.NoError(t, err, tc.name)
		assert.NotNil(t, ref, tc.name)
		assert.Equal(t, tc.want, ref.Type, tc.name)
		assert.Equal(t, script, ref.Script.RawScriptBytes(), tc.name)
	}
}
