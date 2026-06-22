package blockfrost

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Salvionied/apollo/serialization"
	"github.com/Salvionied/apollo/serialization/Address"
	"github.com/Salvionied/apollo/serialization/Asset"
	"github.com/Salvionied/apollo/serialization/AssetName"
	"github.com/Salvionied/apollo/serialization/MultiAsset"
	"github.com/Salvionied/apollo/serialization/PlutusData"
	"github.com/Salvionied/apollo/serialization/Policy"
	"github.com/Salvionied/apollo/serialization/TransactionInput"
	"github.com/Salvionied/apollo/serialization/TransactionOutput"
	"github.com/Salvionied/apollo/serialization/UTxO"
	"github.com/Salvionied/apollo/serialization/Value"
	"github.com/Salvionied/cbor/v2"
	"github.com/tj/assert"
)

const testAddr = "addr_test1wpgexmeunzsykesf42d4eqet5yvzeap6trjnflxqtkcf66g0kpnxt"

// rawScriptBytes is an arbitrary but stable byte payload standing in for a
// serialised Plutus script. The version key is what we assert on; the raw
// bytes only need to round-trip unchanged (and crucially un-tagged).
var rawScriptBytes = []byte{0x46, 0x01, 0x00, 0x00, 0x22, 0x00, 0x11}

func buildUtxoWithScriptRef(
	t *testing.T,
	scriptRef *PlutusData.ScriptRef,
) UTxO.UTxO {
	t.Helper()

	addr, err := Address.DecodeAddress(testAddr)
	assert.NoError(t, err)

	txId, _ := hex.DecodeString(
		"0000000000000000000000000000000000000000000000000000000000000000",
	)

	out := TransactionOutput.TransactionOutput{
		IsPostAlonzo: true,
		PostAlonzo: TransactionOutput.TransactionOutputAlonzo{
			Address:   addr,
			Amount:    Value.Value{Coin: 2_000_000}.ToAlonzoValue(),
			ScriptRef: scriptRef,
		},
	}

	return UTxO.UTxO{
		Input: TransactionInput.TransactionInput{
			TransactionId: txId,
			Index:         0,
		},
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
	ref, err := PlutusData.NewV1ScriptRef(PlutusData.PlutusV1Script(rawScriptBytes))
	assert.NoError(t, err)

	utxo := buildUtxoWithScriptRef(t, &ref)
	item, err := buildAdditionalUtxoItem(utxo)
	assert.NoError(t, err)

	sr := extractScriptRef(t, item)
	assert.NotNil(t, sr)
	assert.NotNil(t, sr.PlutusV1)
	assert.Nil(t, sr.PlutusV2)
	assert.Nil(t, sr.PlutusV3)
	assert.Equal(t, hex.EncodeToString(rawScriptBytes), *sr.PlutusV1)
}

func TestBuildAdditionalUtxoItemPlutusV2(t *testing.T) {
	ref, err := PlutusData.NewV2ScriptRef(PlutusData.PlutusV2Script(rawScriptBytes))
	assert.NoError(t, err)

	utxo := buildUtxoWithScriptRef(t, &ref)
	item, err := buildAdditionalUtxoItem(utxo)
	assert.NoError(t, err)

	sr := extractScriptRef(t, item)
	assert.NotNil(t, sr)
	assert.Nil(t, sr.PlutusV1)
	assert.NotNil(t, sr.PlutusV2)
	assert.Nil(t, sr.PlutusV3)
	assert.Equal(t, hex.EncodeToString(rawScriptBytes), *sr.PlutusV2)
}

func TestBuildAdditionalUtxoItemPlutusV3(t *testing.T) {
	ref, err := PlutusData.NewV3ScriptRef(PlutusData.PlutusV3Script(rawScriptBytes))
	assert.NoError(t, err)

	utxo := buildUtxoWithScriptRef(t, &ref)
	item, err := buildAdditionalUtxoItem(utxo)
	assert.NoError(t, err)

	sr := extractScriptRef(t, item)
	assert.NotNil(t, sr)
	assert.Nil(t, sr.PlutusV1)
	assert.Nil(t, sr.PlutusV2)
	assert.NotNil(t, sr.PlutusV3)
	assert.Equal(t, hex.EncodeToString(rawScriptBytes), *sr.PlutusV3)
}

// TestBuildAdditionalUtxoItemEmitsRawNotTagged guards against the old bug
// where the whole tag-24-wrapped scriptRef CBOR was emitted instead of the
// raw inner Plutus script bytes.
func TestBuildAdditionalUtxoItemEmitsRawNotTagged(t *testing.T) {
	ref, err := PlutusData.NewV3ScriptRef(PlutusData.PlutusV3Script(rawScriptBytes))
	assert.NoError(t, err)

	// What the buggy code used to emit: the CBOR marshal of the ScriptRef,
	// i.e. tag 24 wrapping the inner [3, bytes].
	taggedCbor, err := cbor.Marshal(&ref)
	assert.NoError(t, err)
	taggedHex := hex.EncodeToString(taggedCbor)

	utxo := buildUtxoWithScriptRef(t, &ref)
	item, err := buildAdditionalUtxoItem(utxo)
	assert.NoError(t, err)

	sr := extractScriptRef(t, item)
	assert.NotNil(t, sr.PlutusV3)
	assert.NotEqual(t, taggedHex, *sr.PlutusV3,
		"must emit raw script bytes, not the tag-24 scriptRef envelope")
	assert.Equal(t, hex.EncodeToString(rawScriptBytes), *sr.PlutusV3)
}

func TestBuildAdditionalUtxoItemNativeScriptUnsupported(t *testing.T) {
	// Construct a real native script ref: inner CBOR is [0, native_script]
	// where native_script is itself an array (a ScriptPubkey [0, keyHash]),
	// NOT a byte string. This guards against decoding element 2 as []byte
	// before checking the script type, which would surface a low-level CBOR
	// type error instead of the clear "unsupported" error.
	nativeScript := []interface{}{uint64(0), make([]byte, 28)}
	inner, err := cbor.Marshal(
		[]interface{}{uint64(scriptTypeNative), nativeScript},
	)
	assert.NoError(t, err)
	ref := PlutusData.ScriptRef(inner)

	utxo := buildUtxoWithScriptRef(t, &ref)
	_, err = buildAdditionalUtxoItem(utxo)
	assert.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "unsupported script type in additional UTxO set"),
		"error should mention unsupported script type, got: %v", err,
	)
}

