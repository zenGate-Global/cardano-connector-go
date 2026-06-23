package utxorpc

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"time"

	"connectrpc.com/connect"
	"github.com/Salvionied/apollo/v2/backend"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/query"
	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/submit"
	syncpb "github.com/utxorpc/go-codegen/utxorpc/v1alpha/sync"
	sdk "github.com/utxorpc/go-sdk"
	connector "github.com/zenGate-Global/cardano-connector-go"
)

type UtxorpcProvider struct {
	client    *sdk.UtxorpcClient
	networkId int
}

type Config struct {
	BaseUrl   string
	ApiKey    string
	NetworkId int
}

var _ connector.Provider = (*UtxorpcProvider)(nil)

func New(config Config) (*UtxorpcProvider, error) {
	opts := []sdk.ClientOption{
		sdk.WithBaseUrl(config.BaseUrl),
	}
	if config.ApiKey != "" {
		opts = append(opts, sdk.WithHeaders(map[string]string{
			"dmtr-api-key": config.ApiKey,
		}))
	}
	client := sdk.NewClient(opts...)

	provider := &UtxorpcProvider{
		client:    client,
		networkId: config.NetworkId,
	}

	return provider, nil
}

func (u *UtxorpcProvider) GetProtocolParameters(
	ctx context.Context,
) (backend.ProtocolParameters, error) {
	req := connect.NewRequest(&query.ReadParamsRequest{})
	resp, err := u.client.ReadParamsWithContext(ctx, req)
	if err != nil {
		return backend.ProtocolParameters{}, fmt.Errorf(
			"utxorpc: ReadParams failed: %w",
			err,
		)
	}

	if resp.Msg == nil || resp.Msg.GetValues() == nil ||
		resp.Msg.GetValues().GetCardano() == nil {
		return backend.ProtocolParameters{}, errors.New(
			"utxorpc: ReadParams returned empty Cardano parameters",
		)
	}
	params := resp.Msg.GetValues().GetCardano()

	maxBlockSize, err := backend.BoundedIntFromUint64(params.GetMaxBlockBodySize(), "max block body size")
	if err != nil {
		return backend.ProtocolParameters{}, err
	}
	maxTxSize, err := backend.BoundedIntFromUint64(params.GetMaxTxSize(), "max tx size")
	if err != nil {
		return backend.ProtocolParameters{}, err
	}
	maxBlockHeaderSize, err := backend.BoundedIntFromUint64(params.GetMaxBlockHeaderSize(), "max block header size")
	if err != nil {
		return backend.ProtocolParameters{}, err
	}
	collateralPercent, err := backend.BoundedIntFromUint64(params.GetCollateralPercentage(), "collateral percentage")
	if err != nil {
		return backend.ProtocolParameters{}, err
	}
	maxCollateralInputs, err := backend.BoundedIntFromUint64(params.GetMaxCollateralInputs(), "max collateral inputs")
	if err != nil {
		return backend.ProtocolParameters{}, err
	}

	pp := backend.ProtocolParameters{
		MinFeeConstant:     bigIntToInt64(params.GetMinFeeConstant()),
		MinFeeCoefficient:  bigIntToInt64(params.GetMinFeeCoefficient()),
		MaxTxSize:          maxTxSize,
		MaxBlockSize:       maxBlockSize,
		MaxBlockHeaderSize: maxBlockHeaderSize,
		KeyDeposits:        bigIntToString(params.GetStakeKeyDeposit()),
		PoolDeposits:       bigIntToString(params.GetPoolDeposit()),
		PoolInfluence:      rationalToFloat64(params.GetPoolInfluence()),
		MonetaryExpansion:  rationalToFloat64(params.GetMonetaryExpansion()),
		TreasuryExpansion:  rationalToFloat64(params.GetTreasuryExpansion()),
		ProtocolMajorVersion: int(
			params.GetProtocolVersion().GetMajor(),
		),
		ProtocolMinorVersion: int(
			params.GetProtocolVersion().GetMinor(),
		),
		MinPoolCost:         bigIntToString(params.GetMinPoolCost()),
		MaxValSize:          strconv.FormatUint(params.GetMaxValueSize(), 10),
		CollateralPercent:   collateralPercent,
		MaxCollateralInputs: maxCollateralInputs,
		CoinsPerUtxoByte:    bigIntToString(params.GetCoinsPerUtxoByte()),
	}

	if prices := params.GetPrices(); prices != nil {
		pp.PriceMem = rationalToFloat64(prices.GetMemory())
		pp.PriceStep = rationalToFloat64(prices.GetSteps())
	}
	if txEx := params.GetMaxExecutionUnitsPerTransaction(); txEx != nil {
		pp.MaxTxExMem = strconv.FormatUint(txEx.GetMemory(), 10)
		pp.MaxTxExSteps = strconv.FormatUint(txEx.GetSteps(), 10)
	}
	if blockEx := params.GetMaxExecutionUnitsPerBlock(); blockEx != nil {
		pp.MaxBlockExMem = strconv.FormatUint(blockEx.GetMemory(), 10)
		pp.MaxBlockExSteps = strconv.FormatUint(blockEx.GetSteps(), 10)
	}

	// Parse cost models from UTxO RPC protobuf response.
	// Keys match ComputeScriptDataHash expectations: "PlutusV1", "PlutusV2", "PlutusV3".
	if cm := params.GetCostModels(); cm != nil {
		pp.CostModels = make(map[string][]int64)
		if v1 := cm.GetPlutusV1(); v1 != nil {
			pp.CostModels["PlutusV1"] = append([]int64(nil), v1.GetValues()...)
		}
		if v2 := cm.GetPlutusV2(); v2 != nil {
			pp.CostModels["PlutusV2"] = append([]int64(nil), v2.GetValues()...)
		}
		if v3 := cm.GetPlutusV3(); v3 != nil {
			pp.CostModels["PlutusV3"] = append([]int64(nil), v3.GetValues()...)
		}
	}

	return mergeProtocolParamsWithPreset(pp, protocolParamsPreset), nil
}

