package blockfrost

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Salvionied/apollo/v2/backend"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
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
		if strings.Contains(baseURL, "blockfrost.io") && !strings.HasSuffix(baseURL, "/v0") {
			baseURL += "/v0"
		}
	}

	provider := &BlockfrostProvider{
		httpClient:                httpClient,
		baseURL:                   baseURL,
		projectID:                 config.ProjectID,
		networkName:               config.NetworkName,
		networkId:                 config.NetworkId,
		customSubmissionEndpoints: config.CustomSubmissionEndpoints,
	}
	return provider, nil
}

func (b *BlockfrostProvider) Network() int {
	return b.networkId
}

func (b *BlockfrostProvider) Epoch(ctx context.Context) (int, error) {
	var bfEpoch BlockfrostEpoch
	path := "/epochs/latest"

	err := b.doRequest(ctx, "GET", path, nil, &bfEpoch)
	if err != nil {
		return 0, fmt.Errorf("failed to get current epoch: %w", err)
	}

	return bfEpoch.Epoch, nil
}

// GetProtocolParameters fetches the current protocol parameters from Blockfrost.
func (b *BlockfrostProvider) GetProtocolParameters(
	ctx context.Context,
) (backend.ProtocolParameters, error) {
	var raw bfProtocolParams
	path := "/epochs/latest/parameters"

	err := b.doRequest(ctx, "GET", path, nil, &raw)
	if err != nil {
		return backend.ProtocolParameters{}, fmt.Errorf(
			"failed to get protocol parameters: %w",
			err,
		)
	}

	return raw.toProtocolParams()
}

func (b *BlockfrostProvider) GetGenesisParams(
	ctx context.Context,
) (backend.GenesisParameters, error) {
	var raw bfGenesisParams
	path := "/genesis"

	err := b.doRequest(ctx, "GET", path, nil, &raw)
	if err != nil {
		return backend.GenesisParameters{}, fmt.Errorf(
			"failed to get genesis parameters: %w",
			err,
		)
	}

	return backend.GenesisParameters{
		ActiveSlotsCoefficient: raw.ActiveSlotsCoefficient,
		UpdateQuorum:           raw.UpdateQuorum,
		MaxLovelaceSupply:      strconv.FormatInt(raw.MaxLovelaceSupply, 10),
		NetworkMagic:           raw.NetworkMagic,
		EpochLength:            raw.EpochLength,
		SystemStart:            raw.SystemStart,
		SlotsPerKesPeriod:      raw.SlotsPerKesPeriod,
		SlotLength:             raw.SlotLength,
		MaxKesEvolutions:       raw.MaxKesEvolutions,
		SecurityParam:          raw.SecurityParam,
	}, nil
}

func (b *BlockfrostProvider) GetTip(
	ctx context.Context,
) (connector.Tip, error) {
	var bfTip struct {
		Height uint64 `json:"height"`
		Hash   string `json:"hash"`
		Slot   uint64 `json:"slot"`
	}
	path := "/blocks/latest"

	err := b.doRequest(ctx, "GET", path, nil, &bfTip)
	if err != nil {
		return connector.Tip{}, fmt.Errorf("failed to get tip: %w", err)
	}
	return connector.Tip{
		Slot:   bfTip.Slot,
		Height: bfTip.Height,
		Hash:   bfTip.Hash,
	}, nil
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
) ([]common.Utxo, error) {
	address, err := common.NewAddress(addr)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", connector.ErrInvalidAddress, addr, err)
	}
	return b.fetchUtxosPaged(ctx, address, fmt.Sprintf("/addresses/%s/utxos", addr))
}

func (b *BlockfrostProvider) GetUtxosWithUnit(
	ctx context.Context,
	addr string,
	unit string,
) ([]common.Utxo, error) {
	address, err := common.NewAddress(addr)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", connector.ErrInvalidAddress, addr, err)
	}
	return b.fetchUtxosPaged(ctx, address, fmt.Sprintf("/addresses/%s/utxos/%s", addr, unit))
}

