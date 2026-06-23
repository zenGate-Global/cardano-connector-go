package maestro

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	maestroClient "github.com/maestro-org/go-sdk/client"
	"github.com/maestro-org/go-sdk/utils"

	"github.com/Salvionied/apollo/v2/backend"
	connector "github.com/zenGate-Global/cardano-connector-go"
)

var _ connector.Provider = (*MaestroProvider)(nil)

// New creates a new Maestro provider instance.
func New(config Config) (*MaestroProvider, error) {
	if config.ProjectID == "" {
		return nil, errors.New("maestro project ID is required")
	}

	networkName := strings.ToLower(config.NetworkName)
	switch networkName {
	case "mainnet", "preprod", "preview":
		// Supported networks
	default:
		return nil, fmt.Errorf(
			"unsupported or missing network name: %s",
			config.NetworkName,
		)
	}

	client := maestroClient.NewClient(config.ProjectID, networkName)
	genesisParams, err := resolveGenesisParams(config, networkName)
	if err != nil {
		return nil, err
	}
	protocolParamsPreset, err := resolveProtocolParamsPreset(networkName)
	if err != nil {
		return nil, err
	}

	provider := &MaestroProvider{
		client:                 client,
		genesisParams:          genesisParams,
		protocolParamsOverride: config.ProtocolParamsOverride,
		protocolParamsPreset:   protocolParamsPreset,
		networkName:            networkName,
		networkId:              config.NetworkId,
	}

	return provider, nil
}

// Network returns the network ID of the provider.
func (m *MaestroProvider) Network() int {
	return m.networkId
}

// Epoch returns the current epoch number.
func (m *MaestroProvider) Epoch(ctx context.Context) (int, error) {
	resp, err := m.client.CurrentEpoch()
	if err != nil {
		return 0, fmt.Errorf("maestro: failed to get current epoch: %w", err)
	}
	return resp.Data.EpochNo, nil
}

// GetProtocolParameters fetches the current protocol parameters from Maestro.
func (m *MaestroProvider) GetProtocolParameters(
	ctx context.Context,
) (backend.ProtocolParameters, error) {
	if m.protocolParamsOverride != nil {
		return *m.protocolParamsOverride, nil
	}

	resp, err := m.client.ProtocolParameters()
	if err != nil {
		return backend.ProtocolParameters{}, fmt.Errorf(
			"maestro: failed to get protocol parameters: %w",
			err,
		)
	}

	protocolParams, err := adaptMaestroProtocolParams(resp.Data)
	if err != nil {
		return backend.ProtocolParameters{}, fmt.Errorf(
			"maestro: failed to adapt protocol parameters: %w",
			err,
		)
	}
	return mergeMaestroProtocolParams(protocolParams, m.protocolParamsPreset), nil
}

// GetGenesisParams returns the genesis parameters for the configured network.
func (m *MaestroProvider) GetGenesisParams(
	ctx context.Context,
) (backend.GenesisParameters, error) {
	_ = ctx
	return m.genesisParams, nil
}

// GetTip returns the current tip of the blockchain.
func (m *MaestroProvider) GetTip(ctx context.Context) (connector.Tip, error) {
	resp, err := m.client.ChainTip()
	if err != nil {
		return connector.Tip{}, fmt.Errorf(
			"maestro: failed to get chain tip: %w",
			err,
		)
	}
	return connector.Tip{
		Slot:   uint64(resp.Data.Slot),
		Height: uint64(resp.Data.Height),
		Hash:   resp.Data.BlockHash,
	}, nil
}

// GetUtxosByAddress fetches all UTxOs for a given address.
func (m *MaestroProvider) GetUtxosByAddress(
	ctx context.Context,
	addr string,
) ([]common.Utxo, error) {
	address, err := common.NewAddress(addr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", connector.ErrInvalidAddress, err)
	}
	return m.collectUtxos(addr, address, nil)
}

// GetUtxosWithUnit fetches all UTxOs for a given address that contain a specific asset.
func (m *MaestroProvider) GetUtxosWithUnit(
	ctx context.Context,
	addr, unit string,
) ([]common.Utxo, error) {
	address, err := common.NewAddress(addr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", connector.ErrInvalidAddress, err)
	}
	return m.collectUtxos(addr, address, &unit)
}