func (u *UtxorpcProvider) GetGenesisParams(
	ctx context.Context,
) (backend.GenesisParameters, error) {
	return backend.GenesisParameters{}, connector.ErrNotImplemented
}

func (u *UtxorpcProvider) Network() int {
	return u.networkId
}

func (u *UtxorpcProvider) Epoch(ctx context.Context) (int, error) {
	return 0, connector.ErrNotImplemented
}

func (u *UtxorpcProvider) GetTip(ctx context.Context) (connector.Tip, error) {
	tipReq := connect.NewRequest(&syncpb.ReadTipRequest{})
	tipResp, err := u.client.ReadTipWithContext(ctx, tipReq)
	if err != nil {
		return connector.Tip{}, fmt.Errorf(
			"utxorpc: failed to get tip: %w",
			err,
		)
	}

	if tipResp.Msg == nil || tipResp.Msg.GetTip() == nil {
		return connector.Tip{}, errors.New(
			"received nil tip from ReadTipResponse",
		)
	}
	blockRef := tipResp.Msg.GetTip()

	height := blockRef.GetHeight()
	if height == 0 {
		// Older gateways do not populate height on the tip BlockRef; fetch the
		// block to recover the height.
		blockReq := connect.NewRequest(&syncpb.FetchBlockRequest{
			Ref: []*syncpb.BlockRef{blockRef},
		})
		blockResp, blockErr := u.client.FetchBlockWithContext(ctx, blockReq)
		if blockErr != nil {
			return connector.Tip{}, fmt.Errorf(
				"utxorpc: failed to get block: %w",
				blockErr,
			)
		}
		if blockResp.Msg == nil || len(blockResp.Msg.GetBlock()) == 0 ||
			blockResp.Msg.GetBlock()[0] == nil {
			return connector.Tip{}, errors.New(
				"received nil or empty block data from FetchBlockResponse for tip",
			)
		}
		height = blockResp.Msg.GetBlock()[0].GetCardano().GetHeader().GetHeight()
	}

	return connector.Tip{
		Slot:   blockRef.GetSlot(),
		Height: height,
		Hash:   hex.EncodeToString(blockRef.GetHash()),
	}, nil
}

