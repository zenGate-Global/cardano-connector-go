package kupmios

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/Salvionied/apollo/v2/backend"
	"github.com/SundaeSwap-finance/kugo"
	ogmigo "github.com/SundaeSwap-finance/ogmigo/v6"
	"github.com/SundaeSwap-finance/ogmigo/v6/ouroboros/chainsync/num"
	"github.com/SundaeSwap-finance/ogmigo/v6/ouroboros/shared"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// chainFetcher resolves datums and reference scripts by hash. It is implemented
// by *kugo.Client via the Kupo /v1/datums/{hash} and /v1/scripts/{hash}
// endpoints. Kupo's /matches response carries only the datum hash and script
// hash (not the resolved datum/script bytes), so both must be fetched
// separately.
type chainFetcher interface {
	Datum(ctx context.Context, datumHash string) (string, error)
	Script(ctx context.Context, scriptHash string) (*kugo.Script, error)
}

// matchToUtxo converts a kugo.Match into a gouroboros common.Utxo. Inline
// datums are resolved (and hash-verified) via the supplied datumFetcher.
func matchToUtxo(
	ctx context.Context,
	match kugo.Match,
	address common.Address,
	fetcher chainFetcher,
) (common.Utxo, error) {
	hashBytes, err := hex.DecodeString(match.TransactionID)
	if err != nil {
		return common.Utxo{}, err
	}
	if len(hashBytes) != common.Blake2b256Size {
		return common.Utxo{}, fmt.Errorf(
			"invalid tx hash length: expected %d bytes, got %d",
			common.Blake2b256Size,
			len(hashBytes),
		)
	}
	var txId common.Blake2b256
	copy(txId[:], hashBytes)
	if match.OutputIndex < 0 {
		return common.Utxo{}, fmt.Errorf(
			"negative output index: %d",
			match.OutputIndex,
		)
	}
	if match.OutputIndex > math.MaxUint32 {
		return common.Utxo{}, fmt.Errorf(
			"output index %d exceeds uint32 range",
			match.OutputIndex,
		)
	}
	utxo, err := sharedValueToUtxo(
		txId,
		uint32(match.OutputIndex),
		shared.Value(match.Value),
		address,
	)
	if err != nil {
		return common.Utxo{}, err
	}
	output, ok := utxo.Output.(*babbage.BabbageTransactionOutput)
	if !ok {
		return common.Utxo{}, fmt.Errorf(
			"unexpected UTxO output type: %T",
			utxo.Output,
		)
	}

	// Set datum option from kupo match data. Kupo only returns the datum hash
	// in matches; its datum_type discriminator says whether the on-chain
	// output carried an inline datum or just the hash.
	if match.DatumHash != "" {
		switch match.DatumType {
		case "inline":
			opt, err := fetchInlineDatumOption(ctx, fetcher, match.DatumHash)
			if err != nil {
				return common.Utxo{}, err
			}
			output.DatumOption = opt
		case "hash":
			opt, err := parseDatumOption(match.DatumHash)
			if err != nil {
				return common.Utxo{}, fmt.Errorf(
					"failed to parse datum option: %w",
					err,
				)
			}
			output.DatumOption = opt
		default:
			return common.Utxo{}, fmt.Errorf(
				"unsupported kupo datum type %q for datum hash %s",
				match.DatumType,
				match.DatumHash,
			)
		}
	}

	// Set the reference script. Kupo's /matches response carries only the
	// script hash, not the resolved script bytes, so when a script hash is
	// present we resolve the script via /v1/scripts/{hash}. The resolved bytes
	// are verified against the claimed hash by kupoScriptToScriptRef.
	//
	// Chain-read hydration is best-effort: if the script cannot be resolved
	// (empty/invalid body, transient failure) or parsed, do NOT abort the whole
	// fetch; keep the UTxO with an unresolved (nil) reference script.
	script := match.Script
	if script.Script == "" && match.ScriptHash != "" {
		fetched, err := fetcher.Script(ctx, match.ScriptHash)
		if err != nil {
			slog.Warn("kupmios: leaving reference script unresolved during hydration",
				"script_hash", match.ScriptHash,
				"utxo", fmt.Sprintf("%s#%d", match.TransactionID, match.OutputIndex),
				"err", err)
		} else if fetched != nil {
			script = *fetched
		}
	}
	if script.Script != "" {
		ref, err := kupoScriptToScriptRef(script, match.ScriptHash)
		if err != nil {
			slog.Warn("kupmios: leaving reference script unresolved during hydration (parse failed)",
				"script_hash", match.ScriptHash,
				"utxo", fmt.Sprintf("%s#%d", match.TransactionID, match.OutputIndex),
				"err", err)
		} else {
			output.TxOutScriptRef = ref
		}
	}

	return utxo, nil
}

