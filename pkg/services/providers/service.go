package providers

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/big"
	"math/rand"
	"strings"
	"time"

	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/tvm/cell"
	"github.com/xssnick/tonutils-storage-provider/pkg/contract"
	"github.com/xssnick/tonutils-storage/provider"

	"mytonstorage-backend/pkg/clients/agentrpc"
	tonstorage "mytonstorage-backend/pkg/clients/ton-storage"
	"mytonstorage-backend/pkg/models"
	v1 "mytonstorage-backend/pkg/models/api/v1"
	"mytonstorage-backend/pkg/utils"
)

const (
	providersLimit         = 256
	providerRequestTimeout = 14 * time.Second
	getStorageRatesTimeout = 7 * time.Second
)

type files interface {
	IsBagExpired(ctx context.Context, bagID string, userAddress string, sec uint64) (expired bool, err error)
}

type storage interface {
	GetBag(ctx context.Context, bagId string) (*tonstorage.BagDetailed, error)
}

type service struct {
	files               files
	storage             storage
	agent               *agentrpc.Client
	maxAllowedSpan      uint64
	unpaidFilesLifetime time.Duration
	logger              *slog.Logger
}

type Providers interface {
	FetchProvidersRates(ctx context.Context, req v1.OffersRequest) (resp v1.ProviderRatesResponse, err error)
	FetchProvidersRatesBySize(ctx context.Context, providers []string, bagSize uint64, span uint32) (resp v1.ProviderRatesResponse)
	InitStorageContract(ctx context.Context, info v1.InitStorageContractRequest, providers []v1.ProviderShort) (resp v1.Transaction, err error)
	EditStorageContract(ctx context.Context, address string, amount uint64, providers []v1.ProviderShort) (resp v1.Transaction, err error)

	fetchProviderRates(ctx context.Context, providerKey string, bagSize uint64, span uint32) (offer *v1.ProviderOffer, reason string)
}

func (s *service) FetchProvidersRates(ctx context.Context, req v1.OffersRequest) (resp v1.ProviderRatesResponse, err error) {
	log := s.logger.With(
		"method", "FetchProvidersRates",
		"bag_id", req.BagID,
		"providers", req.Providers)

	if len(req.Providers) > providersLimit {
		log.Error("too many providers requested", slog.Int("limit", providersLimit))
		err = models.NewAppError(models.BadRequestErrorCode, "too many providers requested")
		return
	}

	if len(req.Providers) == 0 {
		return
	}

	bagSize := req.BagSize
	if bagSize == 0 {
		details, bErr := s.storage.GetBag(ctx, req.BagID)
		if bErr != nil {
			log.Error("failed to get bag details", slog.String("error", bErr.Error()))
			err = models.NewAppError(models.StorageExpiredCode, "probably bag is expired")
			return
		}

		bagSize = details.BagSize
	}

	resp = s.FetchProvidersRatesBySize(ctx, req.Providers, bagSize, req.Span)

	return resp, nil
}

func (s *service) FetchProvidersRatesBySize(ctx context.Context, providers []string, bagSize uint64, span uint32) (resp v1.ProviderRatesResponse) {
	if len(providers) == 0 {
		return
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, providerRequestTimeout)
	defer cancel()

	ratesByKey, err := s.agent.GetStorageRates(timeoutCtx, providers, bagSize)
	if err != nil {
		for _, providerKey := range providers {
			resp.Declines = append(resp.Declines, v1.ProviderDecline{
				ProviderKey: providerKey,
				Reason:      "can't fetch rates",
			})
		}
		return
	}

	for _, providerKey := range providers {
		row, ok := ratesByKey[strings.ToUpper(strings.TrimSpace(providerKey))]
		if !ok {
			resp.Declines = append(resp.Declines, v1.ProviderDecline{
				ProviderKey: providerKey,
				Reason:      "can't fetch rates",
			})
			continue
		}
		offer, reason := s.offerFromRatesRow(providerKey, bagSize, span, row)
		if reason != "" {
			resp.Declines = append(resp.Declines, v1.ProviderDecline{
				ProviderKey: providerKey,
				Reason:      reason,
			})
			continue
		}
		resp.Offers = append(resp.Offers, *offer)
	}

	return
}

