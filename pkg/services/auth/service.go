package auth

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/ton/wallet"

	"mytonstorage-backend/pkg/models"
	v1 "mytonstorage-backend/pkg/models/api/v1"
)

type service struct {
	verifier *wallet.TonConnectVerifier
	key      ed25519.PrivateKey
	host     string
	logger   *slog.Logger
}

type Auth interface {
	GetData() string
	Login(ctx context.Context, info v1.LoginInfo) (sessionID string, err error)
	Authenticate(ctx context.Context, signature, sessionData string) (addr string, err error)
}

func (s *service) GetData() string {
	return "auth:mytonstorage:" + s.host
}

func (s *service) Login(ctx context.Context, info v1.LoginInfo) (sessionID string, err error) {
	logger := s.logger.With(
		slog.String("method", "Login"),
		slog.String("address", info.Address),
	)

	addr, err := address.ParseRawAddr(info.Address)
	if err != nil {
		logger.Error("failed to parse address", slog.Any("error", err))
		err = models.NewAppError(models.BadRequestErrorCode, models.ErrMsgInvalidAddress)
		return
	}

	if vErr := s.verifier.VerifyProof(ctx, addr, info.Proof, s.GetData(), info.StateInit); vErr != nil {
		logger.Error("failed to verify proof", slog.Any("error", vErr))
		err = models.NewAppError(models.BadRequestErrorCode, models.ErrMsgInvalidProof)
		return
	}

	timestamp := time.Now().Unix()
	sessionData := fmt.Sprintf("%d:%s", timestamp, addr.String())
	signature := ed25519.Sign(s.key, []byte(sessionData))
	sessionID = fmt.Sprintf("%x:%s", signature, sessionData)

	// todo: save to db

	return
}

func (s *service) Authenticate(ctx context.Context, signature, sessionData string) (addr string, err error) {
	logger := s.logger.With(
		slog.String("method", "Authenticate"),
	)

	signedMessage := []byte(sessionData)
	sigBytes, err := hex.DecodeString(signature)
	if err != nil || !ed25519.Verify(s.key.Public().(ed25519.PublicKey), signedMessage, sigBytes) {
		logger.Error("failed to verify signature", slog.Any("error", err))
		err = models.NewAppError(models.UnauthorizedErrorCode, models.ErrMsgInvalidSignature)
		return
	}

	dataParts := strings.SplitN(sessionData, ":", 2)
	if len(dataParts) != 2 {
		logger.Error("invalid session data format")
		err = models.NewAppError(models.BadRequestErrorCode, models.ErrMsgInvalidSession)
		return
	}

	a, err := address.ParseAddr(dataParts[1])
	if err != nil {
		logger.Error("failed to parse address", slog.Any("error", err))
		err = models.NewAppError(models.BadRequestErrorCode, models.ErrMsgInvalidAddress)
		return
	}

	addr = a.String()

	return
}

func New(verifier *wallet.TonConnectVerifier, key ed25519.PrivateKey, host string, logger *slog.Logger) Auth {
	return &service{verifier: verifier, key: key, host: host, logger: logger}
}