// collectUtxos pages through Maestro's UTxOs-at-address endpoint, optionally
// filtered by asset unit, and converts each entry to a gouroboros common.Utxo.
func (m *MaestroProvider) collectUtxos(
	addrStr string,
	address common.Address,
	unit *string,
) ([]common.Utxo, error) {
	const maxPages = 1000
	utxos := make([]common.Utxo, 0)
	var lastCursor string

	newParams := func() *utils.Parameters {
		params := utils.NewParameters()
		if unit != nil {
			params.Asset(*unit)
		}
		// Request the resolved output CBOR and resolved datums so inline datums
		// and reference scripts hydrate completely (see maestroUtxoToCommon).
		params.WithCbor()
		params.ResolveDatums()
		return params
	}

	params := newParams()
	for range maxPages {
		resp, err := m.client.UtxosAtAddress(addrStr, params)
		if err != nil {
			return nil, err
		}
		for _, maestroUtxo := range resp.Data {
			utxo, err := maestroUtxoToCommon(maestroUtxo, address)
			if err != nil {
				return nil, fmt.Errorf("maestro: failed to parse UTxO: %w", err)
			}
			utxos = append(utxos, utxo)
		}

		lastCursor = resp.NextCursor
		if lastCursor == "" {
			break
		}
		params = newParams()
		params.Cursor(lastCursor)
	}

	if lastCursor != "" {
		return nil, fmt.Errorf("maestro: UTxO pagination exceeded %d pages; results may be incomplete", maxPages)
	}

	return utxos, nil
}

// GetScriptCborByScriptHash fetches the CBOR of a script by its hash, hex-encoded.
func (m *MaestroProvider) GetScriptCborByScriptHash(
	ctx context.Context,
	scriptHash string,
) (string, error) {
	resp, err := m.client.ScriptByHash(scriptHash)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return "", fmt.Errorf(
				"maestro: script not found for hash %s: %w",
				scriptHash,
				connector.ErrNotFound,
			)
		}
		return "", fmt.Errorf(
			"maestro: failed to get script by hash %s: %w",
			scriptHash,
			err,
		)
	}

	if resp.Data.Bytes == "" {
		return "", fmt.Errorf(
			"maestro: no script CBOR available for hash %s: %w",
			scriptHash,
			connector.ErrNotFound,
		)
	}
	return resp.Data.Bytes, nil
}

// GetUtxoByUnit finds the single UTxO containing a specific unit (NFT).
func (m *MaestroProvider) GetUtxoByUnit(
	ctx context.Context,
	unit string,
) (*common.Utxo, error) {
	params := utils.NewParameters()
	params.Count(2)

	resp, err := m.client.AddressHoldingAsset(unit, params)
	if err != nil {
		return nil, err
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("unit not found: %w", connector.ErrNotFound)
	}
	if len(resp.Data) > 1 {
		return nil, errors.New(
			"unit is held by more than one address, cannot determine unique UTxO",
		)
	}

	address := resp.Data[0].Address
	utxos, err := m.GetUtxosWithUnit(ctx, address, unit)
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
			"unit is present in multiple UTxOs at the same address",
		)
	}

	return &utxos[0], nil
}

// GetUtxosByOutRef queries UTxOs by their output references.
func (m *MaestroProvider) GetUtxosByOutRef(
	ctx context.Context,
	outRefs []connector.OutRef,
) ([]common.Utxo, error) {
	if len(outRefs) == 0 {
		return nil, nil
	}

	results := make([]common.Utxo, 0, len(outRefs))

	for _, ref := range outRefs {
		// Request the resolved output CBOR and datums so inline datums and
		// reference scripts hydrate completely (see maestroUtxoToCommon).
		params := utils.NewParameters()
		params.WithCbor()
		params.ResolveDatums()
		resp, err := m.client.TransactionOutputFromReference(
			ref.TxHash,
			int(ref.Index),
			params,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"maestro: failed to get utxo %s#%d: %w",
				ref.TxHash,
				ref.Index,
				err,
			)
		}

		address, err := common.NewAddress(resp.Data.Address)
		if err != nil {
			return nil, fmt.Errorf(
				"maestro: invalid address for utxo %s#%d: %w",
				ref.TxHash,
				ref.Index,
				err,
			)
		}
		utxo, err := maestroUtxoToCommon(resp.Data, address)
		if err != nil {
			return nil, fmt.Errorf(
				"maestro: failed to adapt utxo %s#%d: %w",
				ref.TxHash,
				ref.Index,
				err,
			)
		}
		results = append(results, utxo)
	}
	return results, nil
}

