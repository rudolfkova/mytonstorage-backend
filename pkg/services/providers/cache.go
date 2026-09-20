package providers

import (
	"context"
	"fmt"
	"time"

	"mytonstorage-backend/pkg/cache"
	v1 "mytonstorage-backend/pkg/models/api/v1"
)

type providersCache struct {
	cache *cache.SimpleCache
	svc   Providers
}

func (c *providersCache) FetchProvidersRates(ctx context.Context, req v1.OffersRequest) (resp v1.ProviderRatesResponse, err error) {
	return c.svc.FetchProvidersRates(ctx, req)
}

func (c *providersCache) FetchProvidersRatesBySize(ctx context.Context, providers []string, bagSize uint64, span uint32) (resp v1.ProviderRatesResponse) {
	return c.svc.FetchProvidersRatesBySize(ctx, providers, bagSize, span)
}

func (c *providersCache) InitStorageContract(ctx context.Context, info v1.InitStorageContractRequest, providers []v1.ProviderShort) (resp v1.Transaction, err error) {
	return c.svc.InitStorageContract(ctx, info, providers)
}

func (c *providersCache) EditStorageContract(ctx context.Context, address string, amount uint64, providers []v1.ProviderShort) (resp v1.Transaction, err error) {
	return c.svc.EditStorageContract(ctx, address, amount, providers)
}

func (c *providersCache) NotifyProviders(ctx context.Context, userAddress, contractAddr string, providers []string) error {
	return c.svc.NotifyProviders(ctx, userAddress, contractAddr, providers)
}

func (c *providersCache) fetchProviderRates(ctx context.Context, providerKey string, bagSize uint64, span uint32) (offer *v1.ProviderOffer, reason string) {
	key := fmt.Sprintf("pr_%s_%d_%d", providerKey, bagSize, span)
	if cached, ok := c.cache.Get(key); ok {
		if offer, ok = cached.(*v1.ProviderOffer); ok {
			return
		}
	}

	offer, reason = c.svc.fetchProviderRates(ctx, providerKey, bagSize, span)
	if offer != nil && reason == "" {
		c.cache.Set(key, offer)
	}

	return
}

func NewProvidersCache(svc Providers) Providers {
	return &providersCache{
		cache: cache.NewSimpleCache(1 * time.Hour),
		svc:   svc,
	}
}