func (u *UtxorpcProvider) GetUtxosByAddress(
	ctx context.Context,
	addr string,
) ([]common.Utxo, error) {
	addrObj, err := common.NewAddress(addr)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: failed to decode address: %s",
			connector.ErrInvalidAddress,
			err,
		)
	}
	addrBytes, err := addrObj.Bytes()
	if err != nil {
		return nil, fmt.Errorf("utxorpc: failed to get address bytes: %w", err)
	}

	return u.searchUtxos(ctx, &cardano.TxOutputPattern{
		Address: &cardano.AddressPattern{
			ExactAddress: addrBytes,
		},
	})
}

func (u *UtxorpcProvider) GetUtxosWithUnit(
	ctx context.Context,
	addr string,
	unit string,
) ([]common.Utxo, error) {
	addrObj, err := common.NewAddress(addr)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: failed to decode address: %s",
			connector.ErrInvalidAddress,
			err,
		)
	}
	addrBytes, err := addrObj.Bytes()
	if err != nil {
		return nil, fmt.Errorf("utxorpc: failed to get address bytes: %w", err)
	}

	if unit == "lovelace" {
		return nil, fmt.Errorf(
			"%w: lovelace is not a valid unit",
			connector.ErrInvalidUnit,
		)
	}

	assetPattern, err := unitToAssetPattern(unit)
	if err != nil {
		return nil, err
	}

	return u.searchUtxos(ctx, &cardano.TxOutputPattern{
		Address: &cardano.AddressPattern{
			ExactAddress: addrBytes,
		},
		Asset: assetPattern,
	})
}

func (u *UtxorpcProvider) GetUtxoByUnit(
	ctx context.Context,
	unit string,
) (*common.Utxo, error) {
	if unit == "lovelace" {
		return nil, fmt.Errorf(
			"%w: lovelace is not a valid unit for GetUtxoByUnit",
			connector.ErrInvalidInput,
		)
	}

	assetPattern, err := unitToAssetPattern(unit)
	if err != nil {
		return nil, err
	}

	utxos, err := u.searchUtxos(ctx, &cardano.TxOutputPattern{
		Asset: assetPattern,
	})
	if err != nil {
		return nil, err
	}

	if len(utxos) == 0 {
		return nil, fmt.Errorf(
			"%w: no UTxO found for unit %s",
			connector.ErrNotFound,
			unit,
		)
	}
	if len(utxos) > 1 {
		return nil, fmt.Errorf(
			"%w: multiple UTxOs (%d) found for unit %s, expected a unique instance",
			connector.ErrMultipleUTXOs,
			len(utxos),
			unit,
		)
	}

	return &utxos[0], nil
}

func (u *UtxorpcProvider) GetUtxosByOutRef(
	ctx context.Context,
	outRefs []connector.OutRef,
) ([]common.Utxo, error) {
	keys := make([]*query.TxoRef, len(outRefs))
	for i, ref := range outRefs {
		hash, err := hex.DecodeString(ref.TxHash)
		if err != nil {
			return nil, err
		}
		keys[i] = &query.TxoRef{
			Hash:  hash,
			Index: ref.Index,
		}
	}

	req := connect.NewRequest(&query.ReadUtxosRequest{Keys: keys})
	resp, err := u.client.ReadUtxosWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	ret := []common.Utxo{}
	if resp.Msg == nil {
		return ret, nil
	}
	for _, item := range resp.Msg.GetItems() {
		utxo, err := utxoFromRpc(item)
		if err != nil {
			return ret, err
		}
		ret = append(ret, utxo)
	}
	return ret, nil
}

