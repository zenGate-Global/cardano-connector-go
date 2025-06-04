package blockfrost

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Salvionied/apollo/serialization/PlutusData"
	"github.com/Salvionied/apollo/serialization/UTxO"
	"github.com/Salvionied/apollo/txBuilding/Backend/Base"
	"github.com/Salvionied/cbor/v2"
	connector "github.com/zenGate-Global/cardano-connector-go"
)

const (
	defaultMainnetBaseURL = "https://cardano-mainnet.blockfrost.io/api/v0"
	defaultPreprodBaseURL = "https://cardano-preprod.blockfrost.io/api/v0"
	defaultPreviewBaseURL = "https://cardano-preview.blockfrost.io/api/v0"
)

var _ connector.Provider = (*BlockfrostProvider)(nil)

func New(config Config) (*BlockfrostProvider, error) {
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		switch strings.ToLower(config.NetworkName) {
		case "mainnet":
			baseURL = defaultMainnetBaseURL
		case "preprod":
			baseURL = defaultPreprodBaseURL
		case "preview":
			baseURL = defaultPreviewBaseURL
		default:
			return nil, fmt.Errorf(
				"unsupported or missing network name: %s, and no BaseURL provided",
				config.NetworkName,
			)
		}
	} else {
		// If a custom baseURL is provided, ensure it doesn't end with /v0 if it's a blockfrost URL
		// or add /v0 if it's a blockfrost domain without it. This logic might need refinement.
		if strings.Contains(baseURL, "blockfrost.io") && !strings.HasSuffix(baseURL, "/v0") {
			baseURL += "/v0"
		}
	}

	provider := &BlockfrostProvider{
		httpClient:                httpClient,
		baseURL:                   baseURL,
		projectID:                 config.ProjectID,
		networkName:               config.NetworkName,
		customSubmissionEndpoints: config.CustomSubmissionEndpoints,
	}
	return provider, nil
}

// GetProtocolParameters fetches the current protocol parameters from Blockfrost.
func (b *BlockfrostProvider) GetProtocolParameters(
	ctx context.Context,
) (Base.ProtocolParameters, error) {
	var bfParams BlockfrostProtocolParameters
	path := "/epochs/latest/parameters"

	err := b.doRequest(ctx, "GET", path, nil, &bfParams)
	if err != nil {
		return Base.ProtocolParameters{}, fmt.Errorf(
			"failed to get protocol parameters: %w",
			err,
		)
	}

	return bfParams.ToBaseParams(), nil
}

func (b *BlockfrostProvider) doRequest(
	ctx context.Context,
	method, path string,
	body io.Reader,
	target interface{},
) error {
	fullURL := b.baseURL + path // Assumes path starts with "/"
	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return fmt.Errorf("blockfrost: failed to create request: %w", err)
	}

	if b.projectID != "" {
		req.Header.Set("project_id", b.projectID)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/cbor")
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("blockfrost: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("blockfrost: failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Attempt to parse Blockfrost error structure
		var bfError struct {
			StatusCode int    `json:"status_code"`
			Err        string `json:"error"`
			Message    string `json:"message"`
		}
		if json.Unmarshal(respBodyBytes, &bfError) == nil &&
			bfError.Message != "" {
			if bfError.StatusCode == http.StatusNotFound {
				return fmt.Errorf(
					"blockfrost API error (%d - %s): %s: %w",
					resp.StatusCode,
					http.StatusText(resp.StatusCode),
					bfError.Message,
					connector.ErrNotFound,
				)
			}
			return fmt.Errorf(
				"blockfrost API error (%d - %s): %s",
				resp.StatusCode,
				http.StatusText(resp.StatusCode),
				bfError.Message,
			)
		}
		// Fallback generic error
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf(
				"blockfrost API error: status %d - %s. Body: %s: %w",
				resp.StatusCode,
				http.StatusText(resp.StatusCode),
				string(respBodyBytes),
				connector.ErrNotFound,
			)
		}
		return fmt.Errorf(
			"blockfrost API error: status %d - %s. Body: %s",
			resp.StatusCode,
			http.StatusText(resp.StatusCode),
			string(respBodyBytes),
		)
	}

	if target != nil {
		if err := json.Unmarshal(respBodyBytes, target); err != nil {
			if s, ok := target.(*string); ok &&
				(method == "POST" && (strings.HasSuffix(path, "/tx/submit"))) {
				*s = strings.Trim(string(respBodyBytes), "\"")
				return nil
			}
			return fmt.Errorf(
				"blockfrost: failed to decode JSON response: %w. Body: %s",
				err,
				string(respBodyBytes),
			)
		}
	}
	return nil
}