func TestBuildAdditionalUtxoItemUnknownScriptTypeUnsupported(t *testing.T) {
	// script_type 9 is not a valid Cardano script type.
	inner, err := cbor.Marshal([]interface{}{uint64(9), rawScriptBytes})
	assert.NoError(t, err)
	ref := PlutusData.ScriptRef(inner)

	utxo := buildUtxoWithScriptRef(t, &ref)
	_, err = buildAdditionalUtxoItem(utxo)
	assert.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "unsupported script type in additional UTxO set"),
		"error should mention unsupported script type, got: %v", err,
	)
}

func TestBuildAdditionalUtxoItemDatumHashCasing(t *testing.T) {
	addr, err := Address.DecodeAddress(testAddr)
	assert.NoError(t, err)

	txId, _ := hex.DecodeString(
		"1111111111111111111111111111111111111111111111111111111111111111",
	)
	datumHashBytes, _ := hex.DecodeString(
		"923918e403bf43c34b4ef6b48eb2ee04babed17320d8d1b9ff9ad086e86f44ec",
	)

	out := TransactionOutput.TransactionOutput{
		IsPostAlonzo: false,
		PreAlonzo: TransactionOutput.TransactionOutputShelley{
			Address:   addr,
			Amount:    Value.Value{Coin: 2_000_000},
			DatumHash: serialization.DatumHash{Payload: datumHashBytes},
			HasDatum:  true,
		},
	}

	utxo := UTxO.UTxO{
		Input: TransactionInput.TransactionInput{
			TransactionId: txId,
			Index:         0,
		},
		Output: out,
	}

	item, err := buildAdditionalUtxoItem(utxo)
	assert.NoError(t, err)

	out2, ok := item[1].(bfTxOut)
	assert.True(t, ok)
	assert.NotNil(t, out2.DatumHash)
	assert.Equal(t, hex.EncodeToString(datumHashBytes), *out2.DatumHash)

	// Marshal the whole tx-out and assert the camelCase key is used (not
	// the old snake_case "datum_hash"), and that the value/datum-hash shape
	// matches the authoritative Ogmios v5 schema.
	jsonBytes, err := json.Marshal(out2)
	assert.NoError(t, err)
	js := string(jsonBytes)
	assert.True(t, strings.Contains(js, `"datumHash"`),
		"expected camelCase datumHash key, got: %s", js)
	assert.False(t, strings.Contains(js, `"datum_hash"`),
		"should not contain snake_case datum_hash, got: %s", js)
}

