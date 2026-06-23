package kupmios

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Salvionied/apollo/v2/backend"
	"github.com/SundaeSwap-finance/kugo"
	ogmigo "github.com/SundaeSwap-finance/ogmigo/v6"
	"github.com/SundaeSwap-finance/ogmigo/v6/ouroboros/chainsync"
	"github.com/SundaeSwap-finance/ogmigo/v6/ouroboros/shared"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/gorilla/websocket"
	connector "github.com/zenGate-Global/cardano-connector-go"
)

var _ connector.Provider = (*KupmiosProvider)(nil)

func New(config Config) (*KupmiosProvider, error) {
	ogmiosClient := ogmigo.New(
		ogmigo.WithEndpoint(config.OgmigoEndpoint),
	)
	kugoClient := kugo.New(
		kugo.WithEndpoint(config.KupoEndpoint),
	)

	return &KupmiosProvider{
		ogmigoClient:   ogmiosClient,
		kugoClient:     kugoClient,
		ogmiosEndpoint: config.OgmigoEndpoint,
		networkId:      config.NetworkId,
	}, nil
}

func (kp *KupmiosProvider) GetProtocolParameters(
	ctx context.Context,
) (backend.ProtocolParameters, error) {
	raw, err := kp.ogmigoClient.CurrentProtocolParameters(ctx)
	if err != nil {
		return backend.ProtocolParameters{}, fmt.Errorf(
			"kupmios: failed to get current protocol parameters from Ogmios: %w",
			err,
		)
	}

	var params ogmiosProtocolParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return backend.ProtocolParameters{}, fmt.Errorf(
			"kupmios: failed to parse Ogmios protocol parameters JSON: %w",
			err,
		)
	}

	return params.toProtocolParams()
}

func (kp *KupmiosProvider) GetGenesisParams(
	ctx context.Context,
) (backend.GenesisParameters, error) {
	raw, err := kp.ogmigoClient.GenesisConfig(ctx, "shelley")
	if err != nil {
		return backend.GenesisParameters{}, fmt.Errorf(
			"kupmios: failed to get shelley genesis parameters: %w",
			err,
		)
	}

	var genesis ogmiosGenesisConfig
	if err := json.Unmarshal(raw, &genesis); err != nil {
		return backend.GenesisParameters{}, fmt.Errorf(
			"kupmios: failed to parse shelley genesis parameters: %w",
			err,
		)
	}

	return genesis.toGenesisParams()
}

func (kp *KupmiosProvider) Network() int {
	return kp.networkId
}

func (kp *KupmiosProvider) Epoch(ctx context.Context) (int, error) {
	ogmigoEpoch, err := kp.ogmigoClient.CurrentEpoch(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get current epoch: %w", err)
	}
	if ogmigoEpoch > math.MaxInt {
		return 0, fmt.Errorf("kupmios: epoch %d exceeds int range", ogmigoEpoch)
	}

	return int(ogmigoEpoch), nil
}

func (kp *KupmiosProvider) GetTip(ctx context.Context) (connector.Tip, error) {
	point, err := kp.ogmigoClient.ChainTip(ctx)
	if err != nil {
		return connector.Tip{}, fmt.Errorf(
			"kupmios: failed to get tip: %w",
			err,
		)
	}

	ps, ok := point.PointStruct()
	if !ok || ps == nil {
		return connector.Tip{}, errors.New("kupmios: chain tip is origin")
	}

	tip := connector.Tip{
		Slot: ps.Slot,
		Hash: ps.ID,
	}
	if ps.Height != nil {
		tip.Height = *ps.Height
	} else {
		// ChainTip does not always carry the block height; query it directly.
		height, err := kp.blockHeight(ctx)
		if err != nil {
			return connector.Tip{}, fmt.Errorf(
				"kupmios: failed to get block height: %w",
				err,
			)
		}
		tip.Height = height
	}

	return tip, nil
}

func (kp *KupmiosProvider) GetUtxosByAddress(
	ctx context.Context,
	addr string,
) ([]common.Utxo, error) {
	address, err := common.NewAddress(addr)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: invalid address %q: %s",
			connector.ErrInvalidAddress,
			addr,
			err,
		)
	}

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
		return []common.Utxo{}, nil
	}

	utxos := make([]common.Utxo, 0, len(matches))
	for _, match := range matches {
		utxo, err := matchToUtxo(ctx, match, address, kp.kugoClient)
		if err != nil {
			return nil, fmt.Errorf(
				"kupmios: failed to adapt kupo match %s#%d: %w",
				match.TransactionID,
				match.OutputIndex,
				err,
			)
		}
		utxos = append(utxos, utxo)
	}
	return utxos, nil
}

