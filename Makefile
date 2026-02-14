.PHONY: proto clean server client

# The magic command for generating gRPC code
proto:
	protoc --go_out=. --go_opt=module=github.com/Mohammad-Mahdi82/NexusOps \
	       --go-grpc_out=. --go-grpc_opt=module=github.com/Mohammad-Mahdi82/NexusOps \
	       proto/monitor.proto

# Clean up generated files
clean:
	rm -rf pkg/monitor/*.go

# Run the server
run-server:
	go run server/main.go

# Run the client
run-client:
	go run client/main.go

# Build the client
build-client:
	go build -ldflags="-H windowsgui" -o Sentry.exe ./client

# Build the server for raspberrypi
build-server-raspberry:
	set GOOS=linux&& set GOARCH=arm64&& go build -o nexus-server ./server