package utxorpc

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Salvionied/apollo/serialization/Address"
	"github.com/Salvionied/apollo/serialization/PlutusData"
	"github.com/Salvionied/apollo/serialization/Redeemer"
	"github.com/Salvionied/apollo/serialization/TransactionInput"
	"github.com/Salvionied/apollo/serialization/TransactionOutput"
	"github.com/Salvionied/apollo/serialization/UTxO"
	"github.com/Salvionied/apollo/txBuilding/Backend/Base"
	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/query"
	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/submit"
	utxorpc_sdk "github.com/utxorpc/go-sdk"
	connector "github.com/zenGate-Global/cardano-connector-go"
)

type UtxorpcProvider struct {
	client    *utxorpc_sdk.UtxorpcClient
	networkId int
}

type Config struct {
	BaseUrl   string
	ApiKey    string
	NetworkId int
}

var _ connector.Provider = (*UtxorpcProvider)(nil)

func New(config Config) (*UtxorpcProvider, error) {
	client := utxorpc_sdk.NewClient(utxorpc_sdk.WithBaseUrl(config.BaseUrl))
	if config.ApiKey != "" {
		client.SetHeader("dmtr-api-key", config.ApiKey)
	}

	provider := &UtxorpcProvider{
		client:    client,
		networkId: config.NetworkId,
	}

	return provider, nil
}

func (u *UtxorpcProvider) GetProtocolParameters(
	ctx context.Context,
) (Base.ProtocolParameters, error) {
	paramsResponse, err := u.client.ReadParamsWithContext(ctx)
	if err != nil {
		return Base.ProtocolParameters{}, fmt.Errorf(
			"utxorpc: ReadParams failed: %w",
			convertGRPCError(err),
		)
	}

	if paramsResponse.Msg == nil || paramsResponse.Msg.GetValues() == nil ||
		paramsResponse.Msg.GetValues().GetCardano() == nil {
		return Base.ProtocolParameters{}, errors.New(
			"utxorpc: ReadParams returned empty Cardano parameters",
		)
	}
	cardanoParams := paramsResponse.Msg.GetValues().GetCardano()

	pp := Base.ProtocolParameters{
		MinFeeConstant:     int(cardanoParams.GetMinFeeConstant()),
		MinFeeCoefficient:  int(cardanoParams.GetMinFeeCoefficient()),
		MaxTxSize:          int(cardanoParams.GetMaxTxSize()),
		MaxBlockSize:       int(cardanoParams.GetMaxBlockBodySize()),
		MaxBlockHeaderSize: int(cardanoParams.GetMaxBlockHeaderSize()),
		KeyDeposits: strconv.FormatUint(
			cardanoParams.GetStakeKeyDeposit(),
			10,
		),
		PoolDeposits: strconv.FormatUint(
			cardanoParams.GetPoolDeposit(),
			10,
		),
		PooolInfluence: float32(
			uint32(
				cardanoParams.GetPoolInfluence().GetNumerator(),
			) / cardanoParams.GetPoolInfluence().
				GetDenominator(),
		),
		MonetaryExpansion: float32(
			uint32(
				cardanoParams.GetMonetaryExpansion().GetNumerator(),
			) / cardanoParams.GetMonetaryExpansion().
				GetDenominator(),
		),
		TreasuryExpansion: float32(
			uint32(
				cardanoParams.GetTreasuryExpansion().GetNumerator(),
			) / cardanoParams.GetTreasuryExpansion().
				GetDenominator(),
		),
		DecentralizationParam: 0,
		ExtraEntropy:          "",
		ProtocolMajorVersion: int(
			cardanoParams.GetProtocolVersion().GetMajor(),
		),
		ProtocolMinorVersion: int(
			cardanoParams.GetProtocolVersion().GetMinor(),
		),
		// MinUtxo:               fmt.Sprintf("%d", cardanoParams),
		MinPoolCost: strconv.FormatUint(cardanoParams.GetMinPoolCost(), 10),
		PriceMem: float32(
			uint32(
				cardanoParams.GetPrices().GetMemory().GetNumerator(),
			) / cardanoParams.GetPrices().
				GetMemory().
				GetDenominator(),
		),
		PriceStep: float32(
			uint32(
				cardanoParams.GetPrices().GetSteps().GetNumerator(),
			) / cardanoParams.GetPrices().
				GetSteps().
				GetDenominator(),
		),
		MaxTxExMem: strconv.FormatUint(
			cardanoParams.GetMaxExecutionUnitsPerTransaction().GetMemory(),
			10,
		),
		MaxTxExSteps: strconv.FormatUint(
			cardanoParams.GetMaxExecutionUnitsPerTransaction().GetSteps(),
			10,
		),
		MaxBlockExMem: strconv.FormatUint(
			cardanoParams.GetMaxExecutionUnitsPerBlock().GetMemory(),
			10,
		),
		MaxBlockExSteps: strconv.FormatUint(
			cardanoParams.GetMaxExecutionUnitsPerBlock().GetSteps(),
			10,
		),
		MaxValSize: strconv.FormatUint(
			cardanoParams.GetMaxValueSize(),
			10,
		),
		CollateralPercent:  int(cardanoParams.GetCollateralPercentage()),
		MaxCollateralInuts: int(cardanoParams.GetMaxCollateralInputs()),
		CoinsPerUtxoByte: strconv.FormatUint(
			cardanoParams.GetCoinsPerUtxoByte(),
			10,
		),
		CoinsPerUtxoWord: "0",
		CostModels: map[string][]int64{
			"PlutusV1": cardanoParams.GetCostModels().GetPlutusV1().GetValues(),
			"PlutusV2": cardanoParams.GetCostModels().GetPlutusV2().GetValues(),
			"PlutusV3": cardanoParams.GetCostModels().GetPlutusV3().GetValues(),
		},
	}
	return pp, nil
}

