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

# Cross-compile for Windows (Sentry)
build-windows:
	GOOS=windows GOARCH=amd64 go build -o sentry.exe client/main.go