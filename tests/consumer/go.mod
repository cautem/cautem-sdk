module github.com/cautem/cautem-sdk/tests/consumer

go 1.27.0

toolchain go1.27.2

require github.com/cautem/cautem-sdk v0.0.0

require (
	github.com/NVIDIA/OpenShell/sdk/go v0.0.0-20261013191600-5a7728c304b5 // indirect
	github.com/cautem/cautem-proto v0.2.0 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260706201446-f0a921348800 // indirect
	google.golang.org/grpc v1.84.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

replace github.com/cautem/cautem-sdk => ../..
