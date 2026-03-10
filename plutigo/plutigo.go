package plutigo

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/Salvionied/apollo/serialization/PlutusData"
	"github.com/Salvionied/apollo/serialization/Redeemer"
	"github.com/Salvionied/apollo/serialization/UTxO"
	"github.com/Salvionied/apollo/txBuilding/Backend/Base"
	cbor "github.com/Salvionied/cbor/v2"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	lscript "github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/cek"
	pdata "github.com/blinklabs-io/plutigo/data"
	connector "github.com/zenGate-Global/cardano-connector-go"
)

type Config struct {
	Provider               connector.Provider
	Resolver               connector.Provider
	ProtocolParamsOverride *Base.ProtocolParameters
	GenesisParamsOverride  *Base.GenesisParameters
	SlotConfig             *SlotConfig
}

type SlotConfig struct {
	ZeroTime   time.Time
	ZeroSlot   uint64
	SlotLength time.Duration
}

type PlutigoProvider struct {
	resolver               connector.Provider
	protocolParamsOverride *Base.ProtocolParameters
	genesisParamsOverride  *Base.GenesisParameters
	slotConfig             *SlotConfig
}

var _ connector.Provider = (*PlutigoProvider)(nil)

func New(config Config) (*PlutigoProvider, error) {
	if config.SlotConfig != nil && config.SlotConfig.SlotLength <= 0 {
		return nil, errors.New("plutigo: invalid slot config: slot length must be positive")
	}
	resolver := config.Provider
	if resolver == nil {
		resolver = config.Resolver
	}
	return &PlutigoProvider{
		resolver:               resolver,
		protocolParamsOverride: config.ProtocolParamsOverride,
		genesisParamsOverride:  config.GenesisParamsOverride,
		slotConfig:             config.SlotConfig,
	}, nil
}

func Wrap(provider connector.Provider) (*PlutigoProvider, error) {
	return New(Config{
		Provider: provider,
	})
}

func (p *PlutigoProvider) GetProtocolParameters(ctx context.Context) (Base.ProtocolParameters, error) {
	if p.protocolParamsOverride != nil {
		return *p.protocolParamsOverride, nil
	}
	if p.resolver != nil {
		return p.resolver.GetProtocolParameters(ctx)
	}
	return Base.ProtocolParameters{}, notImplementedError("GetProtocolParameters")
}

func (p *PlutigoProvider) GetGenesisParams(ctx context.Context) (Base.GenesisParameters, error) {
	if p.genesisParamsOverride != nil {
		return *p.genesisParamsOverride, nil
	}
	if p.resolver != nil {
		return p.resolver.GetGenesisParams(ctx)
	}
	return Base.GenesisParameters{}, notImplementedError("GetGenesisParams")
}

func (p *PlutigoProvider) Network() int {
	if p.resolver != nil {
		return p.resolver.Network()
	}
	return 0
}

func (p *PlutigoProvider) Epoch(ctx context.Context) (int, error) {
	if p.resolver != nil {
		return p.resolver.Epoch(ctx)
	}
	return 0, notImplementedError("Epoch")
}

func (p *PlutigoProvider) GetTip(ctx context.Context) (connector.Tip, error) {
	if p.resolver != nil {
		return p.resolver.GetTip(ctx)
	}
	return connector.Tip{}, notImplementedError("GetTip")
}

func (p *PlutigoProvider) GetUtxosByAddress(ctx context.Context, addr string) ([]UTxO.UTxO, error) {
	if p.resolver != nil {
		return p.resolver.GetUtxosByAddress(ctx, addr)
	}
	return nil, notImplementedError("GetUtxosByAddress")
}

func (p *PlutigoProvider) GetUtxosWithUnit(ctx context.Context, addr string, unit string) ([]UTxO.UTxO, error) {
	if p.resolver != nil {
		return p.resolver.GetUtxosWithUnit(ctx, addr, unit)
	}
	return nil, notImplementedError("GetUtxosWithUnit")
}

func (p *PlutigoProvider) GetUtxoByUnit(ctx context.Context, unit string) (*UTxO.UTxO, error) {
	if p.resolver != nil {
		return p.resolver.GetUtxoByUnit(ctx, unit)
	}
	return nil, notImplementedError("GetUtxoByUnit")
}