// fetchInlineDatumOption fetches the inline datum bytes for the given datum
// hash from Kupo and builds an inline datum option. The fetched bytes are
// verified against the datum hash before use; a mismatch fails closed.
func fetchInlineDatumOption(
	ctx context.Context,
	datums chainFetcher,
	datumHashHex string,
) (*babbage.BabbageTransactionOutputDatumOption, error) {
	if datums == nil {
		return nil, fmt.Errorf(
			"kupo client required to resolve inline datum %s",
			datumHashHex,
		)
	}
	expectedBytes, err := hex.DecodeString(datumHashHex)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid datum hash hex %q: %w",
			datumHashHex,
			err,
		)
	}
	if len(expectedBytes) != common.Blake2b256Size {
		return nil, fmt.Errorf(
			"invalid datum hash length: expected %d bytes, got %d",
			common.Blake2b256Size,
			len(expectedBytes),
		)
	}
	var expected common.Blake2b256
	copy(expected[:], expectedBytes)

	datumCborHex, err := datums.Datum(ctx, datumHashHex)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to fetch inline datum %s: %w",
			datumHashHex,
			err,
		)
	}
	if datumCborHex == "" {
		return nil, fmt.Errorf(
			"kupo returned no datum for inline datum hash %s",
			datumHashHex,
		)
	}
	datumBytes, err := hex.DecodeString(datumCborHex)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid inline datum CBOR hex %q: %w",
			datumCborHex,
			err,
		)
	}
	if computed := common.Blake2b256Hash(datumBytes); computed != expected {
		return nil, fmt.Errorf(
			"inline datum hash mismatch for %s: fetched datum hashes to %s",
			datumHashHex,
			hex.EncodeToString(computed.Bytes()),
		)
	}
	return parseInlineDatumCbor(datumCborHex)
}

// ogmiosUtxoToCommon converts an ogmigo shared.Utxo (as returned by
// UtxosByTxIn) into a gouroboros common.Utxo.
func ogmiosUtxoToCommon(
	raw shared.Utxo,
	addr common.Address,
) (common.Utxo, error) {
	hashBytes, err := hex.DecodeString(raw.Transaction.ID)
	if err != nil {
		return common.Utxo{}, err
	}
	if len(hashBytes) != common.Blake2b256Size {
		return common.Utxo{}, fmt.Errorf(
			"invalid tx hash length: expected %d bytes, got %d",
			common.Blake2b256Size,
			len(hashBytes),
		)
	}
	var txId common.Blake2b256
	copy(txId[:], hashBytes)
	utxo, err := sharedValueToUtxo(txId, raw.Index, raw.Value, addr)
	if err != nil {
		return common.Utxo{}, err
	}
	output, ok := utxo.Output.(*babbage.BabbageTransactionOutput)
	if !ok {
		return common.Utxo{}, fmt.Errorf(
			"unexpected UTxO output type: %T",
			utxo.Output,
		)
	}

	// Set datum option from ogmios UTxO data. Ogmios provides inline datum
	// CBOR hex in the Datum field and the datum hash in DatumHash.
	if raw.Datum != "" {
		opt, err := parseInlineDatumCbor(raw.Datum)
		if err != nil {
			return common.Utxo{}, fmt.Errorf(
				"failed to parse inline datum: %w",
				err,
			)
		}
		output.DatumOption = opt
	} else if raw.DatumHash != "" {
		opt, err := parseDatumOption(raw.DatumHash)
		if err != nil {
			return common.Utxo{}, fmt.Errorf(
				"failed to parse datum hash: %w",
				err,
			)
		}
		output.DatumOption = opt
	}

	// Set script reference from ogmios UTxO data. Chain-read hydration is
	// best-effort: a malformed reference script must not abort the fetch; keep
	// the UTxO with an unresolved (nil) reference script.
	if len(raw.Script) > 0 && string(raw.Script) != "null" {
		ref, err := ogmiosScriptToScriptRef(raw.Script)
		if err != nil {
			slog.Warn("kupmios: leaving reference script unresolved during hydration (ogmios script parse failed)",
				"utxo", fmt.Sprintf("%s#%d", raw.Transaction.ID, raw.Index),
				"err", err)
		} else if ref != nil {
			output.TxOutScriptRef = ref
		}
	}

	return utxo, nil
}

