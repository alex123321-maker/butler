package generatedtools

import (
	"fmt"

	"github.com/butler/butler/internal/actionauth"
	converterv1 "github.com/butler/butler/internal/gen/converter/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const UnixSocket = "/run/butler-converter/runtime.sock"
const RuntimeTarget = "unix://" + UnixSocket

// A no-network runtime uses only this fixed socket volume. Neither the model
// nor a generated package can select a target, token, transport or another RPC.
func NewRuntimeClient(target, token string) (converterv1.ConverterRuntimeServiceClient, *grpc.ClientConn, error) {
	if target != RuntimeTarget || len(token) < 32 {
		return nil, nil, fmt.Errorf("fixed private converter socket and service authentication required")
	}
	c, e := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithPerRPCCredentials(actionauth.ServiceTokenCredentials{Token: token}), grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(192*1024), grpc.MaxCallRecvMsgSize(512*1024)))
	if e != nil {
		return nil, nil, fmt.Errorf("private converter client unavailable")
	}
	return converterv1.NewConverterRuntimeServiceClient(c), c, nil
}
