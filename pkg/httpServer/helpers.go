package httpServer

import (
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mytonstorage-backend/pkg/models"
)

func (h *handler) limitReached(c *fiber.Ctx) error {
	log := h.logger.With(
		slog.String("method", "limitReached"),
		slog.String("method", c.Method()),
		slog.String("url", c.OriginalURL()),
		slog.Any("headers", c.GetReqHeaders()),
	)

	log.Warn("rate limit reached for request")
	return fiber.NewError(fiber.StatusTooManyRequests, models.ErrMsgTooManyRequests)
}

func validateBagID(bagid string) bool {
	if len(bagid) != 64 {
		return false
	}

	bagid = strings.ToLower(bagid)
	for i := range 64 {
		c := bagid[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}

	return true
}

func okHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "ok",
	})
}

// ErrorHandler writes a safe JSON error response. Unknown errors never leak internal details.
func ErrorHandler(c *fiber.Ctx, err error) error {
	if e, ok := err.(*fiber.Error); ok {
		msg := e.Message
		if msg == "" {
			msg = models.ErrMsgInternalServerError
		}
		return c.Status(e.Code).JSON(fiber.Map{
			"error": msg,
		})
	}

	if appErr, ok := err.(*models.AppError); ok {
		msg := appErr.Message
		if msg == "" {
			msg = models.ErrMsgInternalServerError
		}
		return c.Status(appErr.Code).JSON(fiber.Map{
			"error": msg,
		})
	}

	return c.Status(fiber.StatusInternalServerError).JSON(errorResponse{
		Error: models.ErrMsgInternalServerError,
	})
}