// sharedValueToUtxo builds a common.Utxo from an ogmigo shared.Value and the
// owning address. The output is a babbage output with no datum/script set;
// callers attach those afterwards.
func sharedValueToUtxo(
	txId common.Blake2b256,
	outputIndex uint32,
	value shared.Value,
	addr common.Address,
) (common.Utxo, error) {
	input := shelley.ShelleyTransactionInput{
		TxId:        txId,
		OutputIndex: outputIndex,
	}

	// Require int64 range (not just uint64) to keep downstream signed lovelace
	// arithmetic safe.
	lovelaceBig := value.AdaLovelace().BigInt()
	if lovelaceBig.Sign() < 0 || !lovelaceBig.IsInt64() {
		return common.Utxo{}, fmt.Errorf(
			"invalid lovelace quantity %s",
			lovelaceBig.String(),
		)
	}
	lovelace := lovelaceBig.Uint64()
	assetData := make(map[common.Blake2b224]map[cbor.ByteString]*big.Int)

	for policyIdStr, assets := range value {
		if policyIdStr == shared.AdaPolicy {
			continue
		}
		policyBytes, err := hex.DecodeString(policyIdStr)
		if err != nil {
			return common.Utxo{}, fmt.Errorf(
				"invalid policy ID hex %q: %w",
				policyIdStr,
				err,
			)
		}
		if len(policyBytes) != common.Blake2b224Size {
			return common.Utxo{}, fmt.Errorf(
				"invalid policy ID length for %q: expected %d bytes, got %d",
				policyIdStr,
				common.Blake2b224Size,
				len(policyBytes),
			)
		}
		var policyId common.Blake2b224
		copy(policyId[:], policyBytes)

		for assetName, qty := range assets {
			qtyBig := qty.BigInt()
			if qtyBig.Sign() < 0 {
				return common.Utxo{}, fmt.Errorf(
					"negative asset quantity %s for policy %s asset %s",
					qtyBig.String(),
					policyIdStr,
					assetName,
				)
			}
			nameBytes, err := hex.DecodeString(assetName)
			if err != nil {
				return common.Utxo{}, fmt.Errorf(
					"invalid asset name hex %q: %w (asset names must be hex-encoded)",
					assetName,
					err,
				)
			}
			if _, ok := assetData[policyId]; !ok {
				assetData[policyId] = make(map[cbor.ByteString]*big.Int)
			}
			assetData[policyId][cbor.NewByteString(nameBytes)] = new(big.Int).Set(qtyBig)
		}
	}

	var assets *common.MultiAsset[common.MultiAssetTypeOutput]
	if len(assetData) > 0 {
		ma := common.NewMultiAsset[common.MultiAssetTypeOutput](assetData)
		assets = &ma
	}

	output := babbage.BabbageTransactionOutput{
		OutputAddress: addr,
		OutputAmount: mary.MaryTransactionOutputValue{
			Amount: lovelace,
			Assets: assets,
		},
	}

	return common.Utxo{
		Id:     input,
		Output: &output,
	}, nil
}

// parseDatumOption constructs a BabbageTransactionOutputDatumOption from a
// datum hash hex string (datum hash reference, type 0).
func parseDatumOption(
	datumHashHex string,
) (*babbage.BabbageTransactionOutputDatumOption, error) {
	hashBytes, err := hex.DecodeString(datumHashHex)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid datum hash hex %q: %w",
			datumHashHex,
			err,
		)
	}
	if len(hashBytes) != common.Blake2b256Size {
		return nil, fmt.Errorf(
			"invalid datum hash length: expected %d bytes, got %d",
			common.Blake2b256Size,
			len(hashBytes),
		)
	}
	var hash common.Blake2b256
	copy(hash[:], hashBytes)

	cborBytes, err := cbor.Encode([]any{0, hash})
	if err != nil {
		return nil, fmt.Errorf("failed to encode datum option: %w", err)
	}
	var opt babbage.BabbageTransactionOutputDatumOption
	if err := opt.UnmarshalCBOR(cborBytes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal datum option: %w", err)
	}
	return &opt, nil
}

