package blockfrost

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/Salvionied/apollo/v2/backend"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	connector "github.com/zenGate-Global/cardano-connector-go"
)

// toUtxo builds a gouroboros common.Utxo from a BlockFrost UTxO, including the
// value (lovelace + native assets) and a bare datum-hash DatumOption. Inline
// datums and reference scripts are layered on afterwards by hydrateUtxo.
func (raw *bfAddressUTxO) toUtxo(address common.Address) (common.Utxo, error) {
	hashBytes, err := hex.DecodeString(raw.TxHash)
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

	if raw.OutputIndex < 0 {
		return common.Utxo{}, fmt.Errorf("negative output index: %d", raw.OutputIndex)
	}
	if raw.OutputIndex > math.MaxUint32 {
		return common.Utxo{}, fmt.Errorf("output index %d exceeds uint32 range", raw.OutputIndex)
	}
	input := shelley.ShelleyTransactionInput{
		TxId:        txId,
		OutputIndex: uint32(raw.OutputIndex),
	}

	var lovelace uint64
	assetData := make(map[common.Blake2b224]map[cbor.ByteString]*big.Int)

	for _, amt := range raw.Amount {
		if amt.Unit == "lovelace" {
			qty, err := strconv.ParseInt(amt.Quantity, 10, 64)
			if err != nil {
				return common.Utxo{}, fmt.Errorf("invalid lovelace quantity %q: %w", amt.Quantity, err)
			}
			if qty < 0 {
				return common.Utxo{}, fmt.Errorf("negative lovelace quantity: %d", qty)
			}
			lovelace = uint64(qty) //nolint:gosec // validated non-negative above
		} else if len(amt.Unit) >= 56 {
			qty, ok := new(big.Int).SetString(amt.Quantity, 10)
			if !ok {
				return common.Utxo{}, fmt.Errorf("invalid asset quantity %q for unit %s", amt.Quantity, amt.Unit)
			}
			if qty.Sign() < 0 {
				return common.Utxo{}, fmt.Errorf("negative asset quantity %s for unit %s", qty.String(), amt.Unit)
			}
			policyId, assetName, err := backend.ParseAssetUnit(amt.Unit)
			if err != nil {
				return common.Utxo{}, fmt.Errorf("invalid asset unit %q: %w", amt.Unit, err)
			}
			if _, ok := assetData[policyId]; !ok {
				assetData[policyId] = make(map[cbor.ByteString]*big.Int)
			}
			assetData[policyId][assetName] = qty
		} else {
			return common.Utxo{}, fmt.Errorf(
				"unrecognized unit format %q: expected \"lovelace\" or hex string >= 56 chars (policy_id + asset_name)",
				amt.Unit,
			)
		}
	}

	var assets *common.MultiAsset[common.MultiAssetTypeOutput]
	if len(assetData) > 0 {
		ma := common.NewMultiAsset[common.MultiAssetTypeOutput](assetData)
		assets = &ma
	}

	output := babbage.BabbageTransactionOutput{
		OutputAddress: address,
		OutputAmount: mary.MaryTransactionOutputValue{
			Amount: lovelace,
			Assets: assets,
		},
	}

	// Map datum hash to the output's DatumOption when no inline datum is present.
	if raw.DataHash != "" && (len(raw.InlineDatum) == 0 || string(raw.InlineDatum) == "null") {
		dhBytes, err := hex.DecodeString(raw.DataHash)
		if err != nil {
			return common.Utxo{}, fmt.Errorf("invalid data hash hex %q: %w", raw.DataHash, err)
		}
		if len(dhBytes) != common.Blake2b256Size {
			return common.Utxo{}, fmt.Errorf(
				"invalid data hash length: expected %d bytes, got %d",
				common.Blake2b256Size,
				len(dhBytes),
			)
		}
		var hash common.Blake2b256
		copy(hash[:], dhBytes)
		cborBytes, err := cbor.Encode([]any{0, hash})
		if err != nil {
			return common.Utxo{}, fmt.Errorf("failed to encode datum option hash: %w", err)
		}
		var opt babbage.BabbageTransactionOutputDatumOption
		if err := opt.UnmarshalCBOR(cborBytes); err != nil {
			return common.Utxo{}, fmt.Errorf("failed to unmarshal datum option: %w", err)
		}
		output.DatumOption = &opt
	}

	return common.Utxo{
		Id:     input,
		Output: &output,
	}, nil
}

