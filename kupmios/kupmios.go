package kupmios

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Salvionied/apollo/serialization/PlutusData"
	"github.com/Salvionied/apollo/serialization/UTxO"
	"github.com/Salvionied/apollo/txBuilding/Backend/Base"
	"github.com/Salvionied/cbor/v2"
	"github.com/SundaeSwap-finance/kugo"
	"github.com/SundaeSwap-finance/ogmigo/v6"
	"github.com/SundaeSwap-finance/ogmigo/v6/ouroboros/chainsync"
	"github.com/SundaeSwap-finance/ogmigo/v6/ouroboros/chainsync/num"
	"github.com/SundaeSwap-finance/ogmigo/v6/ouroboros/shared"
	connector "github.com/mgpai22/cardano-connector-go"
)

var _ connector.Provider = (*KupmiosProvider)(nil)

func New(config Config) (*KupmiosProvider, error) {
	ogmiosClient := ogmigo.New(
		ogmigo.WithEndpoint(config.OgmigoEndpoint),
	)
	kugoClient := kugo.New(
		kugo.WithEndpoint(config.KugoEndpoint),
	)

	return &KupmiosProvider{
		ogmigoClient: ogmiosClient,
		kugoClient:   kugoClient,
	}, nil
}

func (kp *KupmiosProvider) GetProtocolParameters(
	ctx context.Context,
) (Base.ProtocolParameters, error) {
	ogmigoPPJson, err := kp.ogmigoClient.CurrentProtocolParameters(ctx)
	if err != nil {
		return Base.ProtocolParameters{}, fmt.Errorf(
			"kupmios: failed to get current protocol parameters from Ogmios: %w",
			err,
		)
	}

	var ogmiosParams OgmiosProtocolParameters
	if err := json.Unmarshal(ogmigoPPJson, &ogmiosParams); err != nil {
		return Base.ProtocolParameters{}, fmt.Errorf(
			"kupmios: failed to parse Ogmios protocol parameters JSON: %w",
			err,
		)
	}

	return adaptOgmigoProtocolParamsToConnectorParams(ogmiosParams), nil
}

func (kp *KupmiosProvider) GetUtxosByAddress(
	ctx context.Context,
	addr string,
) ([]UTxO.UTxO, error) {
	matches, err := kp.kugoClient.Matches(
		ctx,
		kugo.OnlyUnspent(),
		kugo.Address(addr),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"kupmios: Kupo request for address UTxOs failed for %s: %w",
			addr,
			err,
		)
	}

	if len(matches) == 0 {
		return []UTxO.UTxO{}, nil
	}

	utxos := make([]UTxO.UTxO, 0, len(matches))
	for _, match := range matches {
		apolloUtxo, err := adaptKupoMatchToApolloUTxO(
			ctx,
			kp.kugoClient,
			match,
			addr,
		)
		if err != nil {
			fmt.Printf(
				"kupmios: warning - failed to adapt kupo match %s#%d: %v\n",
				match.TransactionID,
				match.OutputIndex,
				err,
			)
			continue
		}
		utxos = append(utxos, apolloUtxo)
	}
	return utxos, nil
}