// parseInlineDatumCbor constructs a BabbageTransactionOutputDatumOption for an
// inline datum from its CBOR hex representation.
func parseInlineDatumCbor(
	datumCborHex string,
) (*babbage.BabbageTransactionOutputDatumOption, error) {
	datumBytes, err := hex.DecodeString(datumCborHex)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid datum CBOR hex %q: %w",
			datumCborHex,
			err,
		)
	}
	// Inline datum option: [1, #6.24(datum_cbor)]
	cborBytes, err := cbor.Encode(
		[]any{1, cbor.Tag{Number: 24, Content: datumBytes}},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to encode inline datum option: %w",
			err,
		)
	}
	var opt babbage.BabbageTransactionOutputDatumOption
	if err := opt.UnmarshalCBOR(cborBytes); err != nil {
		return nil, fmt.Errorf(
			"failed to unmarshal inline datum option: %w",
			err,
		)
	}
	return &opt, nil
}

// kupoScriptToScriptRef converts a kugo Script to a common.ScriptRef. The
// script bytes are verified against the script hash claimed by kupo.
func kupoScriptToScriptRef(
	script kugo.Script,
	expectedHashHex string,
) (*common.ScriptRef, error) {
	scriptBytes, err := hex.DecodeString(script.Script)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid script hex %q: %w",
			script.Script,
			err,
		)
	}

	var scriptType uint
	switch script.Language {
	case kugo.ScriptLanguageNative:
		scriptType = common.ScriptRefTypeNativeScript
	case kugo.ScriptLanguagePlutusV1:
		scriptType = common.ScriptRefTypePlutusV1
	case kugo.ScriptLanguagePlutusV2:
		scriptType = common.ScriptRefTypePlutusV2
	case kugo.ScriptLanguagePlutusV3:
		scriptType = common.ScriptRefTypePlutusV3
	default:
		return nil, fmt.Errorf(
			"unsupported kupo script language: %d",
			script.Language,
		)
	}

	return backend.ScriptRefFromBytes(scriptType, scriptBytes, expectedHashHex)
}

// ogmiosScriptToScriptRef converts an Ogmios script JSON to a common.ScriptRef.
// Ogmios v6 uses {"language": "plutus:v1"|"plutus:v2"|"plutus:v3"|"native",
// "cbor": "hex"}. Ogmios does not include the script hash in UTxO responses, so
// no hash verification is possible here.
func ogmiosScriptToScriptRef(
	scriptJSON json.RawMessage,
) (*common.ScriptRef, error) {
	var raw struct {
		Language string `json:"language"`
		Cbor     string `json:"cbor"`
	}
	if err := json.Unmarshal(scriptJSON, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse script JSON: %w", err)
	}
	if raw.Cbor == "" {
		// Native scripts may use "json" field instead of "cbor"; skip these.
		return nil, nil
	}

	scriptBytes, err := hex.DecodeString(raw.Cbor)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid script CBOR hex %q: %w",
			raw.Cbor,
			err,
		)
	}

	var scriptType uint
	switch raw.Language {
	case "native":
		scriptType = common.ScriptRefTypeNativeScript
	case "plutus:v1":
		scriptType = common.ScriptRefTypePlutusV1
	case "plutus:v2":
		scriptType = common.ScriptRefTypePlutusV2
	case "plutus:v3":
		scriptType = common.ScriptRefTypePlutusV3
	default:
		return nil, fmt.Errorf(
			"unsupported ogmios script language %q",
			raw.Language,
		)
	}

	return backend.ScriptRefFromBytes(scriptType, scriptBytes, "")
}

// commonUtxosToShared converts resolved gouroboros UTxOs into the ogmigo
// shared.Utxo wire form expected by EvaluateTxWithAdditionalUtxos.
func commonUtxosToShared(utxos []common.Utxo) ([]shared.Utxo, error) {
	result := make([]shared.Utxo, 0, len(utxos))
	for _, utxo := range utxos {
		su, err := commonUtxoToShared(utxo)
		if err != nil {
			return nil, err
		}
		result = append(result, su)
	}
	return result, nil
}

