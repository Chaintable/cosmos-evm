package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"cosmossdk.io/log"
	pruningtypes "cosmossdk.io/store/pruning/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/evm/rpc/backend"
	cosmosevmserverconfig "github.com/cosmos/evm/server/config"
	cosmosevmtypes "github.com/cosmos/evm/types"

	"go.etcd.io/etcd/client/v3"
	"golang.org/x/sync/errgroup"
)

type EtcdRegisterConfig struct {
	Endpoints       []string `json:"endpoints"`
	LeaseTTLSeconds int64    `json:"lease_ttl_s"`
	Meta            string   `json:"meta"`
}

type NodeInfo struct {
	StateType uint64 `json:"stateType"`
	Address   string `json:"address"`
	Port      uint64 `json:"port"`
	NodeType  uint64 `json:"nodeType"`
}

const (
	StateTypeLatest  uint64 = 1
	StateTypeDelay   uint64 = 2
	StateTypeOffline uint64 = 3
)

const (
	NodeTypeState   uint64 = 1
	NodeTypeArchive uint64 = 2
)

type Register struct {
	etcdCfg    EtcdRegisterConfig
	etcdClient *clientv3.Client
	key        string
	value      string
	logger     log.Logger

	leaseID clientv3.LeaseID
}

func NewRegister(chainID uint64, etcdCfg EtcdRegisterConfig, isArchive bool, logger log.Logger) (*Register, error) {
	logger.Info("Initialize etcd register", "config", etcdCfg)
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   etcdCfg.Endpoints,
		DialTimeout: 2 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("etcd connect failed: %w", err)
	}

	if etcdCfg.Meta == "" {
		return nil, errors.New("meta is empty")
	}
	ipHost := strings.Split(etcdCfg.Meta, ":")
	if len(ipHost) != 2 {
		return nil, errors.New("meta format error (expected ip:port)")
	}
	ip := ipHost[0]
	port, err := strconv.ParseUint(ipHost[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid port in meta: %w", err)
	}

	key := fmt.Sprintf("%d/nativeNodes/%s_%d", chainID, ip, port)

	nodeType := NodeTypeState
	if isArchive {
		nodeType = NodeTypeArchive
	}
	nodeInfo := NodeInfo{
		StateType: StateTypeDelay,
		Address:   ip,
		Port:      port,
		NodeType:  nodeType,
	}
	valueBytes, err := json.Marshal(nodeInfo)
	if err != nil {
		return nil, fmt.Errorf("marshal node info failed: %w", err)
	}

	return &Register{
		etcdCfg:    etcdCfg,
		etcdClient: cli,
		key:        key,
		value:      string(valueBytes),
		logger:     logger,
	}, nil
}

func (r *Register) Register(ctx context.Context) error {
	lease, err := r.etcdClient.Grant(ctx, r.etcdCfg.LeaseTTLSeconds)
	if err != nil {
		return fmt.Errorf("grant lease failed: %w", err)
	}

	_, err = r.etcdClient.Put(
		ctx,
		r.key,
		r.value,
		clientv3.WithLease(lease.ID),
	)
	if err != nil {
		return fmt.Errorf("put key with lease failed: %w", err)
	}

	r.leaseID = lease.ID
	r.logger.Info(
		"Etcd register success",
		"key", r.key,
		"leaseID", lease.ID,
		"ttl", r.etcdCfg.LeaseTTLSeconds,
	)
	return nil
}

func (r *Register) keepAlive(ctx context.Context) error {
	ch, err := r.etcdClient.KeepAlive(ctx, r.leaseID)
	if err != nil {
		r.logger.Error("Etcd keepalive start failed", "error", err)
		return fmt.Errorf("keepalive start failed: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil

		case resp, ok := <-ch:
			if !ok {
				return fmt.Errorf("keepalive channel closed, lease lost")
			}

			r.logger.Debug(
				"Etcd keepalive success",
				"key", r.key,
				"ttl", resp.TTL,
			)
		}
	}
}

func (r *Register) Unregister(ctx context.Context) error {
	if r.leaseID == 0 {
		return nil
	}
	_, err := r.etcdClient.Revoke(ctx, r.leaseID)
	if err != nil {
		return fmt.Errorf("revoke lease failed: %w", err)
	}
	r.logger.Info("Etcd unregister success", "key", r.key)
	return nil
}

func (r *Register) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for {
		if err := r.Register(runCtx); err != nil {
			r.logger.Error("Etcd register failed, retrying...", "error", err)
			time.Sleep(time.Second)
			continue
		}

		if err := r.keepAlive(runCtx); err != nil {
			r.logger.Warn("KeepAlive failed, re-registering...", "error", err)
			time.Sleep(time.Second)
			continue
		}

		select {
		case <-ctx.Done():
			r.logger.Info("Stopping Etcd register service")
			if err := r.Unregister(context.Background()); err != nil {
				r.logger.Error("Failed to unregister", "error", err)
			}
			return nil
		default:
		}
	}
}

func StartEtcdRegister(
	ctx *server.Context,
	context context.Context,
	clientCtx client.Context,
	g *errgroup.Group,
	config cosmosevmserverconfig.Config,
	indexer cosmosevmtypes.EVMTxIndexer,
) error {
	if !config.JSONRPC.Enable || len(config.ETCDConfig) == 0 {
		return nil
	}
	allowUnprotectedTxs := config.JSONRPC.AllowUnprotectedTxs
	evmBackend := backend.NewBackend(ctx, ctx.Logger, clientCtx, allowUnprotectedTxs, indexer)
	chainId, err := evmBackend.ChainID()
	if err != nil {
		return fmt.Errorf("get chain id failed: %w", err)
	}
	var etcdCfg EtcdRegisterConfig
	if err = json.Unmarshal([]byte(config.ETCDConfig), &etcdCfg); err != nil {
		return fmt.Errorf("unmarshal etcd config failed: %+v", err)
	}
	var isArchive = false
	if config.Config.BaseConfig.Pruning == pruningtypes.PruningOptionNothing {
		isArchive = true
	}
	register, err := NewRegister(chainId.ToInt().Uint64(), etcdCfg, isArchive, ctx.Logger.With("module", "etcd-register"))
	if err != nil {
		return err
	}
	g.Go(func() error {
		return register.Start(context)
	})
	return nil
}