func (kp *KupmiosProvider) GetUtxosWithUnit(
	ctx context.Context,
	address string,
	unit string,
) ([]UTxO.UTxO, error) {
	if address == "" {
		return nil, fmt.Errorf(
			"%w: address cannot be empty",
			connector.ErrInvalidInput,
		)
	}

	policyIDFromParse, assetNameFromParse, err := parseUnit(unit)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: invalid unit format '%s': %s",
			connector.ErrInvalidInput,
			unit,
			err,
		)
	}

	var targetKugoAssetIDForFilter shared.AssetID
	var mapAccessPolicyID string
	var mapAccessAssetNameHex string

	if policyIDFromParse == "lovelace" {
		targetKugoAssetIDForFilter = shared.AssetID("lovelace")
		mapAccessPolicyID = "lovelace"
		mapAccessAssetNameHex = ""
	} else {
		if assetNameFromParse == "" {
			return nil, fmt.Errorf("%w: asset name required for native asset unit '%s'", connector.ErrInvalidInput, unit)
		}
		mapAccessPolicyID = policyIDFromParse
		mapAccessAssetNameHex = assetNameFromParse

		fullAssetIDStrForKugoFilter := fmt.Sprintf("%s.%s", mapAccessPolicyID, mapAccessAssetNameHex)
		targetKugoAssetIDForFilter = shared.AssetID(fullAssetIDStrForKugoFilter)
	}

	filters := []kugo.MatchesFilter{
		kugo.OnlyUnspent(),
		kugo.Address(address),
		kugo.AssetID(targetKugoAssetIDForFilter),
	}

	matches, err := kp.kugoClient.Matches(ctx, filters...)
	if err != nil {
		return nil, fmt.Errorf(
			"kupmios: Kugo request for UTxOs at address %s with unit %s failed: %w",
			address,
			unit,
			err,
		)
	}

	if len(matches) == 0 {
		return []UTxO.UTxO{}, nil
	}

	resultUtxos := make([]UTxO.UTxO, 0, len(matches))
	for _, match := range matches {
		assetsInPolicy, policyFound := match.Value[mapAccessPolicyID]
		if !policyFound {
			continue
		}

		assetAmount, assetFound := assetsInPolicy[mapAccessAssetNameHex]
		if !assetFound {
			continue
		}

		if assetAmount.BigInt().Sign() <= 0 {
			continue
		}

		apolloUtxo, adaptErr := adaptKupoMatchToApolloUTxO(
			ctx,
			kp.kugoClient,
			match,
			match.Address,
		)
		if adaptErr != nil {
			fmt.Printf(
				"kupmios: warning - failed to adapt Kupo match %s#%d for GetUtxosWithUnit: %v\n",
				match.TransactionID,
				match.OutputIndex,
				adaptErr,
			)
			continue
		}
		resultUtxos = append(resultUtxos, apolloUtxo)
	}

	return resultUtxos, nil
}

func (kp *KupmiosProvider) GetUtxoByUnit(
	ctx context.Context,
	unit string,
) (*UTxO.UTxO, error) {
	policyIDFromParse, assetNameFromParse, err := parseUnit(unit)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: invalid unit format '%s': %s",
			connector.ErrInvalidInput,
			unit,
			err,
		)
	}

	var targetKugoAssetID shared.AssetID
	var kugoFilterPolicyIDStr string
	var kugoFilterAssetNameHexStr string

	if policyIDFromParse == "lovelace" {
		targetKugoAssetID = shared.AssetID("lovelace")
		kugoFilterPolicyIDStr = "lovelace"
		kugoFilterAssetNameHexStr = ""
	} else {
		if assetNameFromParse == "" {
			return nil, fmt.Errorf("%w: asset name cannot be empty for native asset unit '%s'", connector.ErrInvalidInput, unit)
		}
		kugoFilterPolicyIDStr = policyIDFromParse
		kugoFilterAssetNameHexStr = assetNameFromParse

		fullAssetIDStrForKugoFilter := fmt.Sprintf("%s.%s", kugoFilterPolicyIDStr, assetNameFromParse)
		targetKugoAssetID = shared.AssetID(fullAssetIDStrForKugoFilter)
	}

	kugoAssetFilter := kugo.AssetID(targetKugoAssetID)
	filters := []kugo.MatchesFilter{
		kugo.OnlyUnspent(),
		kugoAssetFilter,
	}

	matches, err := kp.kugoClient.Matches(ctx, filters...)
	if err != nil {
		return nil, fmt.Errorf(
			"kupmios: Kupo request for UTxO by unit %s failed: %w",
			unit,
			err,
		)
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf(
			"%w: no UTxO found for unit %s",
			connector.ErrNotFound,
			unit,
		)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf(
			"%w: multiple UTxOs (%d) found for unit %s, expected a unique instance",
			connector.ErrMultipleUTXOs,
			len(matches),
			unit,
		)
	}

	match := matches[0]

	var assetAmount num.Int
	var ok bool

	assetsInPolicy, policyFound := match.Value[kugoFilterPolicyIDStr]
	if !policyFound {
		return nil, fmt.Errorf(
			"kupmios: internal error - policy %s not found in matched UTxO %s#%d for unit %s",
			kugoFilterPolicyIDStr,
			match.TransactionID,
			match.OutputIndex,
			unit,
		)
	}

	assetAmount, ok = assetsInPolicy[kugoFilterAssetNameHexStr]
	if !ok {
		return nil, fmt.Errorf(
			"kupmios: internal error - asset %s (hex: %s) not found under policy %s in matched UTxO %s#%d for unit %s",
			assetNameFromParse,
			kugoFilterAssetNameHexStr,
			kugoFilterPolicyIDStr,
			match.TransactionID,
			match.OutputIndex,
			unit,
		)
	}

	_ = assetAmount

	// if assetAmount.BigInt().Cmp(big.NewInt(1)) != 0 {
	// 	// TODO: Handle when quantity is not 1??
	// }

	apolloUtxo, adaptErr := adaptKupoMatchToApolloUTxO(
		ctx,
		kp.kugoClient,
		match,
		match.Address,
	)
	if adaptErr != nil {
		return nil, fmt.Errorf(
			"kupmios: failed to adapt Kupo match for unit %s (tx: %s#%d): %w",
			unit,
			match.TransactionID,
			match.OutputIndex,
			adaptErr,
		)
	}

	return &apolloUtxo, nil
}