func (kp *KupmiosProvider) GetUtxosWithUnit(
	ctx context.Context,
	address string,
	unit string,
) ([]common.Utxo, error) {
	if address == "" {
		return nil, fmt.Errorf(
			"%w: address cannot be empty",
			connector.ErrInvalidInput,
		)
	}

	utxos, err := kp.GetUtxosByAddress(ctx, address)
	if err != nil {
		return nil, err
	}

	matcher, err := newUnitMatcher(unit)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: invalid unit format %q: %s",
			connector.ErrInvalidInput,
			unit,
			err,
		)
	}

	result := make([]common.Utxo, 0, len(utxos))
	for _, utxo := range utxos {
		if matcher.matches(utxo) {
			result = append(result, utxo)
		}
	}

	return result, nil
}

func (kp *KupmiosProvider) GetUtxoByUnit(
	ctx context.Context,
	unit string,
) (*common.Utxo, error) {
	matcher, err := newUnitMatcher(unit)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: invalid unit format %q: %s",
			connector.ErrInvalidInput,
			unit,
			err,
		)
	}

	// Kupo can index matches by asset across all addresses.
	matches, err := kp.kugoClient.Matches(ctx,
		kugo.OnlyUnspent(),
		kugo.AssetID(shared.AssetID(matcher.kugoAssetID)),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"kupmios: Kupo request for UTxO by unit %s failed: %w",
			unit,
			err,
		)
	}

	found := make([]common.Utxo, 0, 1)
	for _, match := range matches {
		address, err := common.NewAddress(match.Address)
		if err != nil {
			return nil, fmt.Errorf(
				"kupmios: invalid address %q in match %s#%d: %w",
				match.Address,
				match.TransactionID,
				match.OutputIndex,
				err,
			)
		}
		utxo, err := matchToUtxo(ctx, match, address, kp.kugoClient)
		if err != nil {
			return nil, fmt.Errorf(
				"kupmios: failed to adapt Kupo match for unit %s (tx: %s#%d): %w",
				unit,
				match.TransactionID,
				match.OutputIndex,
				err,
			)
		}
		if matcher.matches(utxo) {
			found = append(found, utxo)
		}
	}

	if len(found) == 0 {
		return nil, fmt.Errorf(
			"%w: no UTxO found for unit %s",
			connector.ErrNotFound,
			unit,
		)
	}
	if len(found) > 1 {
		return nil, fmt.Errorf(
			"%w: multiple UTxOs (%d) found for unit %s, expected a unique instance",
			connector.ErrMultipleUTXOs,
			len(found),
			unit,
		)
	}

	return &found[0], nil
}