func (p *PlutigoProvider) GetUtxosByOutRef(ctx context.Context, outRefs []connector.OutRef) ([]UTxO.UTxO, error) {
	if p.resolver != nil {
		return p.resolver.GetUtxosByOutRef(ctx, outRefs)
	}
	return nil, notImplementedError("GetUtxosByOutRef")
}

func (p *PlutigoProvider) GetDelegation(ctx context.Context, rewardAddress string) (connector.Delegation, error) {
	if p.resolver != nil {
		return p.resolver.GetDelegation(ctx, rewardAddress)
	}
	return connector.Delegation{}, notImplementedError("GetDelegation")
}

func (p *PlutigoProvider) GetDatum(ctx context.Context, datumHash string) (PlutusData.PlutusData, error) {
	if p.resolver != nil {
		return p.resolver.GetDatum(ctx, datumHash)
	}
	return PlutusData.PlutusData{}, notImplementedError("GetDatum")
}

func (p *PlutigoProvider) AwaitTx(ctx context.Context, txHash string, checkInterval time.Duration) (bool, error) {
	if p.resolver != nil {
		return p.resolver.AwaitTx(ctx, txHash, checkInterval)
	}
	return false, notImplementedError("AwaitTx")
}

func (p *PlutigoProvider) SubmitTx(ctx context.Context, tx []byte) (string, error) {
	if p.resolver != nil {
		return p.resolver.SubmitTx(ctx, tx)
	}
	return "", notImplementedError("SubmitTx")
}

func (p *PlutigoProvider) GetScriptCborByScriptHash(ctx context.Context, scriptHash string) (string, error) {
	if p.resolver != nil {
		return p.resolver.GetScriptCborByScriptHash(ctx, scriptHash)
	}
	return "", notImplementedError("GetScriptCborByScriptHash")
}