func (kp *KupmiosProvider) GetUtxosByOutRef(
	ctx context.Context,
	outRefs []connector.OutRef,
) ([]UTxO.UTxO, error) {
	if len(outRefs) == 0 {
		return []UTxO.UTxO{}, nil
	}

	foundUtxosMap := make(map[string]UTxO.UTxO)
	processedOutRefs := make(map[string]bool)

	for _, ref := range outRefs {
		outRefKey := fmt.Sprintf("%s#%d", ref.TxHash, ref.Index)
		if _, done := processedOutRefs[outRefKey]; done {
			continue
		}

		matches, err := kp.kugoClient.Matches(ctx,
			kugo.TxOut(chainsync.NewTxID(ref.TxHash, int(ref.Index))),
		)
		if err != nil {
			fmt.Printf(
				"kupmios: Kugo request for OutRef %s#%d failed: %v\n",
				ref.TxHash,
				ref.Index,
				err,
			)
			continue
		}

		for _, match := range matches {
			if match.TransactionID == ref.TxHash &&
				match.OutputIndex == int(ref.Index) {
				utxoKey := fmt.Sprintf(
					"%s#%d",
					match.TransactionID,
					match.OutputIndex,
				)
				if _, exists := foundUtxosMap[utxoKey]; !exists {
					apolloUtxo, adaptErr := adaptKupoMatchToApolloUTxO(
						ctx,
						kp.kugoClient,
						match,
						match.Address,
					)
					if adaptErr != nil {
						fmt.Printf(
							"kupmios: Failed to adapt Kupo match for OutRef %s#%d: %v\n",
							ref.TxHash,
							ref.Index,
							adaptErr,
						)
						continue
					}
					foundUtxosMap[utxoKey] = apolloUtxo
				}
				processedOutRefs[outRefKey] = true
				break
			}
		}
	}

	results := make([]UTxO.UTxO, 0, len(foundUtxosMap))
	for _, utxo := range foundUtxosMap {
		results = append(results, utxo)
	}

	return results, nil
}

func (kp *KupmiosProvider) GetDelegation(
	ctx context.Context,
	addrStr string,
) (connector.Delegation, error) {
	if !strings.HasPrefix(addrStr, "stake1") &&
		!strings.HasPrefix(addrStr, "stake_test1") {
		return connector.Delegation{}, fmt.Errorf(
			"%w: expected a stake address (starting with stake1 or stake_test1), got %s",
			connector.ErrInvalidAddress,
			addrStr,
		)
	}

	ogmigoDelegation, err := kp.ogmigoClient.GetDelegation(ctx, addrStr)
	if err != nil {
		return connector.Delegation{}, fmt.Errorf(
			"kupmios: Ogmigo GetDelegation failed for %s: %w",
			addrStr,
			err,
		)
	}

	delegationActive := ogmigoDelegation.PoolID != ""

	return connector.Delegation{
		PoolId:  ogmigoDelegation.PoolID,
		Rewards: ogmigoDelegation.Rewards.Uint64(),
		Active:  delegationActive,
	}, nil
}

func (kp *KupmiosProvider) GetOgmiosUtxo(
	ctx context.Context,
	txIns []chainsync.TxInQuery,
) ([]shared.Utxo, error) {
	ogmigoUTxOs, err := kp.ogmigoClient.UtxosByTxIn(ctx, txIns...)
	if err != nil {
		return nil, fmt.Errorf("kupmios: Ogmigo GetDelegation failed: %w", err)
	}

	return ogmigoUTxOs, nil
}