func (b *BlockfrostProvider) GetUtxosByAddress(
	ctx context.Context,
	addr string,
) ([]UTxO.UTxO, error) {
	var allBfUtxos []BlockfrostUTXO
	page := 1

	for {
		var bfUtxos []BlockfrostUTXO
		path := fmt.Sprintf("/addresses/%s/utxos?page=%d", addr, page)
		err := b.doRequest(ctx, "GET", path, nil, &bfUtxos)
		if err != nil {
			if page == 1 && errors.Is(err, connector.ErrNotFound) {
				return []UTxO.UTxO{}, nil
			}
			return nil, err
		}

		if len(bfUtxos) == 0 {
			break
		}

		allBfUtxos = append(allBfUtxos, bfUtxos...)

		if len(bfUtxos) < 100 {
			break
		}
		page++
	}

	return adaptBlockfrostAddressUTxOs(allBfUtxos, addr, ctx, b), nil
}

func (b *BlockfrostProvider) GetScriptCborByScriptHash(
	ctx context.Context,
	scriptHash string,
) (string, error) {
	var bfScriptCbor bfScriptCbor
	path := fmt.Sprintf("/scripts/%s/cbor", scriptHash)

	err := b.doRequest(ctx, "GET", path, nil, &bfScriptCbor)
	if err != nil {
		return "", err
	}

	if bfScriptCbor.ScriptCbor == "" {
		return "", fmt.Errorf(
			"no script CBOR found for script hash: %s",
			scriptHash,
		)
	}

	return bfScriptCbor.ScriptCbor, nil
}

func (b *BlockfrostProvider) GetUtxosWithUnit(
	ctx context.Context,
	addr string,
	unit string,
) ([]UTxO.UTxO, error) {
	var allBfUtxos []BlockfrostUTXO
	page := 1

	for {
		var bfUtxos []BlockfrostUTXO
		path := fmt.Sprintf("/addresses/%s/utxos/%s?page=%d", addr, unit, page)
		err := b.doRequest(ctx, "GET", path, nil, &bfUtxos)
		if err != nil {
			if page == 1 && errors.Is(err, connector.ErrNotFound) {
				return []UTxO.UTxO{}, nil
			}
			return nil, err
		}

		if len(bfUtxos) == 0 {
			break
		}

		allBfUtxos = append(allBfUtxos, bfUtxos...)

		if len(bfUtxos) < 100 {
			break
		}
		page++
	}

	return adaptBlockfrostAddressUTxOs(allBfUtxos, addr, ctx, b), nil
}

// GetUtxoByUnit queries a UTxO by a specific unit.
// Blockfrost doesn't have a direct endpoint for "get UTxO by unit".
// We can query /assets/{asset}/addresses, then for each address, query its UTxOs for that asset.
// This can be inefficient. If it's an NFT (supply 1), we can be more direct.
func (b *BlockfrostProvider) GetUtxoByUnit(
	ctx context.Context,
	unit string,
) (*UTxO.UTxO, error) {
	// Get addresses holding this asset with count=2 to check if it's held by multiple addresses
	var addressesHoldingAsset []struct {
		Address  string `json:"address"`
		Quantity string `json:"quantity"`
	}

	assetAddressesPath := fmt.Sprintf("/assets/%s/addresses?count=2", unit)
	err := b.doRequest(
		ctx,
		"GET",
		assetAddressesPath,
		nil,
		&addressesHoldingAsset,
	)
	if err != nil {
		if errors.Is(err, connector.ErrNotFound) {
			return nil, fmt.Errorf("unit not found: %w", connector.ErrNotFound)
		}
		return nil, fmt.Errorf(
			"failed to get addresses for asset %s: %w",
			unit,
			err,
		)
	}

	if len(addressesHoldingAsset) == 0 {
		return nil, fmt.Errorf("unit not found: %w", connector.ErrNotFound)
	}

	if len(addressesHoldingAsset) > 1 {
		return nil, errors.New(
			"unit needs to be an NFT or only held by one address",
		)
	}

	address := addressesHoldingAsset[0].Address

	utxos, err := b.GetUtxosWithUnit(ctx, address, unit)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get UTxOs for address %s with unit %s: %w",
			address,
			unit,
			err,
		)
	}

	if len(utxos) == 0 {
		return nil, fmt.Errorf(
			"unit not found in address UTxOs: %w",
			connector.ErrNotFound,
		)
	}

	if len(utxos) > 1 {
		return nil, errors.New(
			"unit needs to be an NFT or only held by one address",
		)
	}

	return &utxos[0], nil
}