func (p *PlutigoProvider) EvaluateTx(
	ctx context.Context,
	tx []byte,
	additionalUTxOs []UTxO.UTxO,
) (map[string]Redeemer.ExecutionUnits, error) {
	txType, err := ledger.DetermineTransactionType(tx)
	if err != nil {
		return nil, classifiedError(connector.ErrInvalidInput, "determine transaction type", err)
	}
	if txType != ledger.TxTypeAlonzo && txType != ledger.TxTypeBabbage && txType != ledger.TxTypeConway {
		return nil, classifiedError(
			connector.ErrNotImplemented,
			fmt.Sprintf("transaction era %d is not supported for local evaluation", txType),
			nil,
		)
	}

	decodedTx, err := ledger.NewTransactionFromCbor(txType, tx)
	if err != nil {
		return nil, classifiedError(connector.ErrInvalidInput, "decode transaction", err)
	}

	witnesses := decodedTx.Witnesses()
	if witnesses == nil || witnesses.Redeemers() == nil {
		return map[string]Redeemer.ExecutionUnits{}, nil
	}

	resolvedInputsByKey, resolvedInputs, err := p.resolveInputs(ctx, decodedTx, additionalUTxOs)
	if err != nil {
		return nil, err
	}

	witnessDatums, err := p.resolveWitnessDatums(ctx, witnesses.PlutusData(), resolvedInputsByKey)
	if err != nil {
		return nil, err
	}

	protocolParams, maxMem, maxSteps, err := p.resolveProtocolParameters(ctx)
	if err != nil {
		return nil, err
	}

	slotState, err := p.resolveSlotState(ctx)
	if err != nil {
		return nil, err
	}

	mint := lcommon.MultiAsset[lcommon.MultiAssetTypeMint]{}
	if decodedTx.AssetMint() != nil {
		mint = *decodedTx.AssetMint()
	}

	results := make(map[string]Redeemer.ExecutionUnits)
	for _, redeemerTag := range supportedRedeemerTags() {
		for _, redeemerIndex := range witnesses.Redeemers().Indexes(redeemerTag) {
			redeemerKey := lcommon.RedeemerKey{
				Tag:   redeemerTag,
				Index: uint32(redeemerIndex),
			}
			redeemerValue := witnesses.Redeemers().Value(redeemerIndex, redeemerTag)

			purpose, err := buildScriptPurpose(
				redeemerKey,
				resolvedInputsByKey,
				decodedTx.Inputs(),
				mint,
				decodedTx.Certificates(),
				decodedTx.Withdrawals(),
				decodedTx.VotingProcedures(),
				decodedTx.ProposalProcedures(),
				witnessDatums,
			)
			if err != nil {
				return nil, err
			}

			resolvedScript, err := p.resolveScript(ctx, witnesses, resolvedInputs, purpose.ScriptHash().String())
			if err != nil {
				return nil, err
			}

			budget := lcommon.ExUnits{
				Memory: maxMem,
				Steps:  maxSteps,
			}
			evalContext, err := buildEvalContext(protocolParams, resolvedScript.version)
			if err != nil {
				return nil, err
			}

			var usedBudget lcommon.ExUnits
			switch resolvedScript.version {
			case scriptVersionV1:
				txInfo, err := lscript.NewTxInfoV1FromTransaction(slotState, decodedTx, resolvedInputs)
				if err != nil {
					return nil, classifiedError(connector.ErrEvaluationFailed, "build V1 transaction info", err)
				}
				scriptContext := lscript.NewScriptContextV1V2(txInfo, purpose)
				var datum pdata.PlutusData
				if spendingPurpose, ok := purpose.(lscript.ScriptPurposeSpending); ok {
					datum = spendingPurpose.Datum
					if datum == nil {
						return nil, classifiedError(
							connector.ErrNotFound,
							"missing datum for spending redeemer "+resultKey(redeemerKey),
							nil,
						)
					}
				}
				usedBudget, err = resolvedScript.v1.Evaluate(
					datum,
					redeemerValue.Data.Data,
					scriptContext.ToPlutusData(),
					budget,
					evalContext,
				)
				if err != nil {
					return nil, classifiedError(connector.ErrEvaluationFailed, "evaluate Plutus V1 script", err)
				}
			case scriptVersionV2:
				txInfo, err := lscript.NewTxInfoV2FromTransaction(slotState, decodedTx, resolvedInputs)
				if err != nil {
					return nil, classifiedError(connector.ErrEvaluationFailed, "build V2 transaction info", err)
				}
				scriptContext := lscript.NewScriptContextV1V2(txInfo, purpose)
				var datum pdata.PlutusData
				if spendingPurpose, ok := purpose.(lscript.ScriptPurposeSpending); ok {
					datum = spendingPurpose.Datum
					if datum == nil {
						return nil, classifiedError(
							connector.ErrNotFound,
							"missing datum for spending redeemer "+resultKey(redeemerKey),
							nil,
						)
					}
				}
				usedBudget, err = resolvedScript.v2.Evaluate(
					datum,
					redeemerValue.Data.Data,
					scriptContext.ToPlutusData(),
					budget,
					evalContext,
				)
				if err != nil {
					return nil, classifiedError(connector.ErrEvaluationFailed, "evaluate Plutus V2 script", err)
				}
			case scriptVersionV3:
				txInfo, err := lscript.NewTxInfoV3FromTransaction(slotState, decodedTx, resolvedInputs)
				if err != nil {
					return nil, classifiedError(connector.ErrEvaluationFailed, "build V3 transaction info", err)
				}
				scriptContext := lscript.NewScriptContextV3(
					txInfo,
					lscript.Redeemer{
						Tag:     redeemerKey.Tag,
						Index:   redeemerKey.Index,
						Data:    redeemerValue.Data.Data,
						ExUnits: redeemerValue.ExUnits,
					},
					purpose,
				)
				usedBudget, err = resolvedScript.v3.Evaluate(
					scriptContext.ToPlutusData(),
					budget,
					evalContext,
				)
				if err != nil {
					return nil, classifiedError(connector.ErrEvaluationFailed, "evaluate Plutus V3 script", err)
				}
			default:
				return nil, classifiedError(connector.ErrNotImplemented, "unsupported Plutus script version", nil)
			}

			results[resultKey(redeemerKey)] = Redeemer.ExecutionUnits{
				Mem:   usedBudget.Memory,
				Steps: usedBudget.Steps,
			}
		}
	}

	return results, nil
}

type localSlotState struct {
	zeroTime   time.Time
	zeroSlot   uint64
	slotLength time.Duration
}