func (kp *KupmiosProvider) GetDatum(
	ctx context.Context,
	datumHash string,
) (PlutusData.PlutusData, error) {
	datumCBORHex, err := kp.kugoClient.Datum(ctx, string(datumHash))
	if err != nil {
		if strings.Contains(
			err.Error(),
			"not found",
		) { // Kupo might return a specific error
			return PlutusData.PlutusData{}, fmt.Errorf(
				"kupmios: datum hash %s not found via Kupo: %w",
				datumHash,
				connector.ErrNotFound,
			)
		}
		return PlutusData.PlutusData{}, fmt.Errorf(
			"kupmios: Kupo request for datum %s failed: %w",
			datumHash,
			err,
		)
	}

	if datumCBORHex == "" {
		return PlutusData.PlutusData{}, fmt.Errorf(
			"kupmios: Kupo returned empty CBOR for datum hash %s: %w",
			datumHash,
			connector.ErrNotFound,
		)
	}

	datumBytes, err := hex.DecodeString(datumCBORHex)
	if err != nil {
		return PlutusData.PlutusData{}, fmt.Errorf(
			"kupmios: invalid datum CBOR hex from Kupo for %s: %w",
			datumHash,
			err,
		)
	}
	var pd PlutusData.PlutusData
	if err := cbor.Unmarshal(datumBytes, &pd); err != nil {
		return PlutusData.PlutusData{}, fmt.Errorf(
			"kupmios: failed to unmarshal datum CBOR for %s: %w",
			datumHash,
			err,
		)
	}
	return pd, nil
}

func (kp *KupmiosProvider) AwaitTx(
	ctx context.Context,
	txHash string,
	checkInterval time.Duration,
) (bool, error) {
	if string(txHash) == "" {
		return false, fmt.Errorf(
			"%w: transaction hash cannot be empty",
			connector.ErrInvalidInput,
		)
	}

	if checkInterval <= 0 {
		checkInterval = 5 * time.Second
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false, fmt.Errorf(
				"AwaitTx for %s cancelled or timed out: %w",
				txHash,
				ctx.Err(),
			)
		case <-ticker.C:
			matches, err := kp.kugoClient.Matches(ctx,
				kugo.Transaction(string(txHash)),
			)
			if err != nil {
				continue
			}

			if len(matches) > 0 {
				if matches[0].CreatedAt.SlotNo > 0 {
					return true, nil
				}
			}
		}
	}
}

func (kp *KupmiosProvider) SubmitTx(
	ctx context.Context,
	txBytes []byte,
) (string, error) {
	submittedTxID, err := kp.ogmigoClient.SubmitTx(
		ctx,
		hex.EncodeToString(txBytes),
	)
	if err != nil || submittedTxID.Error.Code != 0 {
		return "", fmt.Errorf(
			"kupmios: Ogmigo tx submission failed: %s",
			submittedTxID.Error.Message,
		)
	}

	return submittedTxID.ID, nil
}

func (kp *KupmiosProvider) EvaluateTx(
	ctx context.Context,
	txBytes []byte,
	additionalUTxOs []UTxO.UTxO,
) ([]connector.EvalRedeemer, error) {
	if len(additionalUTxOs) > 0 {

		ogmigoUTxOs := make([]shared.Utxo, len(additionalUTxOs))
		for i, utxo := range additionalUTxOs {
			ogmigoUTxOs[i] = adaptApolloUtxoToOgmigo(utxo)
		}

		ogmigoEvalResult, err := kp.ogmigoClient.EvaluateTxWithAdditionalUtxos(
			ctx,
			hex.EncodeToString(txBytes),
			ogmigoUTxOs,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"kupmios: Ogmigo tx evaluation failed: %w. Raw Ogmigo Error: %s",
				connector.ErrEvaluationFailed,
				err.Error(),
			)
		}
		return adaptOgmigoEvalResult(ogmigoEvalResult)
	}

	ogmigoEvalResult, err := kp.ogmigoClient.EvaluateTx(
		ctx,
		hex.EncodeToString(txBytes),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"kupmios: Ogmigo tx evaluation failed: %w. Raw Ogmigo Error: %s",
			connector.ErrEvaluationFailed,
			err.Error(),
		)
	}

	return adaptOgmigoEvalResult(ogmigoEvalResult)
}

func parseUnit(
	unitStr string,
) (policyID string, assetNameHex string, err error) {
	if unitStr == "lovelace" {
		return "lovelace", "", nil
	}
	parts := strings.SplitN(unitStr, ".", 2)
	if len(parts) == 2 {
		policyID = parts[0]
		assetNameHex = parts[1]
		if len(policyID) != 56 {
			return "", "", errors.New("invalid policyId length in unit")
		}
		return policyID, assetNameHex, nil
	} else if len(parts) == 1 && len(parts[0]) == 56 {
		return parts[0], "", nil
	} else if len(parts) == 1 && len(parts[0]) >= 56 {
		policyID = parts[0][:56]
		assetNameHex = parts[0][56:]
		return policyID, assetNameHex, nil
	}
	return "", "", errors.New(
		"invalid unit format, expected 'policy.assetNameHex' or 'lovelace'",
	)
}
