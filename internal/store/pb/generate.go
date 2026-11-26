// Package pb is the generated gRPC store API.
//
//go:generate protoc --proto_path=../../../api --go_out=../../.. --go_opt=module=todo-service --go-grpc_out=../../.. --go-grpc_opt=module=todo-service store/v1/store.proto
package pb
