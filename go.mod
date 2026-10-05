module github.com/zenGate-Global/cardano-connector-go

go 1.26.5

require (
	github.com/SundaeSwap-finance/kugo v1.3.1
	github.com/SundaeSwap-finance/ogmigo/v6 v6.2.1
	github.com/blinklabs-io/gouroboros v0.208.5
	github.com/blinklabs-io/plutigo v0.7.2
	github.com/gorilla/websocket v1.5.3
	github.com/maestro-org/go-sdk v1.2.1
	github.com/stretchr/testify v1.12.1
	github.com/tj/assert v0.0.3
	github.com/utxorpc/go-codegen v0.19.2
	github.com/utxorpc/go-sdk v0.1.0
)

require go.yaml.in/yaml/v3 v3.0.5 // indirect

require (
	connectrpc.com/connect v1.20.0
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/Salvionied/apollo/v2 v2.0.0
	github.com/aws/aws-sdk-go v1.55.6 // indirect
	github.com/bits-and-blooms/bitset v1.24.6 // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.5.0 // indirect
	github.com/btcsuite/btcd/btcutil v1.2.0 // indirect
	github.com/btcsuite/btcd/chaincfg/chainhash v1.2.0 // indirect
	github.com/btcsuite/btcd/chainhash/v2 v2.0.0 // indirect
	github.com/btcsuite/btcutil v1.0.2 // indirect
	github.com/buger/jsonparser v1.1.2 // indirect
	github.com/consensys/gnark-crypto v0.21.0 // indirect
	github.com/decred/dcrd/crypto/blake256 v1.1.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.4 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.3 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

// replace github.com/SundaeSwap-finance/ogmigo/v6 => github.com/mgpai22/ogmigo/v6 v6.0.4

// replace github.com/SundaeSwap-finance/kugo => github.com/mgpai22/kugo v1.2.1

// replace github.com/utxorpc/go-sdk => github.com/mgpai22/go-sdk v0.0.3

replace github.com/maestro-org/go-sdk => github.com/mgpai22/maestro-cardano-go-sdk v1.4.2

replace github.com/Salvionied/apollo/v2 => github.com/zenGate-Global/apollo/v2 v2.0.0-20260624043416-6d27a5261d3b