func (s localSlotState) SlotToTime(slot uint64) (time.Time, error) {
	if slot < s.zeroSlot {
		return time.Time{}, fmt.Errorf("slot %d precedes zero slot %d", slot, s.zeroSlot)
	}
	return s.zeroTime.Add(time.Duration(slot-s.zeroSlot) * s.slotLength), nil
}

func (s localSlotState) TimeToSlot(t time.Time) (uint64, error) {
	if t.Before(s.zeroTime) {
		return 0, fmt.Errorf("time %s precedes zero time %s", t.UTC().Format(time.RFC3339), s.zeroTime.UTC().Format(time.RFC3339))
	}
	return s.zeroSlot + uint64(t.Sub(s.zeroTime)/s.slotLength), nil
}

type scriptVersion int

const (
	scriptVersionUnknown scriptVersion = iota
	scriptVersionV1
	scriptVersionV2
	scriptVersionV3
)

type resolvedScript struct {
	version scriptVersion
	v1      lcommon.PlutusV1Script
	v2      lcommon.PlutusV2Script
	v3      lcommon.PlutusV3Script
}

func (p *PlutigoProvider) resolveInputs(
	ctx context.Context,
	tx lcommon.Transaction,
	additionalUTxOs []UTxO.UTxO,
) (map[string]lcommon.Utxo, []lcommon.Utxo, error) {
	resolvedInputs := make(map[string]lcommon.Utxo, len(additionalUTxOs))
	for _, utxo := range additionalUTxOs {
		converted, err := apolloUtxoToLedger(utxo)
		if err != nil {
			return nil, nil, classifiedError(connector.ErrInvalidInput, "decode additional UTxO", err)
		}
		resolvedInputs[converted.Id.String()] = converted
	}

	neededRefs := make([]connector.OutRef, 0)
	seenNeeded := make(map[string]struct{})
	for _, input := range appendTransactionInputs(tx.Inputs(), tx.ReferenceInputs()) {
		key := input.String()
		if _, ok := resolvedInputs[key]; ok {
			continue
		}
		if _, ok := seenNeeded[key]; ok {
			continue
		}
		seenNeeded[key] = struct{}{}
		neededRefs = append(neededRefs, connector.OutRef{
			TxHash: input.Id().String(),
			Index:  input.Index(),
		})
	}

	if len(neededRefs) > 0 {
		if p.resolver == nil {
			return nil, nil, classifiedError(
				connector.ErrNotFound,
				"missing resolver for transaction inputs not present in additionalUTxOs",
				nil,
			)
		}
		fetchedUtxos, err := p.resolver.GetUtxosByOutRef(ctx, neededRefs)
		if err != nil {
			return nil, nil, classifiedError(connector.ErrNotFound, "resolve transaction inputs", err)
		}
		for _, utxo := range fetchedUtxos {
			converted, err := apolloUtxoToLedger(utxo)
			if err != nil {
				return nil, nil, classifiedError(connector.ErrInvalidInput, "decode resolver UTxO", err)
			}
			if _, exists := resolvedInputs[converted.Id.String()]; !exists {
				resolvedInputs[converted.Id.String()] = converted
			}
		}
	}

	orderedResolved := make([]lcommon.Utxo, 0, len(tx.Inputs())+len(tx.ReferenceInputs()))
	seenResolved := make(map[string]struct{})
	for _, input := range appendTransactionInputs(tx.Inputs(), tx.ReferenceInputs()) {
		resolved, ok := resolvedInputs[input.String()]
		if !ok {
			return nil, nil, classifiedError(
				connector.ErrNotFound,
				"missing input "+input.String(),
				nil,
			)
		}
		if _, ok := seenResolved[input.String()]; ok {
			continue
		}
		seenResolved[input.String()] = struct{}{}
		orderedResolved = append(orderedResolved, resolved)
	}

	return resolvedInputs, orderedResolved, nil
}

