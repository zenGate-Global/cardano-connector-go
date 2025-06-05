module github.com/zenGate-Global/cardano-connector-go

go 1.24.0

require (
	github.com/Salvionied/apollo v1.1.1
	github.com/Salvionied/cbor/v2 v2.6.0
	github.com/SundaeSwap-finance/kugo v1.2.0
	github.com/SundaeSwap-finance/ogmigo/v6 v6.0.1
	github.com/stretchr/testify v1.8.1
	github.com/tj/assert v0.0.3
	github.com/utxorpc/go-codegen v0.16.0
	github.com/utxorpc/go-sdk v0.0.0-20250528145820-748c177d7090
)

require (
	connectrpc.com/connect v1.18.1 // indirect
	github.com/aws/aws-sdk-go v1.55.6 // indirect
	github.com/btcsuite/btcutil v1.0.2 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fxamacker/cbor/v2 v2.8.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/crypto v0.38.0 // indirect
	golang.org/x/net v0.40.0 // indirect
	golang.org/x/sync v0.14.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.25.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/SundaeSwap-finance/ogmigo/v6 => github.com/mgpai22/ogmigo/v6 v6.0.4

replace github.com/SundaeSwap-finance/kugo => github.com/mgpai22/kugo v1.2.1

replace github.com/utxorpc/go-sdk => github.com/mgpai22/go-sdk v0.0.3