// commonUtxoToShared converts a single resolved gouroboros UTxO into an ogmigo
// shared.Utxo. The value is encoded as Ogmios expects: the outer key is "ada"
// (with inner key "lovelace") for the coin, and the policy ID hex (with inner
// asset-name hex) for native assets.
func commonUtxoToShared(utxo common.Utxo) (shared.Utxo, error) {
	out := utxo.Output

	coin, err := bigIntToNum(out.Amount())
	if err != nil {
		return shared.Utxo{}, fmt.Errorf("invalid lovelace amount: %w", err)
	}
	value := shared.Value{
		shared.AdaPolicy: {
			shared.AdaAsset: coin,
		},
	}
	if assets := out.Assets(); assets != nil {
		for _, policyId := range assets.Policies() {
			policyHex := hex.EncodeToString(policyId.Bytes())
			for _, assetName := range assets.Assets(policyId) {
				qty, err := bigIntToNum(assets.Asset(policyId, assetName))
				if err != nil {
					return shared.Utxo{}, fmt.Errorf(
						"invalid asset quantity for policy %s: %w",
						policyHex,
						err,
					)
				}
				if value[policyHex] == nil {
					value[policyHex] = map[string]num.Int{}
				}
				value[policyHex][hex.EncodeToString(assetName)] = qty
			}
		}
	}

	su := shared.Utxo{
		Transaction: shared.UtxoTxID{
			ID: hex.EncodeToString(utxo.Id.Id().Bytes()),
		},
		Index:   utxo.Id.Index(),
		Address: out.Address().String(),
		Value:   value,
	}

	// Datum: inline datum CBOR hex goes in Datum, a bare datum hash in
	// DatumHash.
	if datum := out.Datum(); datum != nil {
		datumCbor, err := datum.MarshalCBOR()
		if err != nil {
			return shared.Utxo{}, fmt.Errorf(
				"failed to encode inline datum: %w",
				err,
			)
		}
		su.Datum = hex.EncodeToString(datumCbor)
	} else if datumHash := out.DatumHash(); datumHash != nil {
		su.DatumHash = hex.EncodeToString(datumHash.Bytes())
	}

	// Reference script: Ogmios expects {"language": ..., "cbor": ...}.
	if script := out.ScriptRef(); script != nil {
		scriptJSON, err := ogmiosScriptRefJSON(script)
		if err != nil {
			return shared.Utxo{}, err
		}
		su.Script = scriptJSON
	}

	return su, nil
}

// bigIntToNum converts a big.Int quantity into the ogmigo num.Int used by
// shared.Value, preserving the full magnitude (no int64 truncation).
func bigIntToNum(v *big.Int) (num.Int, error) {
	if v == nil {
		return num.Int64(0), nil
	}
	n, ok := num.New(v.String())
	if !ok {
		return num.Int{}, fmt.Errorf("cannot represent quantity %s", v.String())
	}
	return n, nil
}

// ogmiosScriptRefJSON encodes a reference script as the Ogmios script JSON
// object ({"language": "plutus:vN"|"native", "cbor": "<hex>"}).
func ogmiosScriptRefJSON(script common.Script) (json.RawMessage, error) {
	var language string
	switch script.(type) {
	case common.PlutusV1Script:
		language = "plutus:v1"
	case common.PlutusV2Script:
		language = "plutus:v2"
	case common.PlutusV3Script:
		language = "plutus:v3"
	case common.NativeScript:
		language = "native"
	default:
		return nil, fmt.Errorf("unsupported reference script type %T", script)
	}
	payload := struct {
		Language string `json:"language"`
		Cbor     string `json:"cbor"`
	}{
		Language: language,
		Cbor:     hex.EncodeToString(script.RawScriptBytes()),
	}
	return json.Marshal(payload)
}

// parseRedeemerPurpose maps an Ogmios redeemer purpose string to a gouroboros
// RedeemerTag. backend.ParseRedeemerTag accepts spend/mint/cert/publish/reward/
// withdraw; Ogmios v5 additionally emits the long spellings "certificate" and
// "withdrawal", so those are normalized to the accepted forms first
// (case-insensitively) before delegating.
func parseRedeemerPurpose(purpose string) (common.RedeemerTag, error) {
	switch strings.ToLower(strings.TrimSpace(purpose)) {
	case "certificate":
		return backend.ParseRedeemerTag("cert")
	case "withdrawal":
		return backend.ParseRedeemerTag("withdraw")
	default:
		return backend.ParseRedeemerTag(purpose)
	}
}