func (p *PlutigoProvider) resolveWitnessDatums(
	ctx context.Context,
	witnessDatumList []lcommon.Datum,
	resolvedInputs map[string]lcommon.Utxo,
) (map[lcommon.Blake2b256]*lcommon.Datum, error) {
	witnessDatums := make(map[lcommon.Blake2b256]*lcommon.Datum, len(witnessDatumList))
	for i := range witnessDatumList {
		datumCopy := witnessDatumList[i]
		witnessDatums[datumCopy.Hash()] = &datumCopy
	}

	if p.resolver == nil {
		return witnessDatums, nil
	}

	for _, resolvedInput := range resolvedInputs {
		if resolvedInput.Output == nil || resolvedInput.Output.Datum() != nil {
			continue
		}
		datumHash := resolvedInput.Output.DatumHash()
		if datumHash == nil {
			continue
		}
		if _, ok := witnessDatums[*datumHash]; ok {
			continue
		}
		apolloDatum, err := p.resolver.GetDatum(ctx, datumHash.String())
		if err != nil {
			if errors.Is(err, connector.ErrNotFound) {
				continue
			}
			return nil, classifiedError(connector.ErrNotFound, "resolve datum "+datumHash.String(), err)
		}
		ledgerDatum, err := apolloDatumToLedger(apolloDatum)
		if err != nil {
			return nil, classifiedError(connector.ErrInvalidInput, "decode datum "+datumHash.String(), err)
		}
		ledgerDatumCopy := ledgerDatum
		witnessDatums[*datumHash] = &ledgerDatumCopy
	}

	return witnessDatums, nil
}

func (p *PlutigoProvider) resolveProtocolParameters(
	ctx context.Context,
) (Base.ProtocolParameters, int64, int64, error) {
	var params Base.ProtocolParameters
	if p.protocolParamsOverride != nil {
		params = *p.protocolParamsOverride
	} else {
		if p.resolver == nil {
			return Base.ProtocolParameters{}, 0, 0, classifiedError(
				connector.ErrNotImplemented,
				"protocol parameters are required but no resolver or override was provided",
				nil,
			)
		}
		resolvedParams, err := p.resolver.GetProtocolParameters(ctx)
		if err != nil {
			return Base.ProtocolParameters{}, 0, 0, classifiedError(connector.ErrNotFound, "resolve protocol parameters", err)
		}
		params = resolvedParams
	}

	if params.ProtocolMajorVersion <= 0 {
		return Base.ProtocolParameters{}, 0, 0, classifiedError(
			connector.ErrNotImplemented,
			"protocol major version is required for local evaluation",
			nil,
		)
	}

	maxMem, err := parseInt64Param("max_tx_ex_mem", params.MaxTxExMem)
	if err != nil {
		return Base.ProtocolParameters{}, 0, 0, err
	}
	maxSteps, err := parseInt64Param("max_tx_ex_steps", params.MaxTxExSteps)
	if err != nil {
		return Base.ProtocolParameters{}, 0, 0, err
	}

	return params, maxMem, maxSteps, nil
}

func (p *PlutigoProvider) resolveSlotState(ctx context.Context) (localSlotState, error) {
	if p.slotConfig != nil {
		return localSlotState{
			zeroTime:   p.slotConfig.ZeroTime.UTC(),
			zeroSlot:   p.slotConfig.ZeroSlot,
			slotLength: p.slotConfig.SlotLength,
		}, nil
	}

	var genesis Base.GenesisParameters
	if p.genesisParamsOverride != nil {
		genesis = *p.genesisParamsOverride
	} else {
		if p.resolver == nil {
			return localSlotState{}, classifiedError(
				connector.ErrNotImplemented,
				"slot state is required but no resolver, genesis override, or slot config was provided",
				nil,
			)
		}
		resolvedGenesis, err := p.resolver.GetGenesisParams(ctx)
		if err != nil {
			return localSlotState{}, classifiedError(connector.ErrNotFound, "resolve genesis parameters", err)
		}
		genesis = resolvedGenesis
	}

	if genesis.SlotLength <= 0 {
		return localSlotState{}, classifiedError(
			connector.ErrNotImplemented,
			"slot length must be positive to build slot state",
			nil,
		)
	}

	return localSlotState{
		zeroTime:   time.Unix(int64(genesis.SystemStart), 0).UTC(),
		zeroSlot:   0,
		slotLength: time.Duration(genesis.SlotLength) * time.Second,
	}, nil
}

