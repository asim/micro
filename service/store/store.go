package store

import (
	"github.com/micro/cli/v2"
	"github.com/micro/go-micro/v2"
	log "github.com/micro/go-micro/v2/logger"
	pb "github.com/micro/go-micro/v2/store/service/proto"
	"github.com/micro/micro/v2/service/store/handler"
)

var (
	// name of the store service
	name = "go.micro.store"
	// address is the store address
	address = ":8002"
)

// Run runs the micro server
func Run(ctx *cli.Context, srvOpts ...micro.Option) {
	log.Init(log.WithFields(map[string]interface{}{"service": "store"}))

	if len(ctx.String("server_name")) > 0 {
		name = ctx.String("server_name")
	}
	if len(ctx.String("address")) > 0 {
		address = ctx.String("address")
	}

	// Initialise service
	service := micro.NewService(
		micro.Name(name),
		micro.Address(address),
	)

	// the store handler
	h := handler.New(service.Options().Store)
	pb.RegisterStoreHandler(service.Server(), h)

	// start the service
	if err := service.Run(); err != nil {
		log.Fatal(err)
	}
}