// fetchUtxosPaged fetches and hydrates all pages of a Blockfrost UTxO listing.
func (b *BlockfrostProvider) fetchUtxosPaged(
	ctx context.Context,
	address common.Address,
	basePath string,
) ([]common.Utxo, error) {
	var allUtxos []common.Utxo
	page := 1

	for {
		var rawUtxos []bfAddressUTxO
		sep := "?"
		if strings.Contains(basePath, "?") {
			sep = "&"
		}
		path := fmt.Sprintf("%s%spage=%d", basePath, sep, page)
		err := b.doRequest(ctx, "GET", path, nil, &rawUtxos)
		if err != nil {
			if page == 1 && errors.Is(err, connector.ErrNotFound) {
				return []common.Utxo{}, nil
			}
			return nil, err
		}

		if len(rawUtxos) == 0 {
			break
		}

		for _, raw := range rawUtxos {
			utxo, err := b.hydrateUtxo(ctx, raw, address)
			if err != nil {
				return nil, fmt.Errorf("failed to parse UTxO %s#%d: %w", raw.TxHash, raw.OutputIndex, err)
			}
			allUtxos = append(allUtxos, utxo)
		}

		if len(rawUtxos) < 100 {
			break
		}
		page++
	}

	return allUtxos, nil
}

func (b *BlockfrostProvider) GetScriptCborByScriptHash(
	ctx context.Context,
	scriptHash string,
) (string, error) {
	var bfScript bfScriptCbor
	path := fmt.Sprintf("/scripts/%s/cbor", scriptHash)

	err := b.doRequest(ctx, "GET", path, nil, &bfScript)
	if err != nil {
		return "", err
	}

	if bfScript.ScriptCbor == "" {
		return "", fmt.Errorf(
			"no script CBOR found for script hash: %s",
			scriptHash,
		)
	}

	return bfScript.ScriptCbor, nil
}

// GetUtxoByUnit queries a UTxO by a specific unit.
func (b *BlockfrostProvider) GetUtxoByUnit(
	ctx context.Context,
	unit string,
) (*common.Utxo, error) {
	var addressesHoldingAsset []struct {
		Address  string `json:"address"`
		Quantity string `json:"quantity"`
	}

	assetAddressesPath := fmt.Sprintf("/assets/%s/addresses?count=2", unit)
	err := b.doRequest(ctx, "GET", assetAddressesPath, nil, &addressesHoldingAsset)
	if err != nil {
		if errors.Is(err, connector.ErrNotFound) {
			return nil, fmt.Errorf("unit not found: %w", connector.ErrNotFound)
		}
		return nil, fmt.Errorf("failed to get addresses for asset %s: %w", unit, err)
	}

	if len(addressesHoldingAsset) == 0 {
		return nil, fmt.Errorf("unit not found: %w", connector.ErrNotFound)
	}

	if len(addressesHoldingAsset) > 1 {
		return nil, connector.ErrMultipleUTXOs
	}

	address := addressesHoldingAsset[0].Address

	utxos, err := b.GetUtxosWithUnit(ctx, address, unit)
	if err != nil {
		return nil, fmt.Errorf("failed to get UTxOs for address %s with unit %s: %w", address, unit, err)
	}

	if len(utxos) == 0 {
		return nil, fmt.Errorf("unit not found in address UTxOs: %w", connector.ErrNotFound)
	}

	if len(utxos) > 1 {
		return nil, connector.ErrMultipleUTXOs
	}

	return &utxos[0], nil
}

// GetUtxosByOutRef queries UTxOs by their output references.
func (b *BlockfrostProvider) GetUtxosByOutRef(
	ctx context.Context,
	outRefs []connector.OutRef,
) ([]common.Utxo, error) {
	if len(outRefs) == 0 {
		return []common.Utxo{}, nil
	}

	uniqueTxHashes := make(map[string]bool)
	for _, ref := range outRefs {
		uniqueTxHashes[ref.TxHash] = true
	}

	type txResult struct {
		txHash  string
		outputs []bfAddressUTxO
		err     error
	}

	resultChan := make(chan txResult, len(uniqueTxHashes))

	for txHash := range uniqueTxHashes {
		go func(hash string) {
			var txUtxos struct {
				Outputs []bfAddressUTxO `json:"outputs"`
			}
			path := fmt.Sprintf("/txs/%s/utxos", hash)
			err := b.doRequest(ctx, "GET", path, nil, &txUtxos)
			resultChan <- txResult{txHash: hash, outputs: txUtxos.Outputs, err: err}
		}(txHash)
	}

	txOutputsMap := make(map[string][]bfAddressUTxO)
	for i := 0; i < len(uniqueTxHashes); i++ {
		result := <-resultChan
		if result.err != nil {
			if !errors.Is(result.err, connector.ErrNotFound) {
				return nil, fmt.Errorf("failed to get UTxOs for tx %s: %w", result.txHash, result.err)
			}
			continue
		}
		txOutputsMap[result.txHash] = result.outputs
	}

	var results []common.Utxo
	for _, ref := range outRefs {
		outputs, exists := txOutputsMap[ref.TxHash]
		if !exists {
			continue
		}
		for _, raw := range outputs {
			if raw.OutputIndex == int(ref.Index) {
				// The /txs/{hash}/utxos outputs carry no tx_hash field, so set
				// it from the requested ref before hydrating.
				raw.TxHash = ref.TxHash
				addr, err := common.NewAddress(raw.Address)
				if err != nil {
					return nil, fmt.Errorf("failed to decode address %s: %w", raw.Address, err)
				}
				utxo, err := b.hydrateUtxo(ctx, raw, addr)
				if err != nil {
					return nil, fmt.Errorf("failed to adapt utxo for %s#%d: %w", ref.TxHash, ref.Index, err)
				}
				results = append(results, utxo)
				break
			}
		}
	}

	return results, nil
}