func (u *UtxorpcProvider) GetGenesisParams(
	ctx context.Context,
) (Base.GenesisParameters, error) {
	return Base.GenesisParameters{}, connector.ErrNotImplemented
}

func (u *UtxorpcProvider) Network() int {
	return u.networkId
}

func (u *UtxorpcProvider) Epoch(ctx context.Context) (int, error) {
	return 0, connector.ErrNotImplemented
}

func (u *UtxorpcProvider) GetTip(ctx context.Context) (connector.Tip, error) {
	tipResp, err := u.client.ReadTipWithContext(ctx)
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

	blockResp, err := u.client.ReadBlockWithContext(ctx, blockRef)
	if err != nil {
		return connector.Tip{}, fmt.Errorf(
			"utxorpc: failed to get block: %w",
			err,
		)
	}

	if blockResp.Msg == nil || len(blockResp.Msg.GetBlock()) == 0 ||
		blockResp.Msg.GetBlock()[0] == nil {
		return connector.Tip{}, errors.New(
			"received nil or empty block data from FetchBlockResponse for tip",
		)
	}

	block := blockResp.Msg.GetBlock()[0]

	return connector.Tip{
		Slot:   blockRef.GetIndex(),
		Height: block.GetCardano().GetHeader().GetHeight(),
		Hash:   hex.EncodeToString(blockRef.GetHash()),
	}, nil
}

func (u *UtxorpcProvider) GetUtxosByAddress(
	ctx context.Context,
	addr string,
) ([]UTxO.UTxO, error) {
	addrObj, err := Address.DecodeAddress(addr)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: failed to decode address: %s",
			connector.ErrInvalidAddress,
			err,
		)
	}
	addrBytes := addrObj.Bytes()

	ret := []UTxO.UTxO{}

	resp, err := u.client.GetUtxosByAddressWithAssetWithContext(
		ctx,
		addrBytes,
		nil,
		nil,
	)
	if err != nil {
		return ret, fmt.Errorf(
			"utxorpc: GetUtxosByAddress failed: %w",
			convertGRPCError(err),
		)
	}

	if resp.Msg == nil {
		return ret, nil
	}

	for _, item := range resp.Msg.GetItems() {
		var tmpUtxo UTxO.UTxO
		tmpUtxo.Input = TransactionInput.TransactionInput{
			TransactionId: item.GetTxoRef().GetHash(),
			Index:         int(item.GetTxoRef().GetIndex()),
		}
		tmpOutput := TransactionOutput.TransactionOutput{}
		err = tmpOutput.UnmarshalCBOR(item.GetNativeBytes())
		if err != nil {
			return ret, fmt.Errorf(
				"utxorpc: failed to unmarshal UTxO output: %w",
				err,
			)
		}
		tmpUtxo.Output = tmpOutput
		ret = append(ret, tmpUtxo)
	}

	return ret, nil
}

