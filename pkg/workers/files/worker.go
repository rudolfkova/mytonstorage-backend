package filesworker

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"slices"
	"strings"
	"time"

	"github.com/xssnick/tonutils-go/address"

	"mytonstorage-backend/pkg/clients/agentrpc"
	tonclient "mytonstorage-backend/pkg/clients/ton"
	"mytonstorage-backend/pkg/models/db"
)

const (
	maxNotifyAttempts = 10
)

type filesDb interface {
	RemoveUnusedBags(ctx context.Context) (removed []string, err error)
	RemoveUnpaidBagsRelations(ctx context.Context, sec uint64) (bagids []string, err error)
	RemoveNotifiedBags(ctx context.Context, limit int, sec uint64, maxNotifyAttempts int, maxDownloadChecks int) (removed []string, err error)
	GetNotifyInfo(ctx context.Context, limit int, notifyAttempts int) (resp []db.BagStorageContract, err error)
	IncreaseAttempts(ctx context.Context, bags []db.BagStorageContract) error
}

type providersDb interface {
	AddProviderToNotifyQueue(ctx context.Context, notifications []db.ProviderNotification) error
	GetProvidersInProgress(ctx context.Context, limit int, maxDownloadChecks int) (notifications []db.ProviderNotification, err error)
	GetProvidersToNotify(ctx context.Context, limit int, notifyAttempts int) (notifications []db.ProviderNotification, err error)
	IncreaseDownloadChecks(ctx context.Context, notifications []db.ProviderNotification) error
	IncreaseNotifyAttempts(ctx context.Context, notifications []db.ProviderNotification) error
	MarkAsNotified(ctx context.Context, notifications []db.ProviderNotification) error
}

type storage interface {
	RemoveBag(ctx context.Context, bagId string, withFiles bool) error
}

type contractsClient interface {
	GetProvidersInfo(ctx context.Context, addrs []string) (contractsProviders []tonclient.StorageContractProviders, err error)
}

type storageCheckMode int

const (
	storageCheckNotify storageCheckMode = iota
	storageCheckDownload
)

type filesWorker struct {
	filesDb             filesDb
	providersDb         providersDb
	tonstorage          storage
	agent               *agentrpc.Client
	contractsClient     contractsClient
	unpaidFilesLifetime time.Duration
	paidFilesLifetime   time.Duration
	logger              *slog.Logger
}

type Worker interface {
	RemoveUnpaidFiles(ctx context.Context) (interval time.Duration, err error)
	MarkToRemoveUnpaidFiles(ctx context.Context) (interval time.Duration, err error)
	RemoveNotifiedFiles(ctx context.Context) (interval time.Duration, err error)

	TriggerProvidersDownload(ctx context.Context) (interval time.Duration, err error)
	DownloadChecker(ctx context.Context) (interval time.Duration, err error)

	CollectContractProvidersToNotify(ctx context.Context) (interval time.Duration, err error)
}

// This worker check table bags and if some bag have no users(in bag_users) it will be removed from db and from disk.
func (w *filesWorker) RemoveUnpaidFiles(ctx context.Context) (interval time.Duration, err error) {
	const (
		failureInterval = 5 * time.Second
		successInterval = 1 * time.Minute
	)

	log := w.logger.With("worker", "RemoveUnpaidFiles")

	interval = successInterval

	removed, err := w.filesDb.RemoveUnusedBags(ctx)
	if err != nil {
		interval = failureInterval
		return
	}

	for _, bagID := range removed {
		log.Info("removing unused bag from disk", "bag_id", bagID)
		err = w.tonstorage.RemoveBag(ctx, bagID, true)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				log.Info("Bag already deleted")
				continue
			}

			log.Error("failed to remove bag from disk", "bag_id", bagID, "error", err.Error())
			continue
		}
	}

	if len(removed) > 0 {
		log.Info("removed unused files", "count", len(removed))
	}

	return
}

func (w *filesWorker) MarkToRemoveUnpaidFiles(ctx context.Context) (interval time.Duration, err error) {
	const (
		failureInterval = 5 * time.Second
		successInterval = 1 * time.Minute
	)

	log := w.logger.With("worker", "MarkToRemoveUnpaidFiles")

	interval = successInterval

	removed, err := w.filesDb.RemoveUnpaidBagsRelations(ctx, uint64(w.unpaidFilesLifetime.Seconds()))
	if err != nil {
		interval = failureInterval
		return
	}

	if len(removed) > 0 {
		log.Info("removed old unpaid files", "count", len(removed))
	}

	return
}