func (s *service) InitStorageContract(ctx context.Context, info v1.InitStorageContractRequest, providers []v1.ProviderShort) (resp v1.Transaction, err error) {
	log := s.logger.With(
		"method", "InitStorageContract",
		"bag_id", info.BagID,
		"owner", info.OwnerAddress,
		"amount", info.Amount)

	if len(providers) > providersLimit {
		log.Error("too many providers requested", slog.Int("limit", providersLimit))
		err = models.NewAppError(models.BadRequestErrorCode, "too many providers requested")
		return
	}

	if len(providers) == 0 {
		return
	}

	ownerAddr, err := address.ParseAddr(info.OwnerAddress)
	if err != nil {
		log.Error("failed to parse owner address", slog.String("error", err.Error()))
		err = models.NewAppError(models.BadRequestErrorCode, "invalid owner address")
		return
	}

	expired, err := s.files.IsBagExpired(ctx, info.BagID, ownerAddr.String(), uint64(s.unpaidFilesLifetime.Seconds()))
	if err != nil {
		log.Error("failed to check if bag is expired", slog.String("error", err.Error()))
		err = models.NewAppError(models.BadRequestErrorCode, "file expired")
		return
	}

	if expired {
		log.Error("bag is expired", slog.String("bag_id", info.BagID))
		err = models.NewAppError(models.BadRequestErrorCode, "bag is expired")
		return
	}

	details, err := s.storage.GetBag(ctx, info.BagID)
	if err != nil {
		log.Error("failed to get bag details", slog.String("error", err.Error()))
		err = models.NewAppError(models.ServiceUnavailableCode, "failed to get bag details")
		return
	}

	merkle, err := hex.DecodeString(details.MerkleHash)
	if err != nil {
		log.Error("failed to decode merkle hash", slog.String("error", err.Error()))
		err = models.NewAppError(models.InternalServerErrorCode, "")
		return
	}

	torrentHash, err := hex.DecodeString(info.BagID)
	if err != nil {
		log.Error("failed to decode torrent hash", slog.String("error", err.Error()))
		err = models.NewAppError(models.InternalServerErrorCode, "")
		return
	}

	addr, sx, _, err := contract.PrepareV1DeployData(torrentHash, merkle, details.BagSize, details.PieceSize, ownerAddr, nil)
	if err != nil {
		log.Error("failed to prepare contract deploy data", slog.String("error", err.Error()))
		err = models.NewAppError(models.ServiceUnavailableCode, "failed to prepare contract deploy data")
		return
	}

	fmt.Printf("code hash: %x\n", sx.Code.Hash())

	var prs []contract.ProviderV1
	for _, p := range providers {
		d, dErr := hex.DecodeString(p.Pubkey)
		if dErr != nil {
			log.Error("failed to decode provider address", slog.String("error", dErr.Error()))
			err = models.NewAppError(models.BadRequestErrorCode, "invalid provider address")
			return
		}

		pAddr := address.NewAddress(0, 0, d)
		if pAddr == nil {
			log.Error("failed to parse provider address", "provider", p.Pubkey)
			err = models.NewAppError(models.BadRequestErrorCode, "invalid provider address")
			return
		}

		prs = append(prs, contract.ProviderV1{
			Address:       pAddr,
			MaxSpan:       uint32(p.MaxSpan),
			PricePerMBDay: tlb.FromNanoTON(new(big.Int).SetUint64(p.PricePerMBDay)),
		})
	}

	_, stateInit, body, err := contract.PrepareV1DeployData(torrentHash, merkle, details.BagSize, details.PieceSize, ownerAddr, prs)
	if err != nil {
		log.Error("failed to prepare contract deploy data", slog.String("error", err.Error()))
		err = models.NewAppError(models.ServiceUnavailableCode, "failed to prepare contract deploy data")
		return
	}

	siCell, err := tlb.ToCell(stateInit)
	if err != nil {
		log.Error("failed to convert state init to cell", slog.String("error", err.Error()))
		err = models.NewAppError(models.ServiceUnavailableCode, "failed to parse state init")
		return
	}

	b := base64.StdEncoding.EncodeToString(body.ToBOC())
	si := base64.StdEncoding.EncodeToString(siCell.ToBOC())

	resp = v1.Transaction{
		Address:   addr.String(),
		Body:      b,
		StateInit: si,
		Amount:    info.Amount,
	}

	return
}

func (s *service) EditStorageContract(ctx context.Context, contractAddr string, amount uint64, providers []v1.ProviderShort) (resp v1.Transaction, err error) {
	log := s.logger.With(
		"method", "EditStorageContract",
		"contract_address", contractAddr,
	)

	addr, err := address.ParseAddr(contractAddr)
	if err != nil {
		log.Error("failed to parse address", slog.String("error", err.Error()))
		err = models.NewAppError(models.BadRequestErrorCode, "invalid address")
		return
	}

	providersDict := cell.NewDict(256)
	for _, p := range providers {
		d, dErr := hex.DecodeString(p.Pubkey)
		if dErr != nil {
			log.Error("failed to decode provider address", slog.String("error", dErr.Error()))
			err = models.NewAppError(models.BadRequestErrorCode, "invalid provider address")
			return
		}

		pAddr := address.NewAddress(0, 0, d)
		if pAddr == nil {
			log.Error("failed to parse provider address", "provider", p.Pubkey)
			err = models.NewAppError(models.BadRequestErrorCode, "invalid provider address")
			return
		}

		err = providersDict.SetIntKey(new(big.Int).SetBytes(pAddr.Data()),
			cell.BeginCell().
				MustStoreUInt(p.MaxSpan, 32).
				MustStoreBigCoins(big.NewInt(int64(p.PricePerMBDay))).
				EndCell())
		if err != nil {
			err = models.NewAppError(models.InternalServerErrorCode, "failed to set provider data")
			return
		}
	}

	body := cell.BeginCell().
		MustStoreUInt(0x3dc680ae, 32).
		MustStoreUInt(uint64(rand.Int63()), 64).
		MustStoreDict(providersDict).
		EndCell()

	b := base64.StdEncoding.EncodeToString(body.ToBOC())

	resp = v1.Transaction{
		Address:   addr.String(),
		Body:      b,
		StateInit: "",
		Amount:    amount,
	}

	return
}