func (p *PlutigoProvider) resolveScript(
	ctx context.Context,
	witnesses lcommon.TransactionWitnessSet,
	resolvedInputs []lcommon.Utxo,
	scriptHash string,
) (resolvedScript, error) {
	if script, ok := findScriptInWitnesses(witnesses, scriptHash); ok {
		return script, nil
	}
	if script, ok := findScriptInResolvedInputs(resolvedInputs, scriptHash); ok {
		return script, nil
	}
	if p.resolver == nil {
		return resolvedScript{}, classifiedError(connector.ErrNotFound, "script "+scriptHash, nil)
	}

	scriptCborHex, err := p.resolver.GetScriptCborByScriptHash(ctx, scriptHash)
	if err != nil {
		return resolvedScript{}, classifiedError(connector.ErrNotFound, "resolve script "+scriptHash, err)
	}

	script, err := decodeResolvedScript(scriptCborHex, scriptHash)
	if err != nil {
		return resolvedScript{}, err
	}
	return script, nil
}

func apolloUtxoToLedger(utxo UTxO.UTxO) (lcommon.Utxo, error) {
	outputCbor, err := cbor.Marshal(utxo.Output)
	if err != nil {
		return lcommon.Utxo{}, fmt.Errorf("marshal transaction output: %w", err)
	}
	decodedOutput, err := ledger.NewTransactionOutputFromCbor(outputCbor)
	if err != nil {
		return lcommon.Utxo{}, fmt.Errorf("decode transaction output: %w", err)
	}
	input := shelley.NewShelleyTransactionInput(hex.EncodeToString(utxo.Input.TransactionId), utxo.Input.Index)
	return lcommon.Utxo{
		Id:     input,
		Output: decodedOutput,
	}, nil
}

func apolloDatumToLedger(datum PlutusData.PlutusData) (lcommon.Datum, error) {
	cborBytes, err := datum.MarshalCBOR()
	if err != nil {
		return lcommon.Datum{}, fmt.Errorf("marshal datum: %w", err)
	}
	var wrapper pdata.PlutusDataWrapper
	if err := wrapper.UnmarshalCBOR(cborBytes); err != nil {
		return lcommon.Datum{}, fmt.Errorf("decode datum: %w", err)
	}
	return lcommon.Datum{Data: wrapper.Data}, nil
}

func buildScriptPurpose(
	redeemerKey lcommon.RedeemerKey,
	resolvedInputs map[string]lcommon.Utxo,
	inputs []lcommon.TransactionInput,
	mint lcommon.MultiAsset[lcommon.MultiAssetTypeMint],
	certificates []lcommon.Certificate,
	withdrawals map[*lcommon.Address]*big.Int,
	votes lcommon.VotingProcedures,
	proposalProcedures []lcommon.ProposalProcedure,
	witnessDatums map[lcommon.Blake2b256]*lcommon.Datum,
) (purpose lscript.ScriptPurpose, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = classifiedError(connector.ErrInvalidInput, fmt.Sprintf("build script purpose: %v", r), nil)
		}
	}()
	purpose = lscript.BuildScriptPurpose(
		redeemerKey,
		resolvedInputs,
		inputs,
		mint,
		certificates,
		withdrawals,
		votes,
		proposalProcedures,
		witnessDatums,
	)
	return purpose, nil
}

func buildEvalContext(
	params Base.ProtocolParameters,
	version scriptVersion,
) (*cek.EvalContext, error) {
	costModel, err := costModelForVersion(params.CostModels, version)
	if err != nil {
		return nil, err
	}
	evalContext, err := cek.NewEvalContext(
		cekLanguageVersion(version),
		cek.ProtoVersion{
			Major: uint(params.ProtocolMajorVersion),
			Minor: uint(params.ProtocolMinorVersion),
		},
		costModel,
	)
	if err != nil {
		return nil, classifiedError(connector.ErrEvaluationFailed, "build evaluation context", err)
	}
	return evalContext, nil
}

