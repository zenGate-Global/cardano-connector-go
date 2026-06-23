package blockfrost

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/tj/assert"
)

const testAddr = "addr_test1wpgexmeunzsykesf42d4eqet5yvzeap6trjnflxqtkcf66g0kpnxt"

// rawScriptBytes is an arbitrary but stable byte payload standing in for a
// serialised Plutus script. The version key is what we assert on; the raw bytes
// only need to round-trip unchanged (and crucially un-tagged).
var rawScriptBytes = []byte{0x46, 0x01, 0x00, 0x00, 0x22, 0x00, 0x11}

func mustTestAddr(t *testing.T) common.Address {
	t.Helper()
	addr, err := common.NewAddress(testAddr)
	assert.NoError(t, err)
	return addr
}

func buildUtxoWithScriptRef(t *testing.T, scriptRef *common.ScriptRef) common.Utxo {
	t.Helper()

	txId, _ := hex.DecodeString(
		"0000000000000000000000000000000000000000000000000000000000000000",
	)
	var txid common.Blake2b256
	copy(txid[:], txId)

	out := &babbage.BabbageTransactionOutput{
		OutputAddress:  mustTestAddr(t),
		OutputAmount:   mary.MaryTransactionOutputValue{Amount: 2_000_000},
		TxOutScriptRef: scriptRef,
	}

	return common.Utxo{
		Id:     shelley.ShelleyTransactionInput{TxId: txid, OutputIndex: 0},
		Output: out,
	}
}

func extractScriptRef(t *testing.T, item bfAdditionalUtxoItem) *bfScriptRef {
	t.Helper()
	out, ok := item[1].(bfTxOut)
	assert.True(t, ok, "second element should be bfTxOut")
	return out.ScriptRef
}

func TestBuildAdditionalUtxoItemPlutusV1(t *testing.T) {
	ref := &common.ScriptRef{Type: common.ScriptRefTypePlutusV1, Script: common.PlutusV1Script(rawScriptBytes)}

	utxo := buildUtxoWithScriptRef(t, ref)
	item, err := bfAdditionalUtxoItemFromUtxo(utxo)
	assert.NoError(t, err)

	sr := extractScriptRef(t, item)
	assert.NotNil(t, sr)
	assert.NotNil(t, sr.PlutusV1)
	assert.Nil(t, sr.PlutusV2)
	assert.Nil(t, sr.PlutusV3)
	assert.Equal(t, hex.EncodeToString(rawScriptBytes), *sr.PlutusV1)
}

func TestBuildAdditionalUtxoItemPlutusV2(t *testing.T) {
	ref := &common.ScriptRef{Type: common.ScriptRefTypePlutusV2, Script: common.PlutusV2Script(rawScriptBytes)}

	utxo := buildUtxoWithScriptRef(t, ref)
	item, err := bfAdditionalUtxoItemFromUtxo(utxo)
	assert.NoError(t, err)

	sr := extractScriptRef(t, item)
	assert.NotNil(t, sr)
	assert.Nil(t, sr.PlutusV1)
	assert.NotNil(t, sr.PlutusV2)
	assert.Nil(t, sr.PlutusV3)
	assert.Equal(t, hex.EncodeToString(rawScriptBytes), *sr.PlutusV2)
}

func TestBuildAdditionalUtxoItemPlutusV3(t *testing.T) {
	ref := &common.ScriptRef{Type: common.ScriptRefTypePlutusV3, Script: common.PlutusV3Script(rawScriptBytes)}

	utxo := buildUtxoWithScriptRef(t, ref)
	item, err := bfAdditionalUtxoItemFromUtxo(utxo)
	assert.NoError(t, err)

	sr := extractScriptRef(t, item)
	assert.NotNil(t, sr)
	assert.Nil(t, sr.PlutusV1)
	assert.Nil(t, sr.PlutusV2)
	assert.NotNil(t, sr.PlutusV3)
	assert.Equal(t, hex.EncodeToString(rawScriptBytes), *sr.PlutusV3)
}

// TestBuildAdditionalUtxoItemEmitsRawScriptBytes guards that the raw serialised
// script bytes are emitted (RawScriptBytes), not any tag/envelope wrapping.
func TestBuildAdditionalUtxoItemEmitsRawScriptBytes(t *testing.T) {
	script := common.PlutusV3Script(rawScriptBytes)
	ref := &common.ScriptRef{Type: common.ScriptRefTypePlutusV3, Script: script}

	utxo := buildUtxoWithScriptRef(t, ref)
	item, err := bfAdditionalUtxoItemFromUtxo(utxo)
	assert.NoError(t, err)

	sr := extractScriptRef(t, item)
	assert.NotNil(t, sr.PlutusV3)
	assert.Equal(t, hex.EncodeToString(script.RawScriptBytes()), *sr.PlutusV3)
	assert.Equal(t, hex.EncodeToString(rawScriptBytes), *sr.PlutusV3)
}

