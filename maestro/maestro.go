package maestro

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Salvionied/apollo/serialization/PlutusData"
	"github.com/Salvionied/apollo/serialization/Redeemer"
	"github.com/Salvionied/apollo/serialization/TransactionInput"
	"github.com/Salvionied/apollo/serialization/TransactionOutput"
	"github.com/Salvionied/apollo/serialization/UTxO"
	"github.com/Salvionied/apollo/txBuilding/Backend/Base"
	"github.com/Salvionied/cbor/v2"
	"github.com/maestro-org/go-sdk/client"
	"github.com/maestro-org/go-sdk/models"
	"github.com/maestro-org/go-sdk/utils"
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

	maestroClient := client.NewClient(config.ProjectID, networkName)

	provider := &MaestroProvider{
		client:      maestroClient,
		networkName: networkName,
		networkId:   config.NetworkId,
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
	return int(resp.Data.EpochNo), nil
}

// GetProtocolParameters fetches the current protocol parameters from Maestro.
func (m *MaestroProvider) GetProtocolParameters(
	ctx context.Context,
) (Base.ProtocolParameters, error) {
	maestroParams, err := m.client.ProtocolParameters()
	if err != nil {
		return Base.ProtocolParameters{}, fmt.Errorf(
			"maestro: failed to get protocol parameters: %w",
			err,
		)
	}

	return adaptMaestroProtocolParams(maestroParams.Data), nil
}

// Maestro does not provide a genesis parameters endpoint.
func (m *MaestroProvider) GetGenesisParams(
	ctx context.Context,
) (Base.GenesisParameters, error) {
	return Base.GenesisParameters{}, fmt.Errorf(
		"maestro does not provide a genesis parameters endpoint: %w",
		connector.ErrNotImplemented,
	)
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
) ([]UTxO.UTxO, error) {
	utxos := make([]UTxO.UTxO, 0)
	params := utils.NewParameters()
	params.WithCbor()
	params.ResolveDatums()

	utxosAtAddressAtApi, err := m.client.UtxosAtAddress(addr, params)
	if err != nil {
		return utxos, err
	}

	for _, maestroUtxo := range utxosAtAddressAtApi.Data {
		utxo, err := adaptMaestroUtxoToApolloUtxo(maestroUtxo)
		if err != nil {
			return nil, err
		}
		utxos = append(utxos, utxo)
	}

	for utxosAtAddressAtApi.NextCursor != "" {
		params := utils.NewParameters()
		params.WithCbor()
		params.ResolveDatums()
		params.Cursor(utxosAtAddressAtApi.NextCursor)
		utxosAtAddressAtApi, err = m.client.UtxosAtAddress(addr, params)
		if err != nil {
			return utxos, err
		}
		for _, maestroUtxo := range utxosAtAddressAtApi.Data {
			utxo := UTxO.UTxO{}
			decodedHash, _ := hex.DecodeString(maestroUtxo.TxHash)
			utxo.Input = TransactionInput.TransactionInput{
				TransactionId: decodedHash,
				Index:         int(maestroUtxo.Index),
			}
			output := TransactionOutput.TransactionOutput{}
			decodedCbor, _ := hex.DecodeString(maestroUtxo.TxOutCbor)
			err = cbor.Unmarshal(decodedCbor, &output)
			if err != nil {
				return nil, err
			}
			utxo.Output = output
			utxos = append(utxos, utxo)
		}
	}

	return utxos, nil
}

