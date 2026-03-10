package maestro

import (
	"sync"

	"github.com/Salvionied/apollo/txBuilding/Backend/Base"
	"github.com/maestro-org/go-sdk/client"
)

type Config struct {
	// ProjectID is the API key for authenticating with the Maestro API.
	ProjectID string

	// NetworkName is the name of the Cardano network (e.g., "mainnet", "preprod", "preview").
	NetworkName string

	// NetworkId is the numeric identifier for the Cardano network (e.g., 1 for Preprod, 0 for Mainnet).
	NetworkId int

	// ProtocolParamsOverride overrides both live Maestro protocol parameters and the built-in preset.
	ProtocolParamsOverride *Base.ProtocolParameters

	// GenesisParamsOverride overrides the built-in per-network genesis preset.
	GenesisParamsOverride *Base.GenesisParameters
}

// MaestroProvider implements the connector.Provider interface for the Maestro API.
type MaestroProvider struct {
	client                 *client.Client
	genesisParams          Base.GenesisParameters
	protocolParamsOverride *Base.ProtocolParameters
	protocolParamsPreset   Base.ProtocolParameters
	networkId              int
	networkName            string

	// rawCborCache preserves the original txout_cbor hex strings returned by
	// Maestro so they can be passed back verbatim during EvaluateTx. Apollo's
	// CBOR marshaling does not guarantee byte-identical output, and Maestro
	// rejects re-encoded UTxOs as "Malformed additional UTxO".
	// Key format: "txhash#index"
	rawCborCache sync.Map
}
