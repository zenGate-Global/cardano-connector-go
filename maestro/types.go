package maestro

import (
	"github.com/Salvionied/apollo/v2/backend"
	maestroClient "github.com/maestro-org/go-sdk/client"
)

type Config struct {
	// ProjectID is the API key for authenticating with the Maestro API.
	ProjectID string

	// NetworkName is the name of the Cardano network (e.g., "mainnet", "preprod", "preview").
	NetworkName string

	// NetworkId is the numeric identifier for the Cardano network (e.g., 1 for Preprod, 0 for Mainnet).
	NetworkId int

	// ProtocolParamsOverride overrides both live Maestro protocol parameters and the built-in preset.
	ProtocolParamsOverride *backend.ProtocolParameters

	// GenesisParamsOverride overrides the built-in per-network genesis preset.
	GenesisParamsOverride *backend.GenesisParameters
}

// MaestroProvider implements the connector.Provider interface for the Maestro API.
type MaestroProvider struct {
	client                 *maestroClient.Client
	projectID              string
	genesisParams          backend.GenesisParameters
	protocolParamsOverride *backend.ProtocolParameters
	protocolParamsPreset   backend.ProtocolParameters
	networkId              int
	networkName            string
}