// GetUtxosWithUnit fetches all UTxOs for a given address that contain a specific asset.
func (m *MaestroProvider) GetUtxosWithUnit(
	ctx context.Context,
	addr, unit string,
) ([]UTxO.UTxO, error) {
	utxos := make([]UTxO.UTxO, 0)
	params := utils.NewParameters()
	params.WithCbor()
	params.ResolveDatums()
	params.Asset(unit)

	utxosAtAddressAtApi, err := m.client.UtxosAtAddress(addr, params)
	if err != nil {
		return utxos, err
	}

	for _, maestroUtxo := range utxosAtAddressAtApi.Data {
		utxo, err := adaptMaestroUtxoToApolloUtxo(maestroUtxo)
		if err != nil {
			return nil, err
		}
		utxos = append(utxos, utxo)
	}

	for utxosAtAddressAtApi.NextCursor != "" {
		params := utils.NewParameters()
		params.WithCbor()
		params.ResolveDatums()
		params.Asset(unit)
		params.Cursor(utxosAtAddressAtApi.NextCursor)

		utxosAtAddressAtApi, err = m.client.UtxosAtAddress(addr, params)
		if err != nil {
			return utxos, err
		}
		for _, maestroUtxo := range utxosAtAddressAtApi.Data {
			utxo := UTxO.UTxO{}
			decodedHash, _ := hex.DecodeString(maestroUtxo.TxHash)
			utxo.Input = TransactionInput.TransactionInput{
				TransactionId: decodedHash,
				Index:         int(maestroUtxo.Index),
			}
			output := TransactionOutput.TransactionOutput{}
			decodedCbor, _ := hex.DecodeString(maestroUtxo.TxOutCbor)
			err = cbor.Unmarshal(decodedCbor, &output)
			if err != nil {
				return nil, err
			}
			utxo.Output = output
			utxos = append(utxos, utxo)
		}
	}

	return utxos, nil
}

// GetScriptCborByScriptHash fetches the CBOR of a script by its hash.
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

	return resp.Data.Bytes, nil
}

// GetUtxoByUnit finds the single UTxO containing a specific unit (NFT).
func (m *MaestroProvider) GetUtxoByUnit(
	ctx context.Context,
	unit string,
) (*UTxO.UTxO, error) {
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
) ([]UTxO.UTxO, error) {
	if len(outRefs) == 0 {
		return nil, nil
	}

	results := make([]UTxO.UTxO, 0, len(outRefs))

	params := utils.NewParameters()
	params.WithCbor()
	params.ResolveDatums()

	for _, ref := range outRefs {
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
		apolloUtxo, err := adaptMaestroUtxoToApolloUtxo(resp.Data)
		if err != nil {
			return nil, fmt.Errorf(
				"maestro: failed to adapt utxo %s#%d: %w",
				ref.TxHash,
				ref.Index,
				err,
			)
		}
		results = append(results, apolloUtxo)
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

// GetDatum fetches a datum by its hash.
func (m *MaestroProvider) GetDatum(
	ctx context.Context,
	datumHash string,
) (PlutusData.PlutusData, error) {
	resp, err := m.client.DatumFromHash(datumHash)
	if err != nil {
		return PlutusData.PlutusData{}, fmt.Errorf(
			"maestro: failed to get datum by hash %s: %w",
			datumHash,
			err,
		)
	}

	datumBytes, err := hex.DecodeString(resp.Data.Bytes)
	if err != nil {
		return PlutusData.PlutusData{}, fmt.Errorf(
			"invalid datum cbor hex from maestro: %w",
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

// SubmitTx submits a signed transaction.
func (m *MaestroProvider) SubmitTx(
	ctx context.Context,
	txBytes []byte,
) (string, error) {
	txHex := hex.EncodeToString(txBytes)

	resp, err := m.client.SubmitTx(txHex)
	if err != nil {
		return "", fmt.Errorf("maestro: tx submission failed: %w", err)
	}

	if resp.Data == "" {
		return "", errors.New(
			"maestro did not return a transaction hash on submission",
		)
	}
	return resp.Data, nil
}

// EvaluateTx evaluates a transaction's scripts.
func (m *MaestroProvider) EvaluateTx(
	ctx context.Context,
	txBytes []byte,
	additional []UTxO.UTxO,
) (map[string]Redeemer.ExecutionUnits, error) {
	results := make(map[string]Redeemer.ExecutionUnits)
	txHex := hex.EncodeToString(txBytes)

	var additionalUtxos []models.AdditionalUtxo
	if len(additional) > 0 {
		maestroExtras, err := adaptApolloUtxosToMaestro(additional)
		if err != nil {
			return nil, fmt.Errorf("failed to adapt additional UTxOs: %w", err)
		}
		additionalUtxos = maestroExtras
	}

	evaluation, err := m.client.EvaluateTx(txHex, additionalUtxos...)
	if err != nil {
		return nil, err
	}
	for _, eval := range evaluation {
		results[eval.RedeemerTag+":"+strconv.Itoa(eval.RedeemerIndex)] = Redeemer.ExecutionUnits{
			Mem:   eval.ExUnits.Mem,
			Steps: eval.ExUnits.Steps,
		}
	}
	return results, nil
}
