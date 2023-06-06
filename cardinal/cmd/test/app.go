package main

import (
	"context"
	"fmt"
	"net"
	"os"

	"buf.build/gen/go/argus-labs/cardinal/grpc/go/ecs/ecsv1grpc"
	"google.golang.org/grpc"

	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	"github.com/argus-labs/world-engine/cardinal/net/server"
)

/*
testing application.
*/

func main() {
	addr := os.Getenv("grpc_addr")
	fmt.Println("starting app...")
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	fmt.Println("listener established")
	rs := storage.NewRedisStorage(storage.Options{
		Addr: os.Getenv("redis_addr"),
	}, "0")
	ctx := context.Background()
	res := rs.Client.Ping(ctx)
	if err := res.Err(); err != nil {
		panic(err)
	} else {
		fmt.Println("connection to redis established")
	}
	worldStorage := storage.NewWorldStorage(
		storage.Components{Store: &rs, ComponentIndices: &rs}, &rs, storage.NewArchetypeComponentIndex(), storage.NewArchetypeAccessor(), &rs, &rs, &rs)
	gs := server.NewGameServer(worldStorage)
	grpcServer := grpc.NewServer()
	ecsv1grpc.RegisterGameServer(grpcServer, gs)
	fmt.Println("serving application...")
	localErr := grpcServer.Serve(lis)
	if localErr != nil {
		fmt.Println("failed to serve: ", err)
		os.Exit(1)
	}
}
