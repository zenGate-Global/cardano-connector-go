package connector

import (
	"context"
	"time"

	"github.com/Salvionied/apollo/serialization/PlutusData"
	"github.com/Salvionied/apollo/serialization/Redeemer"
	"github.com/Salvionied/apollo/serialization/UTxO"
	"github.com/Salvionied/apollo/txBuilding/Backend/Base"
)

type OutRef struct {
	TxHash string `json:"tx_hash"`
	Index  uint32 `json:"index"`
}

type Delegation struct {
	Active  bool   `json:"active"`
	Rewards uint64 `json:"rewards"`
	PoolId  string `json:"pool_id"`
	Epoch   int    `json:"epoch,omitempty"`
}

type EvalRedeemer struct {
	Tag     Redeemer.RedeemerTag    `json:"tag"`
	Index   uint32                  `json:"index"`
	ExUnits Redeemer.ExecutionUnits `json:"ex_units"`
}

type Tip struct {
	Slot   uint64 `json:"slot"`
	Height uint64 `json:"height"`
	Hash   string `json:"hash"`
}

type Provider interface {
	// GetProtocolParameters fetches the current protocol parameters.
	GetProtocolParameters(ctx context.Context) (Base.ProtocolParameters, error)

	// GetGenesisParams fetches the genesis parameters.
	GetGenesisParams(ctx context.Context) (Base.GenesisParameters, error)

	// Network returns the network id.
	Network() int

	// Epoch returns the current epoch.
	Epoch(ctx context.Context) (int, error)

	// GetTip fetches the current tip of the blockchain.
	GetTip(ctx context.Context) (Tip, error)

	// GetUtxosByAddress queries UTxOs by a Bech32 address.
	GetUtxosByAddress(ctx context.Context, addr string) ([]UTxO.UTxO, error)

	// GetUtxosWithUnit queries UTxOs by address, filtered by a specific asset unit.
	GetUtxosWithUnit(
		ctx context.Context,
		addr string,
		unit string,
	) ([]UTxO.UTxO, error)

	// GetUtxoByUnit queries a UTxO by a specific unit (NFT or fungible token if entire supply is in one UTxO).
	// Returns (nil, nil) if not found but no other error occurred.
	GetUtxoByUnit(ctx context.Context, unit string) (*UTxO.UTxO, error)

	// GetUtxosByOutRef queries UTxOs by their output references.
	GetUtxosByOutRef(ctx context.Context, outRefs []OutRef) ([]UTxO.UTxO, error)

	// GetDelegation fetches delegation information for a reward address.
	GetDelegation(
		ctx context.Context,
		rewardAddress string,
	) (Delegation, error)

	// GetDatum fetches a datum by its hash. Returns the datum as PlutusData.
	GetDatum(
		ctx context.Context,
		datumHash string,
	) (PlutusData.PlutusData, error)

	// AwaitTx waits for a transaction to be confirmed on the blockchain.
	// checkInterval specifies how often to check (e.g., 5*time.Second).
	// A zero or negative duration might use a provider-specific default.
	AwaitTx(
		ctx context.Context,
		txHash string,
		checkInterval time.Duration,
	) (bool, error)

	// SubmitTx submits a signed transaction to the network.
	SubmitTx(ctx context.Context, tx []byte) (string, error)

	// EvaluateTx evaluates a transaction's scripts and returns the execution units.
	// additionalUTxOs can be provided for inputs not yet on-chain.
	EvaluateTx(
		ctx context.Context,
		tx []byte,
		additionalUTxOs []UTxO.UTxO,
	) (map[string]Redeemer.ExecutionUnits, error)

	// GetScriptCborByScriptHash fetches the CBOR representation of a script by its hash.
	GetScriptCborByScriptHash(
		ctx context.Context,
		scriptHash string,
	) (string, error)
}