func findScriptInWitnesses(witnesses lcommon.TransactionWitnessSet, scriptHash string) (resolvedScript, bool) {
	for _, script := range witnesses.PlutusV1Scripts() {
		if script.Hash().String() == scriptHash {
			return resolvedScript{version: scriptVersionV1, v1: script}, true
		}
	}
	for _, script := range witnesses.PlutusV2Scripts() {
		if script.Hash().String() == scriptHash {
			return resolvedScript{version: scriptVersionV2, v2: script}, true
		}
	}
	for _, script := range witnesses.PlutusV3Scripts() {
		if script.Hash().String() == scriptHash {
			return resolvedScript{version: scriptVersionV3, v3: script}, true
		}
	}
	return resolvedScript{}, false
}

func findScriptInResolvedInputs(resolvedInputs []lcommon.Utxo, scriptHash string) (resolvedScript, bool) {
	for _, resolvedInput := range resolvedInputs {
		if resolvedInput.Output == nil {
			continue
		}
		script := resolvedInput.Output.ScriptRef()
		if script == nil {
			continue
		}
		if resolved, ok := resolveConcreteScript(script, scriptHash); ok {
			return resolved, true
		}
	}
	return resolvedScript{}, false
}

func decodeResolvedScript(scriptCborHex string, scriptHash string) (resolvedScript, error) {
	cleaned := strings.TrimSpace(scriptCborHex)
	cleaned = strings.TrimPrefix(cleaned, "0x")
	cleaned = strings.TrimPrefix(cleaned, "0X")
	raw, err := hex.DecodeString(cleaned)
	if err != nil {
		return resolvedScript{}, classifiedError(connector.ErrInvalidInput, fmt.Sprintf("decode script %s hex", scriptHash), err)
	}

	candidates := [][]byte{raw}
	var inner []byte
	if err := cbor.Unmarshal(raw, &inner); err == nil && len(inner) > 0 {
		candidates = append(candidates, inner)
	}

	for _, candidate := range candidates {
		if resolved, ok := decodeScriptCandidate(candidate, scriptHash); ok {
			return resolved, nil
		}
	}

	return resolvedScript{}, classifiedError(
		connector.ErrNotFound,
		fmt.Sprintf("script bytes for %s did not match a supported Plutus version", scriptHash),
		nil,
	)
}

func decodeScriptCandidate(candidate []byte, scriptHash string) (resolvedScript, bool) {
	v1 := lcommon.PlutusV1Script(candidate)
	if v1.Hash().String() == scriptHash {
		return resolvedScript{version: scriptVersionV1, v1: v1}, true
	}
	v2 := lcommon.PlutusV2Script(candidate)
	if v2.Hash().String() == scriptHash {
		return resolvedScript{version: scriptVersionV2, v2: v2}, true
	}
	v3 := lcommon.PlutusV3Script(candidate)
	if v3.Hash().String() == scriptHash {
		return resolvedScript{version: scriptVersionV3, v3: v3}, true
	}
	var scriptRef lcommon.ScriptRef
	if err := scriptRef.UnmarshalCBOR(candidate); err == nil {
		return resolveConcreteScript(scriptRef.Script, scriptHash)
	}
	return resolvedScript{}, false
}

func resolveConcreteScript(script lcommon.Script, scriptHash string) (resolvedScript, bool) {
	switch tmp := script.(type) {
	case lcommon.PlutusV1Script:
		if tmp.Hash().String() == scriptHash {
			return resolvedScript{version: scriptVersionV1, v1: tmp}, true
		}
	case lcommon.PlutusV2Script:
		if tmp.Hash().String() == scriptHash {
			return resolvedScript{version: scriptVersionV2, v2: tmp}, true
		}
	case lcommon.PlutusV3Script:
		if tmp.Hash().String() == scriptHash {
			return resolvedScript{version: scriptVersionV3, v3: tmp}, true
		}
	case *lcommon.PlutusV1Script:
		if tmp != nil && tmp.Hash().String() == scriptHash {
			return resolvedScript{version: scriptVersionV1, v1: *tmp}, true
		}
	case *lcommon.PlutusV2Script:
		if tmp != nil && tmp.Hash().String() == scriptHash {
			return resolvedScript{version: scriptVersionV2, v2: *tmp}, true
		}
	case *lcommon.PlutusV3Script:
		if tmp != nil && tmp.Hash().String() == scriptHash {
			return resolvedScript{version: scriptVersionV3, v3: *tmp}, true
		}
	}
	return resolvedScript{}, false
}