func (u *UtxorpcProvider) GetUtxosWithUnit(
	ctx context.Context,
	addr string,
	unit string,
) ([]UTxO.UTxO, error) {
	addrObj, err := Address.DecodeAddress(addr)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: failed to decode address: %s",
			connector.ErrInvalidAddress,
			err,
		)
	}
	addrBytes := addrObj.Bytes()

	if unit == "lovelace" {
		return nil, fmt.Errorf(
			"%w: lovelace is not a valid unit",
			connector.ErrInvalidUnit,
		)
	}

	unitBytes, err := hex.DecodeString(unit)
	if err != nil {
		return nil, err
	}

	ret := []UTxO.UTxO{}

	resp, err := u.client.GetUtxosByAddressWithAssetWithContext(ctx,
		addrBytes,
		nil,
		unitBytes,
	)
	if err != nil {
		return ret, fmt.Errorf(
			"utxorpc: GetUtxosByAddressWithAsset failed: %w",
			convertGRPCError(err),
		)
	}

	if resp.Msg == nil {
		return ret, nil
	}

	for _, item := range resp.Msg.GetItems() {
		var tmpUtxo UTxO.UTxO
		tmpUtxo.Input = TransactionInput.TransactionInput{
			TransactionId: item.GetTxoRef().GetHash(),
			Index:         int(item.GetTxoRef().GetIndex()),
		}
		tmpOutput := TransactionOutput.TransactionOutput{}
		err = tmpOutput.UnmarshalCBOR(item.GetNativeBytes())
		if err != nil {
			return ret, fmt.Errorf(
				"utxorpc: failed to unmarshal UTxO output: %w",
				err,
			)
		}
		tmpUtxo.Output = tmpOutput
		ret = append(ret, tmpUtxo)
	}

	return ret, nil
}

func (u *UtxorpcProvider) GetUtxoByUnit(
	ctx context.Context,
	unit string,
) (*UTxO.UTxO, error) {
	if unit == "lovelace" {
		return nil, fmt.Errorf(
			"%w: lovelace is not a valid unit for GetUtxoByUnit",
			connector.ErrInvalidInput,
		)
	}

	unitBytes, err := hex.DecodeString(unit)
	if err != nil {
		return nil, err
	}

	resp, err := u.client.GetUtxosByAssetWithContext(
		ctx,
		nil,
		unitBytes,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"utxorpc: GetUtxosByAsset failed: %w",
			convertGRPCError(err),
		)
	}

	if resp.Msg == nil || len(resp.Msg.GetItems()) == 0 {
		return nil, fmt.Errorf(
			"%w: no UTxO found for unit %s",
			connector.ErrNotFound,
			unit,
		)
	}

	items := resp.Msg.GetItems()
	if len(items) > 1 {
		return nil, fmt.Errorf(
			"%w: multiple UTxOs (%d) found for unit %s, expected a unique instance",
			connector.ErrMultipleUTXOs,
			len(items),
			unit,
		)
	}

	item := items[0]
	var tmpUtxo UTxO.UTxO
	tmpUtxo.Input = TransactionInput.TransactionInput{
		TransactionId: item.GetTxoRef().GetHash(),
		Index:         int(item.GetTxoRef().GetIndex()),
	}
	tmpOutput := TransactionOutput.TransactionOutput{}
	err = tmpOutput.UnmarshalCBOR(item.GetNativeBytes())
	if err != nil {
		return nil, fmt.Errorf(
			"utxorpc: failed to unmarshal UTxO output: %w",
			err,
		)
	}
	tmpUtxo.Output = tmpOutput

	return &tmpUtxo, nil
}