// hydrateUtxo builds a common.Utxo from a BlockFrost UTxO and layers on the
// inline datum and reference script (resolved by hash) when present.
func (b *BlockfrostProvider) hydrateUtxo(
	ctx context.Context,
	raw bfAddressUTxO,
	address common.Address,
) (common.Utxo, error) {
	utxo, err := raw.toUtxo(address)
	if err != nil {
		return common.Utxo{}, err
	}
	output, ok := utxo.Output.(*babbage.BabbageTransactionOutput)
	if !ok {
		return common.Utxo{}, fmt.Errorf("unexpected UTxO output type: %T", utxo.Output)
	}
	if len(raw.InlineDatum) > 0 && string(raw.InlineDatum) != "null" {
		datumOpt, err := inlineDatumOptionFromBlockfrost(raw.InlineDatum)
		if err != nil {
			return common.Utxo{}, fmt.Errorf("failed to decode inline datum: %w", err)
		}
		output.DatumOption = datumOpt
	}
	if raw.ReferenceScriptHash != "" {
		scriptRef, err := b.scriptRefByHash(ctx, raw.ReferenceScriptHash)
		if err != nil {
			// Chain-read hydration is best-effort: a reference script that
			// cannot be resolved (empty CBOR, native scripts served only at
			// /scripts/{hash}/json, parse error, transient failure) must NOT
			// abort the whole UTxO fetch. Keep the UTxO with an unresolved
			// (nil) reference script.
			slog.Warn("blockfrost: leaving reference script unresolved during hydration",
				"script_hash", raw.ReferenceScriptHash,
				"utxo", fmt.Sprintf("%s#%d", raw.TxHash, raw.OutputIndex),
				"err", err)
		} else {
			output.TxOutScriptRef = scriptRef
		}
	}
	return utxo, nil
}

// scriptRefByHash resolves a reference script's CBOR by hash and builds a typed
// gouroboros ScriptRef from it.
func (b *BlockfrostProvider) scriptRefByHash(ctx context.Context, hashHex string) (*common.ScriptRef, error) {
	hashBytes, err := hex.DecodeString(hashHex)
	if err != nil {
		return nil, fmt.Errorf("invalid script hash hex %q: %w", hashHex, err)
	}
	if len(hashBytes) != common.Blake2b224Size {
		return nil, fmt.Errorf("invalid script hash length: expected %d bytes, got %d", common.Blake2b224Size, len(hashBytes))
	}
	var scriptHash common.Blake2b224
	copy(scriptHash[:], hashBytes)

	scriptCborHex, err := b.GetScriptCborByScriptHash(ctx, hashHex)
	if err != nil {
		return nil, err
	}
	scriptCbor, err := hex.DecodeString(scriptCborHex)
	if err != nil {
		return nil, fmt.Errorf("invalid script CBOR hex: %w", err)
	}
	return scriptRefFromHash(scriptHash, scriptCbor)
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

	var bfAccountDetails BlockfrostAccountDetails
	path := "/accounts/" + stakeAddrStr

	err := b.doRequest(ctx, "GET", path, nil, &bfAccountDetails)
	if err != nil {
		if errors.Is(err, connector.ErrNotFound) {
			return connector.Delegation{
				PoolId:  "",
				Rewards: 0,
				Active:  false,
			}, nil
		}
		return connector.Delegation{}, fmt.Errorf("failed to get account details for %s: %w", stakeAddrStr, err)
	}

	return adaptBlockfrostAccountToDelegation(bfAccountDetails), nil
}