/*
RemoveNotifiedFiles removes:

(
failed to notify after N attempts
OR failed download check after N attempts
OR fully downloaded
)
AND older than 1 hour

Remove only if all same bagid can be deleted
*/
func (w *filesWorker) RemoveNotifiedFiles(ctx context.Context) (interval time.Duration, err error) {
	const (
		failureInterval   = 5 * time.Second
		successInterval   = 1 * time.Minute
		batch             = 20
		maxDownloadChecks = 10
	)

	log := w.logger.With("worker", "RemoveNotifiedFiles")

	interval = successInterval

	removed, err := w.filesDb.RemoveNotifiedBags(ctx, batch, uint64(w.paidFilesLifetime.Seconds()), maxNotifyAttempts, maxDownloadChecks)
	if err != nil {
		interval = failureInterval
		return
	}

	for _, bagID := range removed {
		log.Info("removing notified bag from disk", "bag_id", bagID)
		err = w.tonstorage.RemoveBag(ctx, bagID, true)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				log.Info("Bag already deleted")
				continue
			}

			log.Error("failed to remove bag from disk", "bag_id", bagID, "error", err.Error())
			continue
		}
	}

	if len(removed) > 0 {
		log.Info("removed notified files", "count", len(removed))
	}

	return
}

func (w *filesWorker) CollectContractProvidersToNotify(ctx context.Context) (interval time.Duration, err error) {
	const (
		failureInterval                = 5 * time.Second
		successInterval                = 1 * time.Second
		nothingToUpdateInterval        = 1 * time.Minute
		batch                          = 10
		fetchContractProvidersAttempts = 10
		fetchProvidersTimeout          = batch * 10 * time.Second
	)

	log := w.logger.With("worker", "CollectContractProvidersToNotify")

	interval = successInterval

	// Get all bags that need to be downloaded by providers
	contractsToNotify, err := w.filesDb.GetNotifyInfo(ctx, batch, fetchContractProvidersAttempts)
	if err != nil {
		err = fmt.Errorf("failed to get notify info: %w", err)
		interval = failureInterval
		return
	}

	if len(contractsToNotify) < batch {
		interval = nothingToUpdateInterval
	}

	var addrs []string
	for _, contract := range contractsToNotify {
		addrs = append(addrs, contract.StorageContract)
	}

	// Defer increasing attempts if there's an error
	defer func() {
		if err != nil {
			_ = w.filesDb.IncreaseAttempts(ctx, contractsToNotify)
		}
	}()

	// Get provider information for each storage contract
	timeoutCtx, cancel := context.WithTimeout(ctx, fetchProvidersTimeout)
	defer cancel()
	contractsProviders, err := w.contractsClient.GetProvidersInfo(timeoutCtx, addrs)
	if err != nil {
		err = fmt.Errorf("failed to get providers info: %w", err)
		interval = failureInterval
		return
	}

	var providersToNotify []db.ProviderNotification
	for _, contract := range contractsProviders {
		sliceIndex := slices.IndexFunc(contractsToNotify, func(item db.BagStorageContract) bool {
			return item.StorageContract == contract.Address
		})
		if sliceIndex == -1 {
			continue
		}

		for _, provider := range contract.Providers {
			pk := hex.EncodeToString([]byte(provider.Key))

			providersToNotify = append(providersToNotify, db.ProviderNotification{
				BagID:           contractsToNotify[sliceIndex].BagID,
				StorageContract: contract.Address,
				ProviderPubkey:  pk,
				Size:            contractsToNotify[sliceIndex].FilesSize,
			})
		}
	}

	if len(providersToNotify) == 0 {
		_ = w.filesDb.IncreaseAttempts(ctx, contractsToNotify)
		interval = nothingToUpdateInterval
		return
	}

	// Update the database with the new provider notifications + mark bags as notified
	err = w.providersDb.AddProviderToNotifyQueue(ctx, providersToNotify)
	if err != nil {
		err = fmt.Errorf("failed to add provider to notify queue: %w", err)
		interval = failureInterval
		return
	}

	if len(providersToNotify) > 0 {
		log.Info("contract relations added to notify queue", "count", len(providersToNotify))
	}

	return
}

func (w *filesWorker) TriggerProvidersDownload(ctx context.Context) (interval time.Duration, err error) {
	const (
		failureInterval         = 5 * time.Second
		successInterval         = 1 * time.Second
		nothingToUpdateInterval = 5 * time.Minute
		batch                   = 100
	)

	log := w.logger.With("worker", "TriggerProvidersDownload")

	interval = successInterval

	providersToNotify, err := w.providersDb.GetProvidersToNotify(ctx, batch, maxNotifyAttempts)
	if err != nil {
		err = fmt.Errorf("failed to get providers to notify: %w", err)
		interval = failureInterval
		return
	}

	if len(providersToNotify) < batch {
		interval = nothingToUpdateInterval
	}

	notified, failed := w.runStorageChecks(ctx, providersToNotify, storageCheckNotify)

	// Defer increasing attempts if there's an error
	defer func() {
		if err != nil {
			_ = w.providersDb.IncreaseNotifyAttempts(ctx, providersToNotify)
		}
	}()

	if len(failed) > 0 {
		_ = w.providersDb.IncreaseNotifyAttempts(ctx, failed)
		log.Warn("Some providers failed notification check", "failed_count", len(failed))
	}

	if len(notified) > 0 {
		aErr := w.providersDb.MarkAsNotified(ctx, notified)
		if aErr != nil {
			err = fmt.Errorf("failed to mark as notified: %w", aErr)
			interval = failureInterval
			return
		}

		log.Info("Providers successfully checked and marked as notified", "count", len(notified))
	}

	return
}