func (u *UtxorpcProvider) GetDelegation(
	ctx context.Context,
	rewardAddress string,
) (connector.Delegation, error) {
	return connector.Delegation{}, connector.ErrNotImplemented
}

func (u *UtxorpcProvider) GetDatum(
	ctx context.Context,
	datumHash string,
) (common.Datum, error) {
	return common.Datum{}, connector.ErrNotImplemented
}

func (u *UtxorpcProvider) AwaitTx(
	ctx context.Context,
	txHash string,
	checkInterval time.Duration,
) (bool, error) {
	hashBytes, err := hex.DecodeString(txHash)
	if err != nil {
		return false, fmt.Errorf(
			"%w: failed to decode tx hash: %s",
			connector.ErrInvalidInput,
			err,
		)
	}

	req := connect.NewRequest(&submit.WaitForTxRequest{
		Ref: [][]byte{hashBytes},
	})
	stream, err := u.client.WaitForTxWithContext(ctx, req)
	if err != nil {
		return false, fmt.Errorf(
			"utxorpc: WaitForTx failed: %w",
			err,
		)
	}
	defer stream.Close()

	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
			if stream.Receive() {
				return true, nil
			}
			if err := stream.Err(); err != nil {
				return false, fmt.Errorf("utxorpc: WaitForTx stream failed: %w", err)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (u *UtxorpcProvider) SubmitTx(
	ctx context.Context,
	tx []byte,
) (string, error) {
	req := connect.NewRequest(&submit.SubmitTxRequest{
		Tx: &submit.AnyChainTx{
			Type: &submit.AnyChainTx_Raw{Raw: tx},
		},
	})
	resp, err := u.client.SubmitTxWithContext(ctx, req)
	if err != nil {
		return "", fmt.Errorf("utxorpc: SubmitTx failed: %w", err)
	}

	ref := resp.Msg.GetRef()
	if len(ref) == 0 {
		return "", errors.New("utxorpc: no tx ref in submit response")
	}
	return hex.EncodeToString(ref), nil
}

// EvaluateTx evaluates the scripts in a transaction. The additionalUTxOs
// argument is IGNORED: the utxorpc EvalTx schema (submit.EvalTxRequest) carries
// only the raw transaction CBOR and has no field for additional/resolved
// UTxOs. This backend can therefore only evaluate transactions whose inputs
// are already visible on-chain to the provider and does NOT support evaluating
// off-chain or chained inputs.
func (u *UtxorpcProvider) EvaluateTx(
	ctx context.Context,
	tx []byte,
	additionalUTxOs []common.Utxo,
) (map[common.RedeemerKey]common.ExUnits, error) {
	// Per the Provider contract, additionalUTxOs are ignored (not an error) by
	// backends that cannot forward them. The utxorpc EvalTx schema carries only
	// the raw tx CBOR and has no field for resolved UTxOs, so off-chain/chained
	// inputs cannot be supplied here.
	_ = additionalUTxOs

	req := connect.NewRequest(&submit.EvalTxRequest{
		Tx: &submit.AnyChainTx{
			Type: &submit.AnyChainTx_Raw{Raw: tx},
		},
	})
	resp, err := u.client.EvalTxWithContext(ctx, req)
	if err != nil {
		return nil, err
	}
	return evalTxResponseToExUnits(resp.Msg)
}

func (u *UtxorpcProvider) GetScriptCborByScriptHash(
	ctx context.Context,
	scriptHash string,
) (string, error) {
	return "", connector.ErrNotImplemented
}

// searchUtxos runs a SearchUtxos query for the given Cardano output pattern and
// parses the matched items into gouroboros common.Utxo values.
func (u *UtxorpcProvider) searchUtxos(
	ctx context.Context,
	pattern *cardano.TxOutputPattern,
) ([]common.Utxo, error) {
	req := connect.NewRequest(&query.SearchUtxosRequest{
		Predicate: &query.UtxoPredicate{
			Match: &query.AnyUtxoPattern{
				UtxoPattern: &query.AnyUtxoPattern_Cardano{
					Cardano: pattern,
				},
			},
		},
	})
	resp, err := u.client.SearchUtxosWithContext(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("utxorpc: SearchUtxos failed: %w", err)
	}

	ret := []common.Utxo{}
	if resp.Msg == nil {
		return ret, nil
	}
	for _, item := range resp.Msg.GetItems() {
		utxo, err := utxoFromRpc(item)
		if err != nil {
			return ret, fmt.Errorf("utxorpc: failed to parse UTxO from RPC: %w", err)
		}
		ret = append(ret, utxo)
	}
	return ret, nil
}

// unitToAssetPattern converts an asset unit (policyId hex + asset name hex) into
// a UTxO RPC AssetPattern.
func unitToAssetPattern(unit string) (*cardano.AssetPattern, error) {
	policyId, assetName, err := backend.ParseAssetUnit(unit)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", connector.ErrInvalidUnit, err)
	}
	return &cardano.AssetPattern{
		PolicyId:  policyId.Bytes(),
		AssetName: assetName.Bytes(),
	}, nil
}

// utxoFromRpc parses a UTxO RPC item into a gouroboros common.Utxo using the
// native CBOR bytes for the output and the TxoRef for the input.
func utxoFromRpc(item *query.AnyUtxoData) (common.Utxo, error) {
	nativeBytes := item.GetNativeBytes()
	if len(nativeBytes) == 0 {
		ref := item.GetTxoRef()
		return common.Utxo{}, fmt.Errorf("no native bytes for utxo %s#%d",
			hex.EncodeToString(ref.GetHash()), ref.GetIndex())
	}

	// Decode the output era-generically so pre-Babbage (Shelley/Mary/Alonzo)
	// UTxOs hydrate, not just Babbage map-encoded outputs.
	output, err := ledger.NewTransactionOutputFromCbor(nativeBytes)
	if err != nil {
		return common.Utxo{}, fmt.Errorf("failed to parse utxo CBOR: %w", err)
	}

	ref := item.GetTxoRef()
	refHash := ref.GetHash()
	if len(refHash) != common.Blake2b256Size {
		return common.Utxo{}, fmt.Errorf("invalid tx hash length: expected %d bytes, got %d", common.Blake2b256Size, len(refHash))
	}
	var txId common.Blake2b256
	copy(txId[:], refHash)

	input := shelley.ShelleyTransactionInput{
		TxId:        txId,
		OutputIndex: ref.GetIndex(),
	}
	return common.Utxo{
		Id:     input,
		Output: output,
	}, nil
}

// evalTxResponseToExUnits converts an EvalTxResponse into a redeemer ExUnits
// map. A missing report, missing cardano report, or zero evaluation results
// is an error: returning an empty map with a nil error would let callers
// silently keep zero execution budgets for their redeemers.
func evalTxResponseToExUnits(msg *submit.EvalTxResponse) (map[common.RedeemerKey]common.ExUnits, error) {
	if msg == nil {
		return nil, errors.New("utxorpc: empty evaluate response")
	}
	report := msg.GetReport()
	if report == nil {
		return nil, errors.New("utxorpc: no evaluation report in response")
	}
	cardanoReport := report.GetCardano()
	if cardanoReport == nil {
		return nil, errors.New("utxorpc: no cardano evaluation report in response")
	}
	result := make(map[common.RedeemerKey]common.ExUnits)
	for _, redeemer := range cardanoReport.GetRedeemers() {
		tag, err := utxorpcPurposeToRedeemerTag(redeemer.GetPurpose())
		if err != nil {
			return nil, fmt.Errorf("utxorpc: failed to map redeemer purpose: %w", err)
		}
		key := common.RedeemerKey{
			Tag:   tag,
			Index: redeemer.GetIndex(),
		}
		eu := redeemer.GetExUnits()
		if eu == nil {
			return nil, fmt.Errorf("utxorpc: no ExUnits in evaluation report for redeemer %d:%d", tag, redeemer.GetIndex())
		}
		mem := eu.GetMemory()
		steps := eu.GetSteps()
		if mem > math.MaxInt64 || steps > math.MaxInt64 {
			return nil, fmt.Errorf("utxorpc: ExUnits overflow: memory=%d steps=%d", mem, steps)
		}
		result[key] = common.ExUnits{
			Memory: int64(mem),
			Steps:  int64(steps),
		}
	}
	if len(result) == 0 {
		return nil, errors.New("utxorpc: script evaluation returned no results")
	}
	return result, nil
}

// utxorpcPurposeToRedeemerTag maps UTxO RPC redeemer purpose enum to gouroboros RedeemerTag.
// UTxO RPC uses 1-based enum (SPEND=1, MINT=2, ...) while gouroboros uses 0-based (Spend=0, Mint=1, ...).
func utxorpcPurposeToRedeemerTag(purpose cardano.RedeemerPurpose) (common.RedeemerTag, error) {
	switch purpose {
	case cardano.RedeemerPurpose_REDEEMER_PURPOSE_SPEND:
		return common.RedeemerTagSpend, nil
	case cardano.RedeemerPurpose_REDEEMER_PURPOSE_MINT:
		return common.RedeemerTagMint, nil
	case cardano.RedeemerPurpose_REDEEMER_PURPOSE_CERT:
		return common.RedeemerTagCert, nil
	case cardano.RedeemerPurpose_REDEEMER_PURPOSE_REWARD:
		return common.RedeemerTagReward, nil
	default:
		return 0, fmt.Errorf("unsupported redeemer purpose: %d", purpose)
	}
}

func bigIntToInt64(bi *cardano.BigInt) int64 {
	if bi == nil {
		return 0
	}
	oneof := bi.GetBigInt()
	if oneof == nil {
		return 0
	}
	switch v := oneof.(type) {
	case *cardano.BigInt_Int:
		if v == nil {
			return 0
		}
		return v.Int
	case *cardano.BigInt_BigUInt:
		if v == nil {
			return 0
		}
		n := new(big.Int).SetBytes(v.BigUInt)
		if !n.IsInt64() {
			return math.MaxInt64
		}
		return n.Int64()
	case *cardano.BigInt_BigNInt:
		if v == nil {
			return 0
		}
		n := new(big.Int).SetBytes(v.BigNInt)
		n.Neg(n)
		if !n.IsInt64() {
			return math.MinInt64
		}
		return n.Int64()
	default:
		return 0
	}
}

func bigIntToString(bi *cardano.BigInt) string {
	if bi == nil {
		return "0"
	}
	oneof := bi.GetBigInt()
	if oneof == nil {
		return "0"
	}
	switch v := oneof.(type) {
	case *cardano.BigInt_Int:
		if v == nil {
			return "0"
		}
		return strconv.FormatInt(v.Int, 10)
	case *cardano.BigInt_BigUInt:
		if v == nil {
			return "0"
		}
		return new(big.Int).SetBytes(v.BigUInt).String()
	case *cardano.BigInt_BigNInt:
		if v == nil {
			return "0"
		}
		n := new(big.Int).SetBytes(v.BigNInt)
		n.Neg(n)
		return n.String()
	default:
		return "0"
	}
}

func rationalToFloat64(v *cardano.RationalNumber) float64 {
	if v == nil || v.GetDenominator() == 0 {
		return 0
	}
	return float64(v.GetNumerator()) / float64(v.GetDenominator())
}