func (u *UtxorpcProvider) GetUtxosByOutRef(
	ctx context.Context,
	outRefs []connector.OutRef,
) ([]UTxO.UTxO, error) {
	txoRefsPtr := make([]*query.TxoRef, len(outRefs))
	for i, ref := range outRefs {
		hash, err := hex.DecodeString(ref.TxHash)
		if err != nil {
			return nil, err
		}
		txoRefsPtr[i] = &query.TxoRef{
			Hash:  hash,
			Index: ref.Index,
		}
	}

	resp, err := u.client.GetUtxosByRefsWithContext(ctx, txoRefsPtr)
	if err != nil {
		return nil, err
	}

	ret := []UTxO.UTxO{}
	var tmpUtxo UTxO.UTxO

	for _, item := range resp.Msg.GetItems() {
		tmpUtxo.Input = TransactionInput.TransactionInput{
			TransactionId: item.GetTxoRef().GetHash(),
			Index:         int(item.GetTxoRef().GetIndex()),
		}
		tmpOutput := TransactionOutput.TransactionOutput{}
		err = tmpOutput.UnmarshalCBOR(item.GetNativeBytes())
		if err != nil {
			return ret, err
		}
		tmpUtxo.Output = tmpOutput
		ret = append(ret, tmpUtxo)
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
) (PlutusData.PlutusData, error) {
	return PlutusData.PlutusData{}, connector.ErrNotImplemented
}

func (u *UtxorpcProvider) AwaitTx(
	ctx context.Context,
	txHash string,
	checkInterval time.Duration,
) (bool, error) {
	stream, err := u.client.WaitForTx(txHash)
	if err != nil {
		return false, fmt.Errorf(
			"utxorpc: WaitForTx failed: %w",
			convertGRPCError(err),
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
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (u *UtxorpcProvider) SubmitTx(
	ctx context.Context,
	tx []byte,
) (string, error) {
	txn := &submit.AnyChainTx{
		Type: &submit.AnyChainTx_Raw{
			Raw: tx,
		},
	}

	req := &submit.SubmitTxRequest{
		Tx: []*submit.AnyChainTx{txn},
	}
	resp, err := u.client.SubmitTxWithContext(ctx, req)
	if err != nil {
		return "", fmt.Errorf("utxorpc: SubmitTx failed: %w", err)
	}

	return resp.Msg.String(), nil
}

func (u *UtxorpcProvider) EvaluateTx(
	ctx context.Context,
	tx []byte,
	additionalUTxOs []UTxO.UTxO,
) (map[string]Redeemer.ExecutionUnits, error) {
	if len(additionalUTxOs) > 0 {
		return nil, errors.New(
			"utxorpc: EvaluateTx does not support additional UTxOs",
		)
	}

	txn := &submit.AnyChainTx{
		Type: &submit.AnyChainTx_Raw{
			Raw: tx,
		},
	}

	req := &submit.EvalTxRequest{
		Tx: []*submit.AnyChainTx{txn},
	}

	res, err := u.client.EvalTxWithContext(ctx, req)
	if err != nil {
		return map[string]Redeemer.ExecutionUnits{}, err
	}

	resp := make(map[string]Redeemer.ExecutionUnits)
	// Use single report since we know we have 1 Tx to eval
	redeemers := res.Msg.GetReport()[0].GetCardano().GetRedeemers()
	for _, r := range redeemers {
		purpose := r.GetPurpose().String()
		switch purpose {
		case "REDEEMER_PURPOSE_SPEND":
			purpose = "spend"
		case "REDEEMER_PURPOSE_MINT":
			purpose = "mint"
		case "REDEEMER_PURPOSE_CERT":
			purpose = "certificate"
		case "REDEEMER_PURPOSE_REWARD":
			purpose = "withdrawal"
		case "REDEEMER_PURPOSE_VOTE":
			purpose = "vote"
		case "REDEEMER_PURPOSE_PROPOSE":
			purpose = "proposal"
		default:
			return resp, errors.New("unknown purpose")
		}
		units := r.GetExUnits()
		resp[fmt.Sprintf("%s:%d", purpose, r.GetIndex())] = Redeemer.ExecutionUnits{
			Steps: int64(units.GetSteps()),
			Mem:   int64(units.GetMemory()),
		}
	}
	return resp, nil
}

func (u *UtxorpcProvider) GetScriptCborByScriptHash(
	ctx context.Context,
	scriptHash string,
) (string, error) {
	return "", connector.ErrNotImplemented
}

func convertGRPCError(err error) error {
	if err == nil {
		return nil
	}

	return err
}