// GetDatum fetches a datum by its hash and returns the decoded gouroboros datum.
func (b *BlockfrostProvider) GetDatum(
	ctx context.Context,
	datumHash string,
) (common.Datum, error) {
	var bfDatum struct {
		Cbor  string `json:"cbor"`
		Error string `json:"error"`
	}
	path := fmt.Sprintf("/scripts/datum/%s/cbor", datumHash)
	err := b.doRequest(ctx, "GET", path, nil, &bfDatum)
	if err != nil {
		return common.Datum{}, err
	}

	if bfDatum.Error != "" || bfDatum.Cbor == "" {
		return common.Datum{}, fmt.Errorf("no datum found for datum hash: %s", datumHash)
	}

	datumBytes, err := hex.DecodeString(bfDatum.Cbor)
	if err != nil {
		return common.Datum{}, fmt.Errorf("invalid datum cbor hex from blockfrost: %w", err)
	}
	var datum common.Datum
	if err := datum.UnmarshalCBOR(datumBytes); err != nil {
		return common.Datum{}, fmt.Errorf("failed to unmarshal datum cbor: %w", err)
	}
	return datum, nil
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
			var txInfo struct {
				Block string `json:"block"`
				Error string `json:"error"`
			}
			path := "/txs/" + txHash
			err := b.doRequest(ctx, "GET", path, nil, &txInfo)
			if err != nil {
				if errors.Is(err, connector.ErrNotFound) {
					continue
				}
				return false, err
			}

			if txInfo.Error != "" {
				continue
			}

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
	var submittedTxHashStr string

	if len(b.customSubmissionEndpoints) > 0 {
		for _, endpoint := range b.customSubmissionEndpoints {
			err := b.doCustomSubmit(ctx, endpoint, txBytes, &submittedTxHashStr)
			if err == nil && submittedTxHashStr != "" {
				return submittedTxHashStr, nil
			}
		}
	}

	err := b.doRequest(ctx, "POST", "/tx/submit", bytes.NewReader(txBytes), &submittedTxHashStr)
	if err != nil {
		return "", fmt.Errorf("%w: %w", connector.ErrTxSubmissionFailed, err)
	}
	if submittedTxHashStr == "" {
		return "", errors.New("blockfrost did not return a transaction hash on submission")
	}
	return submittedTxHashStr, nil
}

func (b *BlockfrostProvider) doCustomSubmit(
	ctx context.Context,
	endpoint string,
	txBytes []byte,
	target *string,
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(txBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/cbor")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("custom submit to %s failed: status %d, body: %s", endpoint, resp.StatusCode, string(bodyBytes))
	}
	if target != nil {
		*target = strings.Trim(string(bodyBytes), "\"")
	}
	return nil
}

// EvaluateTx evaluates a transaction's scripts and returns the per-redeemer
// execution units. additionalUTxOs are forwarded to the evaluator (e.g. inputs
// not yet confirmed on-chain) via the /utils/txs/evaluate/utxos endpoint.
func (b *BlockfrostProvider) EvaluateTx(
	ctx context.Context,
	txBytes []byte,
	additionalUTxOs []common.Utxo,
) (map[common.RedeemerKey]common.ExUnits, error) {
	if len(additionalUTxOs) > 0 {
		items := make([]bfAdditionalUtxoItem, 0, len(additionalUTxOs))
		for _, utxo := range additionalUTxOs {
			item, err := bfAdditionalUtxoItemFromUtxo(utxo)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		evalReq := bfEvalRequest{
			Cbor:              hex.EncodeToString(txBytes),
			AdditionalUtxoSet: items,
		}
		reqBodyBytes, err := json.Marshal(evalReq)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal blockfrost eval request: %w", err)
		}
		data, err := b.doEvaluate(ctx, "/utils/txs/evaluate/utxos", reqBodyBytes, "application/json")
		if err != nil {
			return nil, err
		}
		return parseEvaluateTxResponse(data)
	}

	// Bare evaluation: BlockFrost expects the transaction CBOR hex-encoded in the
	// request body with Content-Type application/cbor.
	body := []byte(hex.EncodeToString(txBytes))
	data, err := b.doEvaluate(ctx, "/utils/txs/evaluate", body, "application/cbor")
	if err != nil {
		return nil, err
	}
	return parseEvaluateTxResponse(data)
}

// doEvaluate performs a POST to a BlockFrost evaluation endpoint and returns the
// raw response body, surfacing non-200 responses as errors.
func (b *BlockfrostProvider) doEvaluate(
	ctx context.Context,
	path string,
	body []byte,
	contentType string,
) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if b.projectID != "" {
		req.Header.Set("project_id", b.projectID)
	}
	req.Header.Set("Content-Type", contentType)

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
		if json.Unmarshal(respBytes, &errorResp) == nil && errorResp.Message != "" {
			return nil, fmt.Errorf("%w: %s", connector.ErrEvaluationFailed, errorResp.Message)
		}
		return nil, fmt.Errorf("%w: could not evaluate the transaction: %s", connector.ErrEvaluationFailed, evalErrorSnippet(respBytes))
	}
	return respBytes, nil
}