func (kp *KupmiosProvider) GetUtxosByOutRef(
	ctx context.Context,
	outRefs []connector.OutRef,
) ([]common.Utxo, error) {
	if len(outRefs) == 0 {
		return []common.Utxo{}, nil
	}

	results := make([]common.Utxo, 0, len(outRefs))
	seen := make(map[string]bool, len(outRefs))

	for _, ref := range outRefs {
		key := fmt.Sprintf("%s#%d", ref.TxHash, ref.Index)
		if seen[key] {
			continue
		}
		seen[key] = true

		query := chainsync.TxInQuery{
			Transaction: shared.UtxoTxID{ID: ref.TxHash},
			Index:       ref.Index,
		}
		raws, err := kp.ogmigoClient.UtxosByTxIn(ctx, query)
		if err != nil {
			return nil, fmt.Errorf(
				"kupmios: Ogmios UtxosByTxIn for %s failed: %w",
				key,
				err,
			)
		}

		for _, raw := range raws {
			if raw.Transaction.ID != ref.TxHash || raw.Index != ref.Index {
				continue
			}
			address, err := common.NewAddress(raw.Address)
			if err != nil {
				return nil, fmt.Errorf(
					"kupmios: invalid address %q for OutRef %s: %w",
					raw.Address,
					key,
					err,
				)
			}
			utxo, err := ogmiosUtxoToCommon(raw, address)
			if err != nil {
				return nil, fmt.Errorf(
					"kupmios: failed to adapt Ogmios UTxO for OutRef %s: %w",
					key,
					err,
				)
			}
			results = append(results, utxo)
		}
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

	summaries, err := kp.queryRewardAccountSummaries(ctx, addrStr)
	if err != nil {
		return connector.Delegation{}, fmt.Errorf(
			"kupmios: delegation lookup failed for %s: %w",
			addrStr,
			err,
		)
	}

	if len(summaries) == 0 {
		return connector.Delegation{}, nil
	}

	summary := summaries[0]
	delegation := connector.Delegation{}
	if summary.StakePool != nil && summary.StakePool.ID != "" {
		delegation.PoolId = summary.StakePool.ID
	} else if summary.Delegate != nil && summary.Delegate.ID != "" {
		delegation.PoolId = summary.Delegate.ID
	}
	if summary.Rewards != nil {
		delegation.Rewards = summary.Rewards.AdaLovelace().Uint64()
	}
	delegation.Active = delegation.PoolId != ""

	return delegation, nil
}

// GetOgmiosUtxo queries UTxOs directly via Ogmios by transaction input. It is
// retained for callers that need the raw ogmigo shared.Utxo wire form.
func (kp *KupmiosProvider) GetOgmiosUtxo(
	ctx context.Context,
	txIns []chainsync.TxInQuery,
) ([]shared.Utxo, error) {
	utxos, err := kp.ogmigoClient.UtxosByTxIn(ctx, txIns...)
	if err != nil {
		return nil, fmt.Errorf("kupmios: Ogmios UtxosByTxIn failed: %w", err)
	}

	return utxos, nil
}

func (kp *KupmiosProvider) GetDatum(
	ctx context.Context,
	datumHash string,
) (common.Datum, error) {
	datumCBORHex, err := kp.kugoClient.Datum(ctx, datumHash)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.Datum{}, fmt.Errorf(
				"kupmios: datum hash %s not found via Kupo: %w",
				datumHash,
				connector.ErrNotFound,
			)
		}
		return common.Datum{}, fmt.Errorf(
			"kupmios: Kupo request for datum %s failed: %w",
			datumHash,
			err,
		)
	}

	if datumCBORHex == "" {
		return common.Datum{}, fmt.Errorf(
			"kupmios: Kupo returned empty CBOR for datum hash %s: %w",
			datumHash,
			connector.ErrNotFound,
		)
	}

	datumBytes, err := hex.DecodeString(datumCBORHex)
	if err != nil {
		return common.Datum{}, fmt.Errorf(
			"kupmios: invalid datum CBOR hex from Kupo for %s: %w",
			datumHash,
			err,
		)
	}

	var datum common.Datum
	if err := datum.UnmarshalCBOR(datumBytes); err != nil {
		return common.Datum{}, fmt.Errorf(
			"kupmios: failed to decode datum CBOR for %s: %w",
			datumHash,
			err,
		)
	}
	return datum, nil
}

type ogmiosRewardAccountSummary struct {
	Delegate *struct {
		ID string `json:"id"`
	} `json:"delegate,omitempty"`
	StakePool *struct {
		ID string `json:"id"`
	} `json:"stakePool,omitempty"`
	Rewards *shared.Value `json:"rewards,omitempty"`
}

func (kp *KupmiosProvider) queryRewardAccountSummaries(
	ctx context.Context,
	addrStr string,
) ([]ogmiosRewardAccountSummary, error) {
	var response struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := kp.ogmiosRPC(
		ctx,
		"queryLedgerState/rewardAccountSummaries",
		map[string]any{"keys": []string{addrStr}},
		&response,
	); err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, fmt.Errorf(
			"ogmios delegation query failed: %s",
			response.Error.Message,
		)
	}

	var asArray []ogmiosRewardAccountSummary
	if err := json.Unmarshal(response.Result, &asArray); err == nil {
		return asArray, nil
	}

	var asMap map[string]*ogmiosRewardAccountSummary
	if err := json.Unmarshal(response.Result, &asMap); err != nil {
		return nil, fmt.Errorf(
			"failed to decode Ogmios delegation summaries: %w",
			err,
		)
	}

	summaries := make([]ogmiosRewardAccountSummary, 0, len(asMap))
	for _, summary := range asMap {
		if summary == nil {
			continue
		}
		summaries = append(summaries, *summary)
	}
	return summaries, nil
}

