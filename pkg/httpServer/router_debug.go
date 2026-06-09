// !ONLY FOR DEBUG PURPOSES
//
//go:build debug

package httpServer

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

const (
	MaxRequests     = 30
	RateLimitWindow = 60 * time.Second
)

func (h *handler) RegisterRoutes() {
	h.logger.Info("Registering debug routes")

	// On server side nginx or other reverse proxy should handle CORS
	// and OPTIONS requests, but for debug purposes we handle it here.
	h.server.Use(func(c *fiber.Ctx) error {
		origin := c.Get("Origin")
		switch origin {
		case "http://localhost:3000", "http://127.0.0.1:3000":
			c.Set("Access-Control-Allow-Origin", origin)
		}
		c.Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		requestedHeaders := c.Get("Access-Control-Request-Headers")
		if requestedHeaders != "" {
			c.Set("Access-Control-Allow-Headers", requestedHeaders)
		} else {
			c.Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-Requested-With")
		}

		c.Set("Access-Control-Allow-Credentials", "true")

		if c.Method() == fiber.MethodOptions {
			return c.SendStatus(fiber.StatusOK)
		}
		return c.Next()
	})

	m := newMetrics(h.namespace, h.subsystem)

	h.server.Use(m.metricsMiddleware)

	h.server.Use(limiter.New(limiter.Config{
		Max:               MaxRequests,
		Expiration:        RateLimitWindow,
		LimitReached:      h.limitReached,
		LimiterMiddleware: limiter.SlidingWindow{},
	}))

	h.server.Get("/health", h.health)
	h.server.Get("/metrics", h.adminAuthMiddleware, h.metrics)

	apiv1 := h.server.Group("/api/v1", h.loggerMiddleware)
	{
		{
			auth := apiv1.Group("")
			auth.Post("/login", h.login)
			auth.Get("/ton-proof", h.getData)
		}

		{
			files := apiv1.Group("/files", h.userAuthMiddleware)
			files.Post("/", h.uploadFiles)
			files.Post("/paid", h.markBagAsPaid)
			files.Post("/details", h.GetBagsInfoShort)
			files.Post("/unpaid", h.getUnpaid)
			files.Delete("/:bag_id", h.deleteBag)
		}

		{
			contracts := apiv1.Group("/contracts", h.userAuthMiddleware)
			contracts.Post("/init-contract", h.initStorageContract)
			contracts.Post("/topup", h.topupBalance)
			contracts.Post("/withdraw", h.withdrawBalance)
			contracts.Post("/update", h.updateProviders)
		}

		{
			providers := apiv1.Group("/providers", h.userAuthMiddleware)
			providers.Post("/offers", h.fetchProvidersOffers)
		}
	}
}