// evaluateResponseToExUnits converts an ogmigo EvaluateTxResponse into a
// redeemer ExUnits map. A response with zero evaluation results is an error.
func evaluateResponseToExUnits(
	resp *ogmigo.EvaluateTxResponse,
) (map[common.RedeemerKey]common.ExUnits, error) {
	if resp == nil {
		return nil, errors.New("empty evaluate response")
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("evaluate tx error: %s", resp.Error.Message)
	}
	if len(resp.ExUnits) == 0 {
		return nil, errors.New("script evaluation returned no results")
	}

	result := make(map[common.RedeemerKey]common.ExUnits, len(resp.ExUnits))
	for _, eu := range resp.ExUnits {
		tag, err := parseRedeemerPurpose(eu.Validator.Purpose)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid redeemer purpose %q: %w",
				eu.Validator.Purpose,
				err,
			)
		}
		if eu.Validator.Index > math.MaxUint32 {
			return nil, fmt.Errorf(
				"redeemer index %d exceeds uint32 range",
				eu.Validator.Index,
			)
		}
		key := common.RedeemerKey{Tag: tag, Index: uint32(eu.Validator.Index)}
		if eu.Budget.Memory > math.MaxInt64 || eu.Budget.Cpu > math.MaxInt64 {
			return nil, fmt.Errorf(
				"ExUnits overflow: memory=%d cpu=%d",
				eu.Budget.Memory,
				eu.Budget.Cpu,
			)
		}
		result[key] = common.ExUnits{
			Memory: int64(eu.Budget.Memory),
			Steps:  int64(eu.Budget.Cpu),
		}
	}
	return result, nil
}

// toProtocolParams maps the Ogmios protocol parameter response onto the
// apollo v2 backend.ProtocolParameters struct.
func (p *ogmiosProtocolParams) toProtocolParams() (backend.ProtocolParameters, error) {
	priceMem, err := backend.ParseFraction(p.ScriptPrices.Memory)
	if err != nil {
		return backend.ProtocolParameters{}, fmt.Errorf(
			"invalid memory price: %w",
			err,
		)
	}
	priceStep, err := backend.ParseFraction(p.ScriptPrices.CPU)
	if err != nil {
		return backend.ProtocolParameters{}, fmt.Errorf(
			"invalid CPU price: %w",
			err,
		)
	}

	var poolInfluence, monetaryExpansion, treasuryExpansion float64
	if p.PoolInfluence != "" {
		poolInfluence, err = backend.ParseFraction(p.PoolInfluence)
		if err != nil {
			return backend.ProtocolParameters{}, fmt.Errorf(
				"invalid pool influence: %w",
				err,
			)
		}
	}
	if p.MonetaryExpansion != "" {
		monetaryExpansion, err = backend.ParseFraction(p.MonetaryExpansion)
		if err != nil {
			return backend.ProtocolParameters{}, fmt.Errorf(
				"invalid monetary expansion: %w",
				err,
			)
		}
	}
	if p.TreasuryExpansion != "" {
		treasuryExpansion, err = backend.ParseFraction(p.TreasuryExpansion)
		if err != nil {
			return backend.ProtocolParameters{}, fmt.Errorf(
				"invalid treasury expansion: %w",
				err,
			)
		}
	}

	pp := backend.ProtocolParameters{
		MinFeeConstant:                   p.MinFeeConstant.lovelace(),
		MinFeeCoefficient:                p.MinFeeCoefficient,
		MaxBlockSize:                     p.MaxBlockBodySize.Bytes,
		MaxTxSize:                        p.MaxTxSize.Bytes,
		MaxBlockHeaderSize:               p.MaxBlockHeaderSize.Bytes,
		KeyDeposits:                      strconv.FormatInt(p.StakeKeyDeposit.lovelace(), 10),
		PoolDeposits:                     strconv.FormatInt(p.PoolDeposit.lovelace(), 10),
		PoolInfluence:                    poolInfluence,
		MonetaryExpansion:                monetaryExpansion,
		TreasuryExpansion:                treasuryExpansion,
		MinPoolCost:                      strconv.FormatInt(p.MinPoolCost.lovelace(), 10),
		MinUtxo:                          strconv.FormatInt(p.MinUtxoConstant.lovelace(), 10),
		ProtocolMajorVersion:             p.Version.Major,
		ProtocolMinorVersion:             p.Version.Minor,
		PriceMem:                         priceMem,
		PriceStep:                        priceStep,
		MaxTxExMem:                       strconv.FormatInt(p.MaxTxExUnits.Memory, 10),
		MaxTxExSteps:                     strconv.FormatInt(p.MaxTxExUnits.CPU, 10),
		MaxBlockExMem:                    strconv.FormatInt(p.MaxBlockExUnits.Memory, 10),
		MaxBlockExSteps:                  strconv.FormatInt(p.MaxBlockExUnits.CPU, 10),
		MaxValSize:                       strconv.Itoa(p.MaxValSize.Bytes),
		CollateralPercent:                p.CollateralPercent,
		MaxCollateralInputs:              p.MaxCollateral,
		CoinsPerUtxoByte:                 strconv.FormatInt(p.MinUtxoDeposit, 10),
		CoinsPerUtxoWord:                 strconv.FormatInt(p.MinUtxoDeposit, 10),
		MaximumReferenceScriptsSize:      p.MaxRefScriptsSize.Bytes,
		MinFeeReferenceScriptsRange:      p.MinFeeRefScripts.Range,
		MinFeeReferenceScriptsBase:       int(p.MinFeeRefScripts.Base),
		MinFeeReferenceScriptsMultiplier: int(p.MinFeeRefScripts.Multiplier),
	}

	// Parse cost models from Ogmios JSON. Ogmios uses keys like "plutus:v1",
	// "plutus:v2", "plutus:v3"; ComputeScriptDataHash expects "PlutusV1",
	// "PlutusV2", "PlutusV3".
	if len(p.CostModels) > 0 {
		pp.CostModels = make(map[string][]int64, len(p.CostModels))
		for key, costs := range p.CostModels {
			pp.CostModels[ogmiosCostModelKey(key)] = costs
		}
	}

	return pp, nil
}

