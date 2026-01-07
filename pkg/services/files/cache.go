package files

import (
	"context"
	"mime/multipart"
	"time"

	"mytonstorage-backend/pkg/cache"
	v1 "mytonstorage-backend/pkg/models/api/v1"
)

type cacheMiddleware struct {
	svc   Files
	cache *cache.SimpleCache
}

func (c *cacheMiddleware) AddFiles(ctx context.Context, mr *multipart.Reader, size uint64, userAddr string) (info v1.UnpaidBagsResponse, err error) {
	return c.svc.AddFiles(ctx, mr, size, userAddr)
}

func (c *cacheMiddleware) DeleteBag(ctx context.Context, bagID string, userAddr string) (err error) {
	err = c.svc.DeleteBag(ctx, bagID, userAddr)
	if err != nil {
		return
	}

	key := "bag_info:" + bagID
	c.cache.Release(key)

	return
}

func (c *cacheMiddleware) MarkBagAsPaid(ctx context.Context, bagID string, userAddr string, storageContract string) error {
	return c.svc.MarkBagAsPaid(ctx, bagID, userAddr, storageContract)
}

func (c *cacheMiddleware) GetUnpaidBags(ctx context.Context, userAddr string) (info v1.UnpaidBagsResponse, err error) {
	return c.svc.GetUnpaidBags(ctx, userAddr)
}

func (c *cacheMiddleware) GetBagsInfoShort(ctx context.Context, contracts []string) (info []v1.BagInfoShort, err error) {
	return c.svc.GetBagsInfoShort(ctx, contracts)
}

func NewCacheMiddleware(
	svc Files,
) Files {
	return &cacheMiddleware{
		svc:   svc,
		cache: cache.NewSimpleCache(12 * time.Hour),
	}
}
