package contracts

import (
	"context"
	"encoding/base64"
	"log/slog"

	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/tvm/cell"

	"mytonstorage-backend/pkg/models"
	v1 "mytonstorage-backend/pkg/models/api/v1"
)

type service struct {
	logger *slog.Logger
}

type Providers interface {
	TopupBalance(ctx context.Context, userAddress string, req v1.TopupRequest) (resp v1.Transaction, err error)
	WithdrawBalance(ctx context.Context, userAddress string, req v1.WithdrawRequest) (resp v1.Transaction, err error)
}

func (s *service) TopupBalance(ctx context.Context, userAddress string, req v1.TopupRequest) (resp v1.Transaction, err error) {
	s.logger.Info("TopupBalance called with", "userAddress", userAddress, slog.Uint64("amount:", req.Amount))

	resp = v1.Transaction{
		Address: req.ContractAddress,
		Amount:  req.Amount,
	}

	return
}

func (s *service) WithdrawBalance(ctx context.Context, userAddress string, req v1.WithdrawRequest) (resp v1.Transaction, err error) {
	s.logger.Info("WithdrawBalance called with", "userAddress", userAddress, "contractAddress", req.ContractAddress)

	addr, err := address.ParseAddr(req.ContractAddress)
	if err != nil {
		s.logger.Error("failed to parse contract address", slog.String("error", err.Error()))
		err = models.NewAppError(models.BadRequestErrorCode, models.ErrMsgInvalidContractAddress)
		return
	}

	body := cell.BeginCell().MustStoreUInt(0x61fff683, 32).MustStoreUInt(0, 64).EndCell().ToBOC()

	resp = v1.Transaction{
		Body:    base64.StdEncoding.EncodeToString(body),
		Address: addr.String(),
		Amount:  tlb.MustFromTON("0.03").Nano().Uint64(),
	}

	return
}

func NewService(logger *slog.Logger) Providers {
	return &service{
		logger: logger,
	}
}
