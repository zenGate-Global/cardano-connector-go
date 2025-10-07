package maestro

import "github.com/maestro-org/go-sdk/client"

type Config struct {
	// ProjectID is the API key for authenticating with the Maestro API.
	ProjectID string

	// NetworkName is the name of the Cardano network (e.g., "mainnet", "preprod", "preview").
	NetworkName string

	// NetworkId is the numeric identifier for the Cardano network (e.g., 1 for Preprod, 0 for Mainnet).
	NetworkId int
}

// MaestroProvider implements the connector.Provider interface for the Maestro API.
type MaestroProvider struct {
	client      *client.Client
	networkId   int
	networkName string
}
