package customfragment

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/ton"
	"github.com/xssnick/tonutils-go/ton/nft"
)

const defaultMainnetConfigURL = "https://ton-blockchain.github.io/global.config.json"

type MainnetVerifier struct {
	configURL string
	mu        sync.Mutex
	pool      *liteclient.ConnectionPool
	api       ton.APIClientWrapped
}

func NewMainnetVerifier(configURL string) *MainnetVerifier {
	configURL = strings.TrimSpace(configURL)
	if configURL == "" {
		configURL = defaultMainnetConfigURL
	}
	return &MainnetVerifier{configURL: configURL}
}

func (v *MainnetVerifier) VerifyMint(ctx context.Context, collectionRaw string, itemIndex *big.Int, ownerRaw string) (string, error) {
	if itemIndex == nil || itemIndex.Sign() < 0 {
		return "", fmt.Errorf("invalid item index")
	}
	collection, err := parseMainnetAddress(collectionRaw)
	if err != nil {
		return "", err
	}
	owner, err := parseMainnetAddress(ownerRaw)
	if err != nil {
		return "", err
	}
	api, err := v.client(ctx)
	if err != nil {
		return "", err
	}
	itemAddress, err := nft.NewCollectionClient(api, collection).GetNFTAddressByIndex(ctx, itemIndex)
	if err != nil {
		return "", fmt.Errorf("resolve NFT address: %w", err)
	}
	data, err := nft.NewItemClient(api, itemAddress).GetNFTData(ctx)
	if err != nil {
		return "", fmt.Errorf("read NFT data: %w", err)
	}
	if !data.Initialized || data.Index == nil || data.Index.Cmp(itemIndex) != 0 ||
		data.CollectionAddress == nil || !data.CollectionAddress.Equals(collection) ||
		data.OwnerAddress == nil || !data.OwnerAddress.Equals(owner) {
		return "", fmt.Errorf("NFT owner, collection or index does not match authorization")
	}
	return itemAddress.StringRaw(), nil
}

func (v *MainnetVerifier) client(ctx context.Context) (ton.APIClientWrapped, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.api != nil {
		return v.api, nil
	}
	pool := liteclient.NewConnectionPool()
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.AddConnectionsFromConfigUrl(connectCtx, v.configURL); err != nil {
		pool.Stop()
		return nil, fmt.Errorf("connect mainnet lite servers: %w", err)
	}
	v.pool = pool
	v.api = ton.NewAPIClient(pool, ton.ProofCheckPolicyFast).WithRetryTimeout(2, 4*time.Second)
	return v.api, nil
}

func (v *MainnetVerifier) Close() {
	if v == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.pool != nil {
		v.pool.Stop()
	}
	v.pool, v.api = nil, nil
}

var _ Verifier = (*MainnetVerifier)(nil)