// GetDelegation fetches delegation information for a reward address.
func (m *MaestroProvider) GetDelegation(
	ctx context.Context,
	stakeAddrStr string,
) (connector.Delegation, error) {
	if !strings.HasPrefix(stakeAddrStr, "stake") {
		return connector.Delegation{}, fmt.Errorf(
			"%w: expected a stake address (stake1...)",
			connector.ErrInvalidAddress,
		)
	}

	resp, err := m.client.StakeAccountInformation(stakeAddrStr)
	if err != nil {
		return connector.Delegation{}, fmt.Errorf(
			"maestro: failed to get account info for %s: %w",
			stakeAddrStr,
			err,
		)
	}

	blockResp, err := m.client.BlockInfo(resp.LastUpdated.BlockHash)
	if err != nil {
		return connector.Delegation{}, fmt.Errorf(
			"maestro: failed to get block info while getting account info for %s: %w",
			stakeAddrStr,
			err,
		)
	}

	return adaptMaestroDelegation(resp.Data, int(blockResp.Data.Epoch)), nil
}

// GetDatum fetches a datum by its hash and decodes it into a gouroboros Datum.
func (m *MaestroProvider) GetDatum(
	ctx context.Context,
	datumHash string,
) (common.Datum, error) {
	resp, err := m.client.DatumFromHash(datumHash)
	if err != nil {
		return common.Datum{}, fmt.Errorf(
			"maestro: failed to get datum by hash %s: %w",
			datumHash,
			err,
		)
	}

	if resp.Data.Bytes == "" {
		return common.Datum{}, fmt.Errorf(
			"maestro: no datum found for datum hash %s: %w",
			datumHash,
			connector.ErrNotFound,
		)
	}

	datumBytes, err := hex.DecodeString(resp.Data.Bytes)
	if err != nil {
		return common.Datum{}, fmt.Errorf(
			"invalid datum cbor hex from maestro: %w",
			err,
		)
	}
	var datum common.Datum
	if err := datum.UnmarshalCBOR(datumBytes); err != nil {
		return common.Datum{}, fmt.Errorf(
			"failed to unmarshal datum cbor: %w",
			err,
		)
	}
	return datum, nil
}

// AwaitTx waits for a transaction to be confirmed.
func (m *MaestroProvider) AwaitTx(
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
			_, err := m.client.TransactionCbor(txHash)
			if err != nil {
				if strings.Contains(err.Error(), "404") {
					continue // Not found yet, keep waiting
				}
				// Any other error is a failure.
				return false, fmt.Errorf(
					"maestro: error while checking tx status for %s: %w",
					txHash,
					err,
				)
			}
			// If no error, the transaction is found and thus confirmed.
			return true, nil
		}
	}
}

// SubmitTx submits a signed transaction and returns its hash as a hex string.
func (m *MaestroProvider) SubmitTx(
	ctx context.Context,
	txBytes []byte,
) (string, error) {
	// The Maestro SDK's Client.SubmitTx posts to a corrupted URL
	// ("/submitmodels.BasicResponse{}/tx") and can never work. Use
	// TxManagerSubmit instead, which posts the hex-encoded transaction
	// CBOR to the documented POST /txmanager submit endpoint.
	txHex := hex.EncodeToString(txBytes)
	txHash, err := m.client.TxManagerSubmit(txHex)
	if err != nil {
		return "", fmt.Errorf("maestro: tx submission failed: %w", err)
	}
	// The endpoint returns the tx hash as a plain-text body; tolerate JSON
	// string quoting and surrounding whitespace.
	txHash = strings.Trim(strings.TrimSpace(txHash), `"`)
	if txHash == "" {
		return "", errors.New(
			"maestro did not return a transaction hash on submission",
		)
	}
	return txHash, nil
}

// EvaluateTx evaluates a transaction's scripts.
//
// additionalUTxOs is IGNORED. The Maestro SDK exposes additional UTxOs as a
// variadic []string (models.AdditionalUtxo), but Maestro's REST
// /transactions/evaluate additional_utxos field expects an array of objects
// (tx ref + resolved output CBOR), which the SDK type cannot represent
// faithfully. Rather than ship a guessed wire format, the resolved UTxOs are
// not forwarded: this backend can only evaluate transactions whose inputs are
// already visible on-chain to Maestro, and does NOT support evaluating
// off-chain or chained inputs.
func (m *MaestroProvider) EvaluateTx(
	ctx context.Context,
	txBytes []byte,
	additionalUTxOs []common.Utxo,
) (map[common.RedeemerKey]common.ExUnits, error) {
	_ = additionalUTxOs
	txHex := hex.EncodeToString(txBytes)
	evaluation, err := m.client.EvaluateTx(txHex)
	if err != nil {
		return nil, err
	}
	return evaluationsToExUnits(evaluation)
}