func TestBuildAdditionalUtxoItemNativeScriptUnsupported(t *testing.T) {
	// A native reference script cannot be encoded for the Ogmios v5 evaluation
	// endpoint and must be rejected.
	native := common.NativeScript{}
	ref := &common.ScriptRef{Type: common.ScriptRefTypeNativeScript, Script: native}

	utxo := buildUtxoWithScriptRef(t, ref)
	_, err := bfAdditionalUtxoItemFromUtxo(utxo)
	assert.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "unsupported script type"),
		"error should mention unsupported script type, got: %v", err,
	)
}

func TestBuildAdditionalUtxoItemDatumHash(t *testing.T) {
	txId, _ := hex.DecodeString(
		"1111111111111111111111111111111111111111111111111111111111111111",
	)
	var txid common.Blake2b256
	copy(txid[:], txId)
	datumHashBytes, _ := hex.DecodeString(
		"923918e403bf43c34b4ef6b48eb2ee04babed17320d8d1b9ff9ad086e86f44ec",
	)
	var datumHash common.Blake2b256
	copy(datumHash[:], datumHashBytes)

	// Datum option [0, hash] is a bare datum hash.
	cborBytes, err := cbor.Encode([]any{0, datumHash})
	assert.NoError(t, err)
	var opt babbage.BabbageTransactionOutputDatumOption
	assert.NoError(t, opt.UnmarshalCBOR(cborBytes))

	out := &babbage.BabbageTransactionOutput{
		OutputAddress: mustTestAddr(t),
		OutputAmount:  mary.MaryTransactionOutputValue{Amount: 2_000_000},
		DatumOption:   &opt,
	}
	utxo := common.Utxo{
		Id:     shelley.ShelleyTransactionInput{TxId: txid, OutputIndex: 0},
		Output: out,
	}

	item, err := bfAdditionalUtxoItemFromUtxo(utxo)
	assert.NoError(t, err)

	out2, ok := item[1].(bfTxOut)
	assert.True(t, ok)
	assert.NotNil(t, out2.DatumHash)
	assert.Equal(t, hex.EncodeToString(datumHashBytes), *out2.DatumHash)
	assert.Nil(t, out2.Datum, "datum and datumHash are mutually exclusive")

	jsonBytes, err := json.Marshal(out2)
	assert.NoError(t, err)
	js := string(jsonBytes)
	assert.True(t, strings.Contains(js, `"datumHash"`),
		"expected camelCase datumHash key, got: %s", js)
	assert.False(t, strings.Contains(js, `"datum_hash"`),
		"should not contain snake_case datum_hash, got: %s", js)
}

// TestBuildAdditionalUtxoItemValueShape guards the Ogmios v5 value encoding
// ({coins, assets:{<policy>.<assetNameHex>: qty}}) against regression.
func TestBuildAdditionalUtxoItemValueShape(t *testing.T) {
	txId, _ := hex.DecodeString(
		"3333333333333333333333333333333333333333333333333333333333333333",
	)
	var txid common.Blake2b256
	copy(txid[:], txId)

	policyHex := "00000000000000000000000000000000000000000000000000000001"
	assetNameHex := "deadbeef"

	var policyId common.Blake2b224
	copy(policyId[:], mustDecodeHexT(t, policyHex))
	assetData := map[common.Blake2b224]map[cbor.ByteString]*big.Int{
		policyId: {cbor.NewByteString(mustDecodeHexT(t, assetNameHex)): big.NewInt(42)},
	}
	ma := common.NewMultiAsset[common.MultiAssetTypeOutput](assetData)

	out := &babbage.BabbageTransactionOutput{
		OutputAddress: mustTestAddr(t),
		OutputAmount:  mary.MaryTransactionOutputValue{Amount: 2_000_000, Assets: &ma},
	}
	utxo := common.Utxo{
		Id:     shelley.ShelleyTransactionInput{TxId: txid, OutputIndex: 0},
		Output: out,
	}

	item, err := bfAdditionalUtxoItemFromUtxo(utxo)
	assert.NoError(t, err)

	out2, ok := item[1].(bfTxOut)
	assert.True(t, ok)
	assert.Equal(t, int64(2_000_000), out2.Value.Coins)
	assert.Equal(t, int64(42), out2.Value.Assets[policyHex+"."+assetNameHex])

	js, err := json.Marshal(out2)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(js), `"coins"`),
		"expected coins key, got: %s", string(js))
}

func mustDecodeHexT(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	assert.NoError(t, err)
	return b
}