// inlineDatumOptionFromBlockfrost builds an inline datum option from BlockFrost's
// inline_datum field, which is a CBOR-encoded datum serialized as a hex string.
// The original CBOR bytes are preserved exactly (no JSON decode/re-encode
// round-trip) so the datum hash is not altered by a non-canonical re-encoding.
func inlineDatumOptionFromBlockfrost(raw json.RawMessage) (*babbage.BabbageTransactionOutputDatumOption, error) {
	var datumCborHex string
	if err := json.Unmarshal(raw, &datumCborHex); err != nil {
		return nil, fmt.Errorf("inline datum must be a CBOR hex string: %w", err)
	}
	datumBytes, err := hex.DecodeString(datumCborHex)
	if err != nil {
		return nil, fmt.Errorf("invalid inline datum CBOR hex %q: %w", datumCborHex, err)
	}
	// Inline datum option: [1, #6.24(datum_cbor)]
	cborBytes, err := cbor.Encode([]any{1, cbor.Tag{Number: 24, Content: datumBytes}})
	if err != nil {
		return nil, fmt.Errorf("failed to encode inline datum option: %w", err)
	}
	var opt babbage.BabbageTransactionOutputDatumOption
	if err := opt.UnmarshalCBOR(cborBytes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal inline datum option: %w", err)
	}
	return &opt, nil
}

// scriptRefFromHash builds a typed gouroboros ScriptRef from a reference
// script's CBOR by detecting its language. It hashes the script as a native
// script and each Plutus version, matching against the known reference script
// hash to both determine the language and validate the bytes. Unlike the prior
// apollo v1 implementation, native scripts ARE supported and an undeterminable
// language is a hard error rather than a raw-cbor fallback.
func scriptRefFromHash(scriptHash common.Blake2b224, scriptCbor []byte) (*common.ScriptRef, error) {
	var native common.NativeScript
	if _, err := cbor.Decode(scriptCbor, &native); err == nil && native.Hash() == scriptHash {
		return &common.ScriptRef{
			Type:   common.ScriptRefTypeNativeScript,
			Script: native,
		}, nil
	}
	v1 := common.PlutusV1Script(scriptCbor)
	if v1.Hash() == scriptHash {
		return &common.ScriptRef{Type: common.ScriptRefTypePlutusV1, Script: v1}, nil
	}
	v2 := common.PlutusV2Script(scriptCbor)
	if v2.Hash() == scriptHash {
		return &common.ScriptRef{Type: common.ScriptRefTypePlutusV2, Script: v2}, nil
	}
	v3 := common.PlutusV3Script(scriptCbor)
	if v3.Hash() == scriptHash {
		return &common.ScriptRef{Type: common.ScriptRefTypePlutusV3, Script: v3}, nil
	}
	return nil, errors.New("unable to determine reference script language from script hash")
}

// bfScriptRefFromScript encodes a reference script into the Ogmios-v5 TxOut
// "script" wire shape used by /utils/txs/evaluate/utxos:
// {"plutus:v1"|"plutus:v2"|"plutus:v3"|"plutus:v4": "<base16 serialised script>"}.
// Native reference scripts are not representable in that schema and are rejected.
func bfScriptRefFromScript(script common.Script) (*bfScriptRef, error) {
	scriptHex := hex.EncodeToString(script.RawScriptBytes())
	ref := &bfScriptRef{}
	switch script.(type) {
	case common.PlutusV1Script:
		ref.PlutusV1 = &scriptHex
	case common.PlutusV2Script:
		ref.PlutusV2 = &scriptHex
	case common.PlutusV3Script:
		ref.PlutusV3 = &scriptHex
	case common.PlutusV4Script:
		ref.PlutusV4 = &scriptHex
	default:
		return nil, fmt.Errorf(
			"unsupported script type %T in additional UTxO: only Plutus v1/v2/v3/v4 reference scripts can be encoded for /utils/txs/evaluate/utxos",
			script,
		)
	}
	return ref, nil
}