// GetUtxosByOutRef queries UTxOs by their output references.
func (b *BlockfrostProvider) GetUtxosByOutRef(
	ctx context.Context,
	outRefs []connector.OutRef,
) ([]UTxO.UTxO, error) {
	if len(outRefs) == 0 {
		return []UTxO.UTxO{}, nil
	}

	uniqueTxHashes := make(map[string]bool)
	for _, ref := range outRefs {
		uniqueTxHashes[ref.TxHash] = true
	}

	type txResult struct {
		txHash string
		utxos  Base.TxUtxos
		err    error
	}

	resultChan := make(chan txResult, len(uniqueTxHashes))

	for txHash := range uniqueTxHashes {
		go func(hash string) {
			var txUtxos Base.TxUtxos
			path := fmt.Sprintf("/txs/%s/utxos", hash)
			err := b.doRequest(ctx, "GET", path, nil, &txUtxos)
			resultChan <- txResult{txHash: hash, utxos: txUtxos, err: err}
		}(txHash)
	}

	txUtxosMap := make(map[string]Base.TxUtxos)
	for i := 0; i < len(uniqueTxHashes); i++ {
		result := <-resultChan
		if result.err != nil {
			if !errors.Is(result.err, connector.ErrNotFound) {
				return nil, fmt.Errorf(
					"failed to get UTxOs for tx %s: %w",
					result.txHash,
					result.err,
				)
			}
			continue
		}
		txUtxosMap[result.txHash] = result.utxos
	}

	var results []UTxO.UTxO
	for _, ref := range outRefs {
		txUtxos, exists := txUtxosMap[ref.TxHash]
		if !exists {
			continue
		}
		for _, bfOut := range txUtxos.Outputs {
			if bfOut.OutputIndex == int(ref.Index) {
				apolloUtxo, err := adaptBlockfrostTxOutputToApolloUTxO(
					ref.TxHash,
					bfOut,
					ctx,
					b,
				)
				if err != nil {
					return nil, fmt.Errorf(
						"failed to adapt utxo for %s#%d: %w",
						ref.TxHash,
						ref.Index,
						err,
					)
				}
				results = append(results, apolloUtxo)
				break
			}
		}
	}

	return results, nil
}

// GetDelegation fetches delegation information for a reward address.
func (b *BlockfrostProvider) GetDelegation(
	ctx context.Context,
	stakeAddrStr string,
) (connector.Delegation, error) {
	if !strings.HasPrefix(stakeAddrStr, "stake") {
		return connector.Delegation{}, fmt.Errorf(
			"%w: expected a stake address (stake1...)",
			connector.ErrInvalidAddress,
		)
	}

	var bfAccountDetails BlockfrostAccountDetails // Use the specific struct
	path := "/accounts/" + stakeAddrStr

	err := b.doRequest(ctx, "GET", path, nil, &bfAccountDetails)
	if err != nil {
		// If the account is not found, Blockfrost returns 404.
		// This means the account has never participated or has no history.
		// In this case, it's not delegated and has 0 rewards.
		if errors.Is(err, connector.ErrNotFound) {
			return connector.Delegation{
				PoolId:  "",
				Rewards: 0,
				Active:  false,
			}, nil
		}
		return connector.Delegation{}, fmt.Errorf(
			"failed to get account details for %s: %w",
			stakeAddrStr,
			err,
		)
	}

	return adaptBlockfrostAccountToDelegation(bfAccountDetails), nil
}

