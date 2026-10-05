package httpServer

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	v1 "mytonstorage-backend/pkg/models/api/v1"
)

type stubAuth struct {
	sessionID string
}

func (s stubAuth) GetData() string { return "" }

func (s stubAuth) Login(context.Context, v1.LoginInfo) (string, error) {
	return s.sessionID, nil
}

func (s stubAuth) Authenticate(context.Context, string, string) (string, error) {
	return "", nil
}

func TestLoginSetsSessionCookieMaxAge(t *testing.T) {
	app := fiber.New()
	h := &handler{
		auth:   stubAuth{sessionID: "sig:1:EQ"},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	app.Post("/api/v1/login", h.login)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}

	var session *http.Cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "session_id" {
			session = cookie
			break
		}
	}
	if session == nil {
		t.Fatal("session_id cookie not set")
	}
	if session.MaxAge != 2592000 {
		t.Fatalf("Max-Age %d, want 2592000", session.MaxAge)
	}
}