func (s *service) fetchProviderRates(ctx context.Context, providerKey string, bagSize uint64, span uint32) (offer *v1.ProviderOffer, reason string) {
	log := s.logger.With(
		"method", "fetchProviderRates",
		"bag_size", bagSize,
		"provider_key", providerKey)

	ratesByKey, err := func() (map[string]agentrpc.StorageRatesRow, error) {
		var out map[string]agentrpc.StorageRatesRow
		rErr := utils.TryNTimes(func() error {
			timeoutCtx, cancel := context.WithTimeout(ctx, getStorageRatesTimeout)
			defer cancel()
			var gErr error
			out, gErr = s.agent.GetStorageRates(timeoutCtx, []string{providerKey}, bagSize)
			return gErr
		}, 3)
		return out, rErr
	}()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Error("provider rates request timed out", slog.String("error", err.Error()))
			reason = "long response time"
			return
		}

		log.Error("failed to fetch rates", slog.String("error", err.Error()))
		reason = "can't fetch rates"
		return
	}

	row, ok := ratesByKey[strings.ToUpper(strings.TrimSpace(providerKey))]
	if !ok {
		log.Error("provider missing from agent response")
		reason = "can't fetch rates"
		return
	}

	return s.offerFromRatesRow(providerKey, bagSize, span, row)
}

func (s *service) offerFromRatesRow(providerKey string, bagSize uint64, span uint32, row agentrpc.StorageRatesRow) (offer *v1.ProviderOffer, reason string) {
	if !row.OK {
		reason = "can't fetch rates"
		if row.Details != "" {
			reason = row.Details
		}
		return
	}

	rates := struct {
		Available        bool
		RatePerMBDay     []byte
		MinBounty        []byte
		SpaceAvailableMB uint64
		MinSpan          uint32
		MaxSpan          uint32
	}{
		Available:        row.Available,
		RatePerMBDay:     row.RatePerMBDay,
		MinBounty:        row.MinBounty,
		SpaceAvailableMB: row.SpaceAvailableMB,
		MinSpan:          row.MinSpan,
		MaxSpan:          row.MaxSpan,
	}

	if rates.SpaceAvailableMB < bagSize {
		reason = "not enough space"
		return
	}

	rates.MinSpan = span
	rates.MaxSpan = span

	if !rates.Available {
		reason = "not available"
		return
	}

	p := provider.ProviderRates{
		Available:        rates.Available,
		RatePerMBDay:     tlb.FromNanoTON(new(big.Int).SetBytes(rates.RatePerMBDay)),
		MinBounty:        tlb.FromNanoTON(new(big.Int).SetBytes(rates.MinBounty)),
		SpaceAvailableMB: rates.SpaceAvailableMB,
		MinSpan:          rates.MinSpan,
		MaxSpan:          rates.MaxSpan,
		Size:             bagSize,
	}

	o := provider.CalculateBestProviderOffer(&p)

	offer = &v1.ProviderOffer{
		OfferSpan:     uint64(o.Span),
		PricePerDay:   o.PerDayNano.Uint64(),
		PricePerProof: o.PerProofNano.Uint64(),
		PricePerMB:    o.RatePerMBNano.Uint64(),
		Provider: v1.ProviderContractData{
			Key:          strings.ToUpper(providerKey),
			MinBounty:    tlb.FromNanoTON(new(big.Int).SetBytes(rates.MinBounty)).String(),
			MinSpan:      uint64(rates.MinSpan),
			MaxSpan:      uint64(rates.MaxSpan),
			RatePerMBDay: new(big.Int).SetBytes(rates.RatePerMBDay).Uint64(),
		},
	}

	return
}

func NewService(agent *agentrpc.Client, files files, storage storage, maxAllowedSpanDays uint32, unpaidFilesLifetime time.Duration, logger *slog.Logger) Providers {
	return &service{
		agent:               agent,
		maxAllowedSpan:      uint64(maxAllowedSpanDays) * 24 * 60 * 60,
		unpaidFilesLifetime: unpaidFilesLifetime,
		files:               files,
		storage:             storage,
		logger:              logger,
	}
}