// GetDatum fetches a datum by its hash.
func (b *BlockfrostProvider) GetDatum(
	ctx context.Context,
	datumHash string,
) (PlutusData.PlutusData, error) {
	var bfDatum struct { // Blockfrost returns { "json_value": null, "cbor": "..." }
		Cbor  string `json:"cbor"`
		Error string `json:"error"`
	}
	path := fmt.Sprintf("/scripts/datum/%s/cbor", datumHash)
	err := b.doRequest(ctx, "GET", path, nil, &bfDatum)
	if err != nil {
		return PlutusData.PlutusData{}, err
	}

	if bfDatum.Error != "" || bfDatum.Cbor == "" {
		return PlutusData.PlutusData{}, fmt.Errorf(
			"no datum found for datum hash: %s",
			datumHash,
		)
	}

	datumBytes, err := hex.DecodeString(bfDatum.Cbor)
	if err != nil {
		return PlutusData.PlutusData{}, fmt.Errorf(
			"invalid datum cbor hex from blockfrost: %w",
			err,
		)
	}
	var pd PlutusData.PlutusData
	if err := cbor.Unmarshal(datumBytes, &pd); err != nil {
		return PlutusData.PlutusData{}, fmt.Errorf(
			"failed to unmarshal datum cbor: %w",
			err,
		)
	}
	return pd, nil
}

// AwaitTx waits for a transaction to be confirmed.
func (b *BlockfrostProvider) AwaitTx(
	ctx context.Context,
	txHash string,
	checkInterval time.Duration,
) (bool, error) {
	if checkInterval <= 0 {
		checkInterval = 3 * time.Second
	}
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-ticker.C:
			var txInfo struct { // Blockfrost's /txs/{hash} response
				Block string `json:"block"`
				Error string `json:"error"`
			}
			path := "/txs/" + txHash
			err := b.doRequest(ctx, "GET", path, nil, &txInfo)
			if err != nil {
				// Check if it's a 404 (transaction not found yet) using the wrapped error
				if errors.Is(err, connector.ErrNotFound) {
					continue // Not found yet, keep waiting
				}
				return false, err // Other error
			}

			// Check if the response has an error field (transaction exists but has error)
			if txInfo.Error != "" {
				continue // Transaction found but has error, keep waiting
			}

			// Check if transaction is confirmed (has block hash)
			if txInfo.Block != "" {
				select {
				case <-ctx.Done():
					return false, ctx.Err()
				case <-time.After(1 * time.Second):
					return true, nil
				}
			}
		}
	}
}

// SubmitTx submits a signed transaction.
func (b *BlockfrostProvider) SubmitTx(
	ctx context.Context,
	txBytes []byte,
) (string, error) {
	var submittedTxHashStr string // Blockfrost returns the tx hash as plain text (quoted string)

	// custom submission endpoints first
	if len(b.customSubmissionEndpoints) > 0 {
		for _, endpoint := range b.customSubmissionEndpoints {
			err := b.doCustomSubmit(ctx, endpoint, txBytes, &submittedTxHashStr)
			if err == nil && submittedTxHashStr != "" {
				return submittedTxHashStr, nil
			}
		}
	}

	err := b.doRequest(
		ctx,
		"POST",
		"/tx/submit",
		bytes.NewReader(txBytes),
		&submittedTxHashStr,
	)
	if err != nil {
		return "", fmt.Errorf("blockfrost tx submission failed: %w", err)
	}
	if submittedTxHashStr == "" {
		return "", errors.New(
			"blockfrost did not return a transaction hash on submission",
		)
	}
	return submittedTxHashStr, nil
}

func (b *BlockfrostProvider) doCustomSubmit(
	ctx context.Context,
	endpoint string,
	txBytes []byte,
	target *string,
) error {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(txBytes),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/cbor")
	// Custom endpoints might or might not need project_id
	// if b.projectID != "" && strings.Contains(endpoint, "blockfrost.io") {
	//    req.Header.Set("project_id", b.projectID)
	// }

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf(
			"custom submit to %s failed: status %d, body: %s",
			endpoint,
			resp.StatusCode,
			string(bodyBytes),
		)
	}
	if target != nil {
		*target = strings.Trim(string(bodyBytes), "\"")
	}
	return nil
}