// bigIntToInt64 converts a big.Int quantity to int64, rejecting values that do
// not fit rather than silently truncating.
func bigIntToInt64(v *big.Int) (int64, error) {
	if v == nil {
		return 0, nil
	}
	if !v.IsInt64() {
		return 0, fmt.Errorf("quantity %s does not fit in int64", v.String())
	}
	return v.Int64(), nil
}

// bfAdditionalUtxoItemFromUtxo builds a single [txIn, txOut] additional-UTxO
// entry from a resolved gouroboros UTxO.
func bfAdditionalUtxoItemFromUtxo(utxo common.Utxo) (bfAdditionalUtxoItem, error) {
	out := utxo.Output

	txIn := bfTxIn{
		TxId:  hex.EncodeToString(utxo.Id.Id().Bytes()),
		Index: int(utxo.Id.Index()),
	}

	coins, err := bigIntToInt64(out.Amount())
	if err != nil {
		return bfAdditionalUtxoItem{}, fmt.Errorf("invalid lovelace amount: %w", err)
	}
	// Ogmios-v6 value: lovelace under "ada", then one entry per policy id hex
	// mapping asset name hex (empty string for the empty name) to quantity.
	val := bfValue{"ada": {"lovelace": coins}}
	if assets := out.Assets(); assets != nil {
		for _, policyId := range assets.Policies() {
			policyHex := hex.EncodeToString(policyId.Bytes())
			for _, assetName := range assets.Assets(policyId) {
				qty, err := bigIntToInt64(assets.Asset(policyId, assetName))
				if err != nil {
					return bfAdditionalUtxoItem{}, fmt.Errorf(
						"invalid asset quantity for %s.%s: %w",
						policyHex, hex.EncodeToString(assetName), err,
					)
				}
				if val[policyHex] == nil {
					val[policyHex] = make(map[string]int64)
				}
				val[policyHex][hex.EncodeToString(assetName)] = qty
			}
		}
	}

	txOut := bfTxOut{
		Address: out.Address().String(),
		Value:   val,
	}

	// Inline datum CBOR hex goes in Datum; a bare datum hash goes in DatumHash.
	if datum := out.Datum(); datum != nil {
		datumCbor, err := datum.MarshalCBOR()
		if err != nil {
			return bfAdditionalUtxoItem{}, fmt.Errorf("failed to encode inline datum: %w", err)
		}
		datumHex := hex.EncodeToString(datumCbor)
		txOut.Datum = &datumHex
	} else if datumHash := out.DatumHash(); datumHash != nil {
		datumHashHex := hex.EncodeToString(datumHash.Bytes())
		txOut.DatumHash = &datumHashHex
	}

	if script := out.ScriptRef(); script != nil {
		ref, err := bfScriptRefFromScript(script)
		if err != nil {
			return bfAdditionalUtxoItem{}, err
		}
		txOut.ScriptRef = ref
	}

	return bfAdditionalUtxoItem{txIn, txOut}, nil
}

// adaptBlockfrostAccountToDelegation converts Blockfrost account details to a connector delegation.
func adaptBlockfrostAccountToDelegation(bfAcc BlockfrostAccountDetails) connector.Delegation {
	rewards := uint64(0)
	if bfAcc.WithdrawableAmount != "" {
		if parsed, err := strconv.ParseUint(bfAcc.WithdrawableAmount, 10, 64); err == nil {
			rewards = parsed
		}
	}

	poolID := ""
	if bfAcc.PoolId != nil {
		poolID = *bfAcc.PoolId
	}

	delegationActive := poolID != "" && bfAcc.Active

	return connector.Delegation{
		PoolId:  poolID,
		Rewards: rewards,
		Active:  delegationActive,
	}
}

