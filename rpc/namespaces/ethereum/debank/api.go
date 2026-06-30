package debank

import (
	"cosmossdk.io/log"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/evm/rpc/backend"
	rpctypes "github.com/cosmos/evm/rpc/types"
)

type API struct {
	ctx         *server.Context
	logger      log.Logger
	backend     backend.EVMBackend
	clientCtx   client.Context
	queryClient *rpctypes.QueryClient
}

func NewAPI(
	ctx *server.Context,
	backend backend.EVMBackend,
	clientCtx client.Context,
) *API {
	return &API{
		ctx:         ctx,
		logger:      ctx.Logger.With("module", "debank"),
		backend:     backend,
		clientCtx:   clientCtx,
		queryClient: rpctypes.NewQueryClient(clientCtx),
	}
}