// ogmiosCostModelKey translates Ogmios cost model keys to the canonical form
// expected by ComputeScriptDataHash ("PlutusV1", "PlutusV2", "PlutusV3").
func ogmiosCostModelKey(key string) string {
	switch key {
	case "plutus:v1":
		return "PlutusV1"
	case "plutus:v2":
		return "PlutusV2"
	case "plutus:v3":
		return "PlutusV3"
	default:
		return key
	}
}

// toGenesisParams maps the Ogmios shelley genesis configuration onto the
// apollo v2 backend.GenesisParameters struct.
func (g *ogmiosGenesisConfig) toGenesisParams() (backend.GenesisParameters, error) {
	activeSlots, err := backend.ParseFraction(g.ActiveSlots)
	if err != nil {
		return backend.GenesisParameters{}, fmt.Errorf(
			"invalid active slots coefficient: %w",
			err,
		)
	}

	var systemStart int64
	if g.StartTime != "" {
		t, err := time.Parse("2006-01-02T15:04:05Z", g.StartTime)
		if err != nil {
			return backend.GenesisParameters{}, fmt.Errorf(
				"invalid genesis start time %q: %w",
				g.StartTime,
				err,
			)
		}
		systemStart = t.Unix()
	}

	// Ogmios reports slot length in milliseconds; genesis params expect seconds.
	slotLength := g.SlotLength.Milliseconds / 1000

	return backend.GenesisParameters{
		ActiveSlotsCoefficient: activeSlots,
		UpdateQuorum:           g.UpdateQuorum,
		MaxLovelaceSupply:      strconv.FormatInt(g.MaxLovelaceSupply, 10),
		NetworkMagic:           g.NetworkMagic,
		EpochLength:            g.EpochLength,
		SystemStart:            systemStart,
		SlotsPerKesPeriod:      g.SlotsPerKesPeriod,
		SlotLength:             slotLength,
		MaxKesEvolutions:       g.MaxKesEvolutions,
		SecurityParam:          g.SecurityParam,
	}, nil
}