func (w *filesWorker) DownloadChecker(ctx context.Context) (interval time.Duration, err error) {
	const (
		failureInterval         = 5 * time.Second
		successInterval         = 1 * time.Second
		nothingToUpdateInterval = 1 * time.Minute
		batch                   = 20
		maxDownloadChecks       = 10
	)

	log := w.logger.With("worker", "DownloadChecker")

	interval = successInterval

	providersToCheck, err := w.providersDb.GetProvidersInProgress(ctx, batch, maxDownloadChecks)
	if err != nil {
		err = fmt.Errorf("failed to get providers to notify: %w", err)
		interval = failureInterval
		return
	}

	if len(providersToCheck) < batch {
		interval = nothingToUpdateInterval
	}

	checked, failed := w.runStorageChecks(ctx, providersToCheck, storageCheckDownload)

	if len(failed) > 0 {
		_ = w.providersDb.IncreaseDownloadChecks(ctx, failed)
		log.Info("Some providers failed download check", "failed_count", len(failed))
	}

	if len(checked) > 0 {
		aErr := w.providersDb.IncreaseDownloadChecks(ctx, checked)
		if aErr != nil {
			err = fmt.Errorf("failed to mark as notified: %w", aErr)
			interval = failureInterval
			return
		}

		log.Info("Providers successfully checked for download", "count", len(checked))
	}

	return
}

func (w *filesWorker) runStorageChecks(ctx context.Context, providers []db.ProviderNotification, mode storageCheckMode) (checked []db.ProviderNotification, failed []db.ProviderNotification) {
	log := w.logger.With("worker", "runStorageChecks")
	if len(providers) == 0 {
		return
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	queries := make([]agentrpc.StorageInfoQuery, 0, len(providers))
	indexByKey := make(map[string]int, len(providers))

	for i, provider := range providers {
		toProof := r.Uint64() % uint64(math.Max(float64(provider.Size), 1))
		if _, err := address.ParseAddr(provider.StorageContract); err != nil {
			log.Error("failed to parse storage contract address",
				"error", err.Error(),
				"storage_contract", provider.StorageContract)
			failed = append(failed, provider)
			continue
		}

		key := strings.ToLower(provider.ProviderPubkey) + "|" + provider.StorageContract
		indexByKey[key] = i
		queries = append(queries, agentrpc.StorageInfoQuery{
			ProviderPubkey:  provider.ProviderPubkey,
			ContractAddress: provider.StorageContract,
			ByteToProof:     toProof,
		})
	}

	if len(queries) == 0 {
		return nil, failed
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	rows, err := w.agent.RequestStorageInfo(timeoutCtx, queries)
	if err != nil {
		log.Error("failed to request storage info from agent", "error", err.Error())
		for key := range indexByKey {
			failed = append(failed, providers[indexByKey[key]])
		}
		return nil, failed
	}

	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		key := strings.ToLower(row.ProviderPubkey) + "|" + row.ContractAddress
		idx, ok := indexByKey[key]
		if !ok {
			continue
		}
		seen[key] = struct{}{}
		provider := providers[idx]

		if !row.OK {
			log.Error("failed to notify provider",
				"error", row.Details,
				"provider_pubkey", provider.ProviderPubkey)
			failed = append(failed, provider)
			continue
		}

		if row.Status == "error" {
			log.Error("provider returned error status",
				"error", row.Reason,
				"provider_pubkey", provider.ProviderPubkey)
			failed = append(failed, provider)
			continue
		}

		provider.Downloaded = row.Downloaded

		switch mode {
		case storageCheckNotify:
			checked = append(checked, provider)
		case storageCheckDownload:
			provider.Downloaded = row.Downloaded
			checked = append(checked, provider)
		}
	}

	for key := range indexByKey {
		if _, ok := seen[key]; ok {
			continue
		}
		log.Error("missing provider in agent response", "provider_pubkey", providers[indexByKey[key]].ProviderPubkey)
		failed = append(failed, providers[indexByKey[key]])
	}

	return
}

func NewWorker(
	filesDb filesDb,
	providersDb providersDb,
	tonstorage storage,
	agent *agentrpc.Client,
	contractsClient contractsClient,
	unpaidFilesLifetime time.Duration,
	paidFilesLifetime time.Duration,
	logger *slog.Logger,
) Worker {
	return &filesWorker{
		filesDb:             filesDb,
		providersDb:         providersDb,
		tonstorage:          tonstorage,
		agent:               agent,
		contractsClient:     contractsClient,
		unpaidFilesLifetime: unpaidFilesLifetime,
		paidFilesLifetime:   paidFilesLifetime,
		logger:              logger,
	}
}