// EvaluateTx evaluates a transaction's scripts.
func (b *BlockfrostProvider) EvaluateTx(
	ctx context.Context,
	txBytes []byte,
	additionalUTxOs []UTxO.UTxO,
) ([]connector.EvalRedeemer, error) {
	additionalBfUtxos := make([]bfAdditionalUtxoItem, 0, len(additionalUTxOs))
	for _, utxo := range additionalUTxOs {

		txIn := bfTxIn{
			TxId:  hex.EncodeToString(utxo.Input.TransactionId),
			Index: utxo.Input.Index,
		}

		currentUtxoAmount := utxo.Output.GetAmount()
		bfVal := bfValue{
			Coins: currentUtxoAmount.GetCoin(),
		}

		if currentUtxoAmount.HasAssets {
			assets := make(map[string]int64)
			for policyId, assetMap := range currentUtxoAmount.Am.Value {
				for assetName, quantity := range assetMap {
					assetNameHex := assetName.HexString()
					var unit string
					if assetNameHex == "" {
						unit = policyId.Value
					} else {
						unit = policyId.Value + "." + assetNameHex
					}
					assets[unit] = quantity
				}
			}
			if len(assets) > 0 {
				bfVal.Assets = assets
			}
		}

		txOut := bfTxOut{
			Address: utxo.Output.GetAddress().String(),
			Value:   bfVal,
		}

		if datumHash := utxo.Output.GetDatumHash(); datumHash != nil &&
			datumHash.Payload != nil &&
			len(datumHash.Payload) > 0 {
			datumHashHex := hex.EncodeToString(datumHash.Payload)
			txOut.DatumHash = &datumHashHex
		}

		if utxo.Output.IsPostAlonzo && utxo.Output.PostAlonzo.Datum != nil {
			if inlineDatum := utxo.Output.GetDatum(); inlineDatum != nil {
				datumCbor, err := cbor.Marshal(inlineDatum)
				if err == nil {
					datumHex := hex.EncodeToString(datumCbor)
					txOut.Datum = &datumHex
				}
			}
		}

		if scriptRef := utxo.Output.GetScriptRef(); scriptRef != nil {
			scriptCbor, err := cbor.Marshal(scriptRef)
			if err == nil {
				scriptHex := hex.EncodeToString(scriptCbor)
				// TODO dyanmically choose v1, v2, or v3
				txOut.ScriptRef = &bfScriptRef{
					PlutusV2: &scriptHex,
				}
			}
		}

		additionalBfUtxos = append(
			additionalBfUtxos,
			bfAdditionalUtxoItem{txIn, txOut},
		)
	}

	evalReq := bfEvalRequest{
		Cbor:              hex.EncodeToString(txBytes),
		AdditionalUtxoSet: additionalBfUtxos,
	}

	reqBodyBytes, err := json.MarshalIndent(evalReq, "", "  ")
	if err != nil {
		return nil, fmt.Errorf(
			"failed to marshal blockfrost eval request: %w",
			err,
		)
	}

	req, _ := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		b.baseURL+"/utils/txs/evaluate/utxos",
		bytes.NewReader(reqBodyBytes),
	)
	if b.projectID != "" {
		req.Header.Set("project_id", b.projectID)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("blockfrost eval request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			StatusCode int    `json:"status_code"`
			Message    string `json:"message"`
			Fault      bool   `json:"fault"`
		}
		if json.Unmarshal(respBytes, &errorResp) == nil {
			if errorResp.StatusCode == http.StatusBadRequest {
				return nil, fmt.Errorf("%s", errorResp.Message)
			}
		}
		return nil, fmt.Errorf(
			"could not evaluate the transaction: %s. Transaction: %s",
			string(respBytes),
			hex.EncodeToString(txBytes),
		)
	}

	var outerResult struct {
		Result bfEvalResult `json:"result"`
	}

	if err := json.Unmarshal(respBytes, &outerResult); err != nil {
		return nil, fmt.Errorf(
			"failed to unmarshal blockfrost eval response: %w. Body: %s",
			err,
			string(respBytes),
		)
	}

	if outerResult.Result.Result == nil {
		return nil, fmt.Errorf(
			"EvaluateTransaction fails: %s",
			string(respBytes),
		)
	}

	return adaptBlockfrostEvalResult(outerResult.Result), nil
}
