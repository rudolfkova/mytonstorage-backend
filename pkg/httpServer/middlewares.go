package httpServer

import (
	"crypto/md5"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mytonstorage-backend/pkg/models"
)

func (h *handler) userAuthMiddleware(c *fiber.Ctx) error {
	cookie := c.Cookies("session_id")
	parts := strings.SplitN(cookie, ":", 2)
	if len(parts) != 2 {
		return ErrorHandler(c, fiber.NewError(fiber.StatusUnauthorized, models.ErrMsgUnauthorized))
	}

	signature, sessionData := parts[0], parts[1]

	addr, err := h.auth.Authenticate(c.Context(), signature, sessionData)
	if err != nil {
		return ErrorHandler(c, fiber.NewError(fiber.StatusUnauthorized, models.ErrMsgUnauthorized))
	}

	c.Context().SetUserValue("address", addr)

	return c.Next()
}

func (h *handler) adminAuthMiddleware(c *fiber.Ctx) error {
	accessToken := c.Get("Authorization")
	if accessToken == "" {
		return ErrorHandler(c, fiber.NewError(fiber.StatusUnauthorized, models.ErrMsgUnauthorized))
	}

	if strings.HasPrefix(strings.ToLower(accessToken), "bearer ") {
		accessToken = accessToken[7:]
	}

	hash := md5.Sum([]byte(accessToken))
	tokenHash := fmt.Sprintf("%x", hash[:])

	if _, exists := h.adminAuthTokens[tokenHash]; !exists {
		return ErrorHandler(c, fiber.NewError(fiber.StatusForbidden, models.ErrMsgForbidden))
	}

	return c.Next()
}

func (h *handler) loggerMiddleware(c *fiber.Ctx) error {
	headers := c.GetReqHeaders()
	if _, ok := headers["Authorization"]; ok {
		headers["Authorization"] = []string{"REDACTED"}
	}

	if _, ok := headers["Cookie"]; ok {
		headers["Cookie"] = []string{"REDACTED"}
	}

	res := c.Next()

	h.logger.Debug(
		"request",
		"status_code", c.Response().StatusCode(),
		"method", c.Method(),
		"url", c.OriginalURL(),
		"headers", headers,
		"body_length", len(c.Body()),
	)

	return res
}