// toProtocolParams converts the BlockFrost protocol-params response into apollo
// v2's backend.ProtocolParameters.
func (p *bfProtocolParams) toProtocolParams() (backend.ProtocolParameters, error) {
	maxBlockSize, err := backend.BoundedInt(p.MaxBlockSize, "max_block_size")
	if err != nil {
		return backend.ProtocolParameters{}, err
	}
	maxTxSize, err := backend.BoundedInt(p.MaxTxSize, "max_tx_size")
	if err != nil {
		return backend.ProtocolParameters{}, err
	}
	maxBlockHeaderSize, err := backend.BoundedInt(p.MaxBlockHeaderSize, "max_block_header_size")
	if err != nil {
		return backend.ProtocolParameters{}, err
	}
	collateralPercent, err := backend.BoundedInt(p.CollateralPercent, "collateral_percent")
	if err != nil {
		return backend.ProtocolParameters{}, err
	}
	maxCollateralInputs, err := backend.BoundedInt(p.MaxCollateralIn, "max_collateral_inputs")
	if err != nil {
		return backend.ProtocolParameters{}, err
	}

	pp := backend.ProtocolParameters{
		MinFeeConstant:                   p.MinFeeB,
		MinFeeCoefficient:                p.MinFeeA,
		MaxBlockSize:                     maxBlockSize,
		MaxTxSize:                        maxTxSize,
		MaxBlockHeaderSize:               maxBlockHeaderSize,
		KeyDeposits:                      p.KeyDeposit,
		PoolDeposits:                     p.PoolDeposit,
		PoolInfluence:                    p.A0,
		MonetaryExpansion:                p.Rho,
		TreasuryExpansion:                p.Tau,
		DecentralizationParam:            p.Decentralisation,
		ExtraEntropy:                     p.ExtraEntropy,
		ProtocolMajorVersion:             p.ProtocolMajorVer,
		ProtocolMinorVersion:             p.ProtocolMinorVer,
		MinUtxo:                          p.MinUtxo,
		MinPoolCost:                      p.MinPoolCost,
		PriceMem:                         p.PriceMem,
		PriceStep:                        p.PriceStep,
		MaxTxExMem:                       p.MaxTxExMem,
		MaxTxExSteps:                     p.MaxTxExSteps,
		MaxBlockExMem:                    p.MaxBlockExMem,
		MaxBlockExSteps:                  p.MaxBlockExSteps,
		MaxValSize:                       p.MaxValSize,
		CollateralPercent:                collateralPercent,
		MaxCollateralInputs:              maxCollateralInputs,
		CoinsPerUtxoWord:                 p.CoinsPerUtxoWord,
		CoinsPerUtxoByte:                 p.CoinsPerUtxoSize,
		MaximumReferenceScriptsSize:      p.MaximumReferenceScriptsSize,
		MinFeeReferenceScriptsRange:      p.MinFeeReferenceScriptsRange,
		MinFeeReferenceScriptsBase:       p.MinFeeReferenceScriptsBase,
		MinFeeReferenceScriptsMultiplier: p.MinFeeReferenceScriptsMultiplier,
	}

	// Cost models. The canonical ledger cost-model vector is a positional list
	// (the order is ledger-defined, NOT alphabetical), which is exactly what
	// BlockFrost's "cost_models_raw" field provides ({"PlutusV1": [n, ...]}).
	// Always prefer it. The named "cost_models" field ({"PlutusV1":
	// {"addInteger-...": n}}) is a map whose iteration/sort order does NOT match
	// the ledger order, so flattening it (e.g. by sorting parameter names) would
	// scramble the coefficients and produce wildly wrong execution budgets in
	// local script evaluation. It is used only as a best-effort fallback when it
	// is already an array, and never by sorting keys.
	costModels, err := parseCostModels(p.CostModelsRaw, p.CostModels)
	if err != nil {
		return pp, err
	}
	pp.CostModels = costModels

	return pp, nil
}