// blockHeight queries the current network block height over the Ogmios
// websocket. ChainTip does not always populate the height field.
func (kp *KupmiosProvider) blockHeight(ctx context.Context) (uint64, error) {
	var response struct {
		Result uint64 `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := kp.ogmiosRPC(
		ctx,
		"queryNetwork/blockHeight",
		nil,
		&response,
	); err != nil {
		return 0, err
	}
	if response.Error != nil {
		return 0, fmt.Errorf(
			"ogmios block height query failed: %s",
			response.Error.Message,
		)
	}
	return response.Result, nil
}

// ogmiosRPC issues a single JSON-RPC request over a short-lived Ogmios
// websocket connection and decodes the response into out.
func (kp *KupmiosProvider) ogmiosRPC(
	ctx context.Context,
	method string,
	params any,
	out any,
) error {
	conn, _, err := websocket.DefaultDialer.DialContext(
		ctx,
		kp.ogmiosEndpoint,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to connect to Ogmios: %w", err)
	}
	defer conn.Close()

	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"id":      nil,
	}
	if params != nil {
		payload["params"] = params
	}
	if err := conn.WriteJSON(payload); err != nil {
		return fmt.Errorf("failed to submit Ogmios %s query: %w", method, err)
	}

	if err := conn.ReadJSON(out); err != nil {
		return fmt.Errorf("failed to read Ogmios %s response: %w", method, err)
	}
	return nil
}

func (kp *KupmiosProvider) AwaitTx(
	ctx context.Context,
	txHash string,
	checkInterval time.Duration,
) (bool, error) {
	if txHash == "" {
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
				kugo.Transaction(txHash),
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
	resp, err := kp.ogmigoClient.SubmitTx(
		ctx,
		hex.EncodeToString(txBytes),
	)
	if err != nil {
		return "", fmt.Errorf("kupmios: Ogmios tx submission failed: %w", err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf(
			"kupmios: Ogmios tx submission failed: %s",
			resp.Error.Message,
		)
	}

	return resp.ID, nil
}

func (kp *KupmiosProvider) EvaluateTx(
	ctx context.Context,
	txBytes []byte,
	additionalUTxOs []common.Utxo,
) (map[common.RedeemerKey]common.ExUnits, error) {
	txHex := hex.EncodeToString(txBytes)

	var resp *ogmigo.EvaluateTxResponse
	var err error
	if len(additionalUTxOs) > 0 {
		sharedUtxos, convErr := commonUtxosToShared(additionalUTxOs)
		if convErr != nil {
			return nil, convErr
		}
		resp, err = kp.ogmigoClient.EvaluateTxWithAdditionalUtxos(
			ctx,
			txHex,
			sharedUtxos,
		)
	} else {
		resp, err = kp.ogmigoClient.EvaluateTx(ctx, txHex)
	}
	if err != nil {
		return nil, fmt.Errorf(
			"kupmios: Ogmios tx evaluation failed: %w. Raw Ogmios Error: %s",
			connector.ErrEvaluationFailed,
			err.Error(),
		)
	}

	return evaluateResponseToExUnits(resp)
}

func (kp *KupmiosProvider) GetScriptCborByScriptHash(
	ctx context.Context,
	scriptHash string,
) (string, error) {
	script, err := kp.kugoClient.Script(ctx, scriptHash)
	if err != nil {
		return "", fmt.Errorf(
			"kupmios: Kupo request for script %s failed: %w",
			scriptHash,
			err,
		)
	}
	if script == nil || script.Script == "" {
		return "", fmt.Errorf(
			"kupmios: script not found for hash %s: %w",
			scriptHash,
			connector.ErrNotFound,
		)
	}

	return script.Script, nil
}

// unitMatcher filters common.Utxo values by an asset unit. The unit is either
// "lovelace" or a concatenation of the 56-hex policy ID and the asset name hex.
type unitMatcher struct {
	lovelace    bool
	policyId    common.Blake2b224
	assetName   []byte
	kugoAssetID string
}

func newUnitMatcher(unit string) (unitMatcher, error) {
	if unit == "lovelace" {
		return unitMatcher{lovelace: true, kugoAssetID: "lovelace"}, nil
	}

	policyId, assetName, err := backend.ParseAssetUnit(unit)
	if err != nil {
		return unitMatcher{}, err
	}

	nameHex := hex.EncodeToString(assetName.Bytes())
	kugoAssetID := hex.EncodeToString(policyId.Bytes())
	if nameHex != "" {
		kugoAssetID = kugoAssetID + "." + nameHex
	}

	return unitMatcher{
		policyId:    policyId,
		assetName:   assetName.Bytes(),
		kugoAssetID: kugoAssetID,
	}, nil
}

func (m unitMatcher) matches(utxo common.Utxo) bool {
	out := utxo.Output
	if m.lovelace {
		return out.Amount().Sign() > 0
	}

	assets := out.Assets()
	if assets == nil {
		return false
	}
	qty := assets.Asset(m.policyId, m.assetName)
	return qty != nil && qty.Sign() > 0
}