// TestBuildAdditionalUtxoItemPostAlonzoHashDatum covers a post-Alonzo (Babbage)
// output that carries a datum HASH (not an inline datum). GetDatumHash() is nil
// for post-Alonzo outputs, so this exercises the DatumOption.DatumType switch
// and must emit datumHash (and not datum).
func TestBuildAdditionalUtxoItemPostAlonzoHashDatum(t *testing.T) {
	addr, err := Address.DecodeAddress(testAddr)
	assert.NoError(t, err)

	txId, _ := hex.DecodeString(
		"2222222222222222222222222222222222222222222222222222222222222222",
	)
	datumHashBytes, _ := hex.DecodeString(
		"923918e403bf43c34b4ef6b48eb2ee04babed17320d8d1b9ff9ad086e86f44ec",
	)

	out := TransactionOutput.TransactionOutput{
		IsPostAlonzo: true,
		PostAlonzo: TransactionOutput.TransactionOutputAlonzo{
			Address: addr,
			Amount:  Value.Value{Coin: 2_000_000}.ToAlonzoValue(),
			Datum: &PlutusData.DatumOption{
				DatumType: PlutusData.DatumTypeHash,
				Hash:      datumHashBytes,
			},
		},
	}

	utxo := UTxO.UTxO{
		Input: TransactionInput.TransactionInput{
			TransactionId: txId,
			Index:         0,
		},
		Output: out,
	}

	item, err := buildAdditionalUtxoItem(utxo)
	assert.NoError(t, err)

	out2, ok := item[1].(bfTxOut)
	assert.True(t, ok)
	assert.NotNil(t, out2.DatumHash,
		"post-Alonzo hash datum must emit datumHash")
	assert.Equal(t, hex.EncodeToString(datumHashBytes), *out2.DatumHash)
	assert.Nil(t, out2.Datum,
		"datum and datumHash are mutually exclusive")

	js, err := json.Marshal(out2)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(js), `"datumHash"`),
		"expected datumHash key, got: %s", string(js))
}

// TestBuildAdditionalUtxoItemValueShape guards the production-proven Ogmios v5
// value encoding ({coins, assets:{<policy>.<assetNameHex>: qty}}) against
// regression.
func TestBuildAdditionalUtxoItemValueShape(t *testing.T) {
	addr, err := Address.DecodeAddress(testAddr)
	assert.NoError(t, err)

	txId, _ := hex.DecodeString(
		"3333333333333333333333333333333333333333333333333333333333333333",
	)

	policyHex := "00000000000000000000000000000000000000000000000000000001"
	assetNameHex := "deadbeef"

	val := Value.PureLovelaceValue(2_000_000)
	an := AssetName.NewAssetNameFromHexString(assetNameHex)
	ma := MultiAsset.MultiAsset[int64]{
		Policy.PolicyId{Value: policyHex}: Asset.Asset[int64]{*an: 42},
	}
	val.AddAssets(ma)

	out := TransactionOutput.TransactionOutput{
		IsPostAlonzo: true,
		PostAlonzo: TransactionOutput.TransactionOutputAlonzo{
			Address: addr,
			Amount:  val.ToAlonzoValue(),
		},
	}

	utxo := UTxO.UTxO{
		Input: TransactionInput.TransactionInput{
			TransactionId: txId,
			Index:         0,
		},
		Output: out,
	}

	item, err := buildAdditionalUtxoItem(utxo)
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
