package blockfrost

import (
	"encoding/hex"
	"testing"

	"github.com/Salvionied/apollo/serialization"
	"github.com/Salvionied/apollo/serialization/PlutusData"
	"github.com/Salvionied/cbor/v2"
	"github.com/tj/assert"
	"github.com/zenGate-Global/cardano-connector-go/tests"
)

// decodeRefInner decodes a ScriptRef's inner [script_type, script_bytes] CBOR.
func decodeRefInner(t *testing.T, ref PlutusData.ScriptRef) (int, []byte) {
	t.Helper()
	var decoded struct {
		_          struct{} `cbor:",toarray"`
		ScriptType int
		Script     []byte
	}
	assert.NoError(t, cbor.Unmarshal([]byte(ref), &decoded))
	return decoded.ScriptType, decoded.Script
}

// TestScriptRefFromHashDetectsPlutusVersion uses a real preprod reference script
// (a Plutus V3 script) to confirm the language is detected by hash match and the
// resulting ScriptRef is the typed [3, raw_script] form.
func TestScriptRefFromHashDetectsPlutusVersion(t *testing.T) {
	scriptCbor, err := hex.DecodeString(tests.ExpectedScriptCbor)
	assert.NoError(t, err)

	ref, err := scriptRefFromHash(tests.ScriptHashToQuery, scriptCbor)
	assert.NoError(t, err)

	scriptType, raw := decodeRefInner(t, ref)
	assert.Equal(t, 3, scriptType, "expected PlutusV3 detection")
	assert.Equal(t, scriptCbor, raw, "must wrap the raw script bytes unchanged")
}

// TestScriptRefFromHashFallsBackOnUnknownHash confirms that a script whose hash
// matches no Plutus version (for example a native script) is preserved as the
// raw CBOR rather than being mislabelled.
func TestScriptRefFromHashFallsBackOnUnknownHash(t *testing.T) {
	scriptCbor, err := hex.DecodeString(tests.ExpectedScriptCbor)
	assert.NoError(t, err)

	wrongHash := "00000000000000000000000000000000000000000000000000000000"
	ref, err := scriptRefFromHash(wrongHash, scriptCbor)
	assert.NoError(t, err)
	assert.Equal(t, scriptCbor, []byte(ref),
		"unmatched hash should fall back to the raw script bytes")
}

// TestScriptRefFromHashDetectsAllPlutusVersions checks v1/v2/v3 detection by
// using each version's own computed hash, so the right language is selected and
// the raw bytes are wrapped unchanged.
func TestScriptRefFromHashDetectsAllPlutusVersions(t *testing.T) {
	script := []byte{0x46, 0x01, 0x00, 0x00, 0x22, 0x00, 0x11}

	v1h, err := PlutusData.PlutusV1Script(script).Hash()
	assert.NoError(t, err)
	v2h, err := PlutusData.PlutusV2Script(script).Hash()
	assert.NoError(t, err)
	v3h, err := PlutusData.PlutusV3Script(script).Hash()
	assert.NoError(t, err)

	for _, tc := range []struct {
		name string
		hash serialization.ScriptHash
		want int
	}{
		{"v1", v1h, 1},
		{"v2", v2h, 2},
		{"v3", v3h, 3},
	} {
		ref, err := scriptRefFromHash(hex.EncodeToString(tc.hash.Bytes()), script)
		assert.NoError(t, err, tc.name)
		scriptType, raw := decodeRefInner(t, ref)
		assert.Equal(t, tc.want, scriptType, tc.name)
		assert.Equal(t, script, raw, tc.name)
	}
}

func TestScriptRefFromHashRejectsInvalidHash(t *testing.T) {
	_, err := scriptRefFromHash("not-hex", []byte{0x01})
	assert.Error(t, err)

	_, err = scriptRefFromHash("00", []byte{0x01})
	assert.Error(t, err, "wrong-length hash should error")
}