func costModelForVersion(costModels map[string][]int64, version scriptVersion) ([]int64, error) {
	if len(costModels) == 0 {
		return nil, classifiedError(connector.ErrNotImplemented, "protocol parameters are missing cost models", nil)
	}

	normalized := make(map[string][]int64, len(costModels))
	for key, model := range costModels {
		normalized[normalizeCostModelKey(key)] = model
	}

	var lookupKeys []string
	switch version {
	case scriptVersionV1:
		lookupKeys = []string{"plutusv1", "plutusscriptv1"}
	case scriptVersionV2:
		lookupKeys = []string{"plutusv2", "plutusscriptv2"}
	case scriptVersionV3:
		lookupKeys = []string{"plutusv3", "plutusscriptv3"}
	default:
		return nil, classifiedError(connector.ErrNotImplemented, "unknown Plutus version", nil)
	}

	for _, key := range lookupKeys {
		if model, ok := normalized[key]; ok && len(model) > 0 {
			return model, nil
		}
	}

	return nil, classifiedError(
		connector.ErrNotImplemented,
		"missing cost model for "+resultKeyFromVersion(version),
		nil,
	)
}

func cekLanguageVersion(version scriptVersion) cek.LanguageVersion {
	switch version {
	case scriptVersionV1:
		return cek.LanguageVersionV1
	case scriptVersionV2:
		return cek.LanguageVersionV2
	case scriptVersionV3:
		return cek.LanguageVersionV3
	default:
		return cek.LanguageVersionV1
	}
}

func normalizeCostModelKey(key string) string {
	var b strings.Builder
	b.Grow(len(key))
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		if r >= 'A' && r <= 'Z' {
			b.WriteRune(r + ('a' - 'A'))
		}
	}
	return b.String()
}

func parseInt64Param(name string, value string) (int64, error) {
	if strings.TrimSpace(value) == "" {
		return 0, classifiedError(connector.ErrNotImplemented, fmt.Sprintf("protocol parameter %s is required", name), nil)
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, classifiedError(connector.ErrInvalidInput, "parse protocol parameter "+name, err)
	}
	return parsed, nil
}

func resultKey(redeemerKey lcommon.RedeemerKey) string {
	var prefix string
	switch redeemerKey.Tag {
	case lcommon.RedeemerTagSpend:
		prefix = "spend"
	case lcommon.RedeemerTagMint:
		prefix = "mint"
	case lcommon.RedeemerTagCert:
		prefix = "certificate"
	case lcommon.RedeemerTagReward:
		prefix = "withdrawal"
	case lcommon.RedeemerTagVoting:
		prefix = "vote"
	case lcommon.RedeemerTagProposing:
		prefix = "proposal"
	default:
		prefix = "unknown"
	}
	return fmt.Sprintf("%s:%d", prefix, redeemerKey.Index)
}

func resultKeyFromVersion(version scriptVersion) string {
	switch version {
	case scriptVersionV1:
		return "PlutusV1"
	case scriptVersionV2:
		return "PlutusV2"
	case scriptVersionV3:
		return "PlutusV3"
	default:
		return "unknown"
	}
}

func appendTransactionInputs(a []lcommon.TransactionInput, b ...[]lcommon.TransactionInput) []lcommon.TransactionInput {
	total := len(a)
	for _, extra := range b {
		total += len(extra)
	}
	combined := make([]lcommon.TransactionInput, 0, total)
	combined = append(combined, a...)
	for _, extra := range b {
		combined = append(combined, extra...)
	}
	return combined
}

func supportedRedeemerTags() []lcommon.RedeemerTag {
	return []lcommon.RedeemerTag{
		lcommon.RedeemerTagSpend,
		lcommon.RedeemerTagMint,
		lcommon.RedeemerTagCert,
		lcommon.RedeemerTagReward,
		lcommon.RedeemerTagVoting,
		lcommon.RedeemerTagProposing,
	}
}

func notImplementedError(method string) error {
	return classifiedError(connector.ErrNotImplemented, method+" is not supported by the plutigo provider", nil)
}

func classifiedError(kind error, message string, err error) error {
	if err != nil {
		return fmt.Errorf("plutigo: %s: %w", message, errors.Join(kind, err))
	}
	return fmt.Errorf("plutigo: %s: %w", message, kind)
}