// parseCostModels resolves protocol cost models, preferring the canonical-order
// array form (BlockFrost "cost_models_raw"). A named/keyed form is rejected
// rather than guessed at, because its key order is not the ledger's canonical
// order. Scrambled or missing cost models corrupt the script-data hash, so this
// fails loud rather than silently returning no models.
func parseCostModels(raw, named json.RawMessage) (map[string][]int64, error) {
	rawPresent := len(raw) > 0 && jsonValuePresent(raw)
	namedPresent := len(named) > 0 && jsonValuePresent(named)

	// First try the named field when it happens to already be array-encoded
	// (some proxies return cost_models directly as arrays). This is the only
	// safe use of the named field.
	var namedArray map[string][]int64
	namedArrayOk := false
	if namedPresent {
		if err := json.Unmarshal(named, &namedArray); err == nil && len(namedArray) > 0 {
			namedArrayOk = true
		}
	}

	if rawPresent {
		var arrayModels map[string][]int64
		if err := json.Unmarshal(raw, &arrayModels); err == nil && len(arrayModels) > 0 {
			return arrayModels, nil
		}
		// cost_models_raw was present but could not be parsed into a non-empty
		// array map. Proceeding with no/wrong cost models would corrupt the
		// script-data hash, so fail loud unless a valid array-form cost_models
		// fallback is available.
		if namedArrayOk {
			return namedArray, nil
		}
		return nil, errors.New(
			"blockfrost cost_models_raw is present but malformed (expected a map of canonical-order int64 arrays); " +
				"refusing to proceed without valid cost models",
		)
	}

	if namedArrayOk {
		return namedArray, nil
	}
	if namedPresent {
		// The named form is a name->value map; its order is not canonical, so
		// it cannot be safely flattened. Without cost_models_raw there is no
		// reliable cost-model vector.
		return nil, errors.New(
			"blockfrost cost models are only available in named (non-canonical-order) form; " +
				"cost_models_raw is required for correct script evaluation",
		)
	}
	return nil, nil
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

// jsonValuePresent reports whether a raw JSON field was present and not null.
func jsonValuePresent(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

// parseEvaluateTxResponse parses a BlockFrost /utils/txs/evaluate response.
// BlockFrost proxies Ogmios, so the payload may be either the legacy Ogmios v5
// jsonwsp shape ({"result":{"EvaluationResult":{...}}}) or the Ogmios v6 shape
// ({"result":[{"validator":...,"budget":...}, ...]}, with failures reported as
// a top-level {"error":{...}} object).
func parseEvaluateTxResponse(data []byte) (map[common.RedeemerKey]common.ExUnits, error) {
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse evaluate response: %w", err)
	}
	if jsonValuePresent(envelope.Error) {
		var ogmiosErr struct {
			Code    int             `json:"code"`
			Message string          `json:"message"`
			Data    json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(envelope.Error, &ogmiosErr); err == nil && ogmiosErr.Message != "" {
			if jsonValuePresent(ogmiosErr.Data) {
				return nil, fmt.Errorf("%w (code %d): %s: %s",
					connector.ErrEvaluationFailed, ogmiosErr.Code, ogmiosErr.Message, string(ogmiosErr.Data))
			}
			return nil, fmt.Errorf("%w (code %d): %s", connector.ErrEvaluationFailed, ogmiosErr.Code, ogmiosErr.Message)
		}
		return nil, fmt.Errorf("%w: %s", connector.ErrEvaluationFailed, string(envelope.Error))
	}
	if !jsonValuePresent(envelope.Result) {
		return nil, fmt.Errorf("unrecognized evaluate response (no result or error): %s", evalErrorSnippet(data))
	}
	if strings.HasPrefix(strings.TrimSpace(string(envelope.Result)), "[") {
		return parseOgmiosV6EvaluationResult(envelope.Result)
	}
	return parseOgmiosV5EvaluationResult(envelope.Result)
}

// parseOgmiosV6EvaluationResult parses the Ogmios v6 evaluateTransaction result
// array: [{"validator":{"purpose":...,"index":...},"budget":{"memory":...,"cpu":...}}].
func parseOgmiosV6EvaluationResult(raw json.RawMessage) (map[common.RedeemerKey]common.ExUnits, error) {
	var items []struct {
		Validator struct {
			Purpose string `json:"purpose"`
			Index   uint64 `json:"index"`
		} `json:"validator"`
		Budget struct {
			Memory uint64 `json:"memory"`
			Cpu    uint64 `json:"cpu"`
		} `json:"budget"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("failed to parse evaluation result: %w", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: script evaluation returned no results", connector.ErrEvaluationFailed)
	}
	result := make(map[common.RedeemerKey]common.ExUnits, len(items))
	for _, item := range items {
		if jsonValuePresent(item.Error) {
			return nil, fmt.Errorf("%w for validator %s:%d: %s",
				connector.ErrEvaluationFailed, item.Validator.Purpose, item.Validator.Index, string(item.Error))
		}
		if item.Validator.Purpose == "" {
			return nil, fmt.Errorf("malformed evaluation result entry: %s", evalErrorSnippet(raw))
		}
		tag, err := parseRedeemerPurpose(item.Validator.Purpose)
		if err != nil {
			return nil, fmt.Errorf("invalid redeemer purpose %q: %w", item.Validator.Purpose, err)
		}
		if item.Validator.Index > math.MaxUint32 {
			return nil, fmt.Errorf("redeemer index %d exceeds uint32 range", item.Validator.Index)
		}
		if item.Budget.Memory > math.MaxInt64 || item.Budget.Cpu > math.MaxInt64 {
			return nil, fmt.Errorf("ExUnits overflow for validator %s:%d: memory=%d cpu=%d",
				item.Validator.Purpose, item.Validator.Index, item.Budget.Memory, item.Budget.Cpu)
		}
		key := common.RedeemerKey{Tag: tag, Index: uint32(item.Validator.Index)}
		result[key] = common.ExUnits{Memory: int64(item.Budget.Memory), Steps: int64(item.Budget.Cpu)}
	}
	return result, nil
}

// parseOgmiosV5EvaluationResult parses the legacy Ogmios v5 jsonwsp result
// object: {"EvaluationResult":{"tag:index":{"memory":...,"steps":...}}} or
// {"EvaluationFailure":{...}}.
func parseOgmiosV5EvaluationResult(raw json.RawMessage) (map[common.RedeemerKey]common.ExUnits, error) {
	var v5Result struct {
		EvaluationResult map[string]struct {
			Memory uint64 `json:"memory"`
			Steps  uint64 `json:"steps"`
		} `json:"EvaluationResult"`
		EvaluationFailure json.RawMessage `json:"EvaluationFailure"`
	}
	if err := json.Unmarshal(raw, &v5Result); err != nil {
		return nil, fmt.Errorf("failed to parse evaluation result: %w", err)
	}
	if jsonValuePresent(v5Result.EvaluationFailure) {
		return nil, fmt.Errorf("%w: %s", connector.ErrEvaluationFailed, string(v5Result.EvaluationFailure))
	}
	if v5Result.EvaluationResult == nil {
		return nil, fmt.Errorf("unrecognized evaluate response: %s", evalErrorSnippet(raw))
	}
	if len(v5Result.EvaluationResult) == 0 {
		return nil, fmt.Errorf("%w: script evaluation returned no results", connector.ErrEvaluationFailed)
	}
	result := make(map[common.RedeemerKey]common.ExUnits, len(v5Result.EvaluationResult))
	for key, budget := range v5Result.EvaluationResult {
		parts := strings.Split(key, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed redeemer key %q: expected format 'tag:index'", key)
		}
		tag, err := parseRedeemerPurpose(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid redeemer tag in key %q: %w", key, err)
		}
		idx, err := strconv.ParseUint(parts[1], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid redeemer index %q in key %q: %w", parts[1], key, err)
		}
		rKey := common.RedeemerKey{Tag: tag, Index: uint32(idx)}
		if budget.Memory > math.MaxInt64 || budget.Steps > math.MaxInt64 {
			return nil, fmt.Errorf("ExUnits overflow in key %q: memory=%d steps=%d", key, budget.Memory, budget.Steps)
		}
		result[rKey] = common.ExUnits{Memory: int64(budget.Memory), Steps: int64(budget.Steps)}
	}
	return result, nil
}

// evalErrorSnippet bounds a response payload for inclusion in error messages.
func evalErrorSnippet(data []byte) string {
	const maxSnippet = 512
	if len(data) > maxSnippet {
		data = data[:maxSnippet]
	}
	return string(data)
}
