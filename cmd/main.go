package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/ton"
	"github.com/xssnick/tonutils-go/ton/wallet"

	tonclient "mytonstorage-backend/pkg/clients/ton"
	tonstorage "mytonstorage-backend/pkg/clients/ton-storage"
	"mytonstorage-backend/pkg/clients/agentrpc"
	"mytonstorage-backend/pkg/httpServer"
	filesRepository "mytonstorage-backend/pkg/repositories/files"
	providersRepository "mytonstorage-backend/pkg/repositories/providers"
	systemRepository "mytonstorage-backend/pkg/repositories/system"
	"mytonstorage-backend/pkg/services/auth"
	contractsService "mytonstorage-backend/pkg/services/contracts"
	filesService "mytonstorage-backend/pkg/services/files"
	providersService "mytonstorage-backend/pkg/services/providers"
	"mytonstorage-backend/pkg/workers"
	"mytonstorage-backend/pkg/workers/cleaner"
	filesworker "mytonstorage-backend/pkg/workers/files"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() (err error) {
	// Tools
	config := loadConfig()
	if config == nil {
		fmt.Println("failed to load configuration")
		return
	}

	logLevel := slog.LevelInfo
	if level, ok := logLevels[config.System.LogLevel]; ok {
		logLevel = level
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// TON Connect Verifier initialization
	lsCfg, err := liteclient.GetConfigFromUrl(context.Background(), config.TON.ConfigURL)
	if err != nil {
		logger.Error("failed to get liteclient config", slog.String("error", err.Error()))
		return
	}

	client := liteclient.NewConnectionPool()
	err = client.AddConnectionsFromConfig(context.Background(), lsCfg)
	if err != nil {
		logger.Error("failed to add connections from config", slog.String("error", err.Error()))
		return
	}

	api := ton.NewAPIClient(client, ton.ProofCheckPolicyFast).WithRetry()
	verifier := wallet.NewTonConnectVerifier(config.System.Host, config.System.AuthSessionDuration, api)

	// Metrics
	dbRequestsCount := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.Metrics.Namespace,
			Subsystem: config.Metrics.BasicSubsystem,
			Name:      "db_requests_count",
			Help:      "Db requests count",
		},
		[]string{"method", "error"},
	)

	dbRequestsDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: config.Metrics.Namespace,
			Subsystem: config.Metrics.BasicSubsystem,
			Name:      "db_requests_duration",
			Help:      "Db requests duration",
		},
		[]string{"method", "error"},
	)

	workersRunCount := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.Metrics.Namespace,
			Subsystem: config.Metrics.BasicSubsystem,
			Name:      "workers_requests_count",
			Help:      "Workers requests count",
		},
		[]string{"method", "error"},
	)

	workersRunDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: config.Metrics.Namespace,
			Subsystem: config.Metrics.BasicSubsystem,
			Name:      "workers_requests_duration",
			Help:      "Workers requests duration",
		},
		[]string{"method", "error"},
	)

	prometheus.MustRegister(
		dbRequestsCount,
		dbRequestsDuration,
		workersRunCount,
		workersRunDuration,
	)

	// Postgres
	connPool, err := connectPostgres(context.Background(), config, logger)
	if err != nil {
		logger.Error("failed to connect to Postgres", slog.String("error", err.Error()))
		return
	}

	// Database
	filesRepo := filesRepository.NewRepository(connPool)
	filesRepo = filesRepository.NewMetrics(dbRequestsCount, dbRequestsDuration, filesRepo)

	systemRepo := systemRepository.NewRepository(connPool)
	systemRepo = systemRepository.NewMetrics(dbRequestsCount, dbRequestsDuration, systemRepo)

	providerRepo := providersRepository.NewRepository(connPool)
	providerRepo = providersRepository.NewMetrics(dbRequestsCount, dbRequestsDuration, providerRepo)

	// Clients
	tonContractsClient, err := tonclient.NewClient(context.Background(), config.TON.ConfigURL, logger)
	if err != nil {
		logger.Error("failed to create TON client", slog.String("error", err.Error()))
		return
	}

	agentRPC, err := agentrpc.New(agentrpc.Config{
		Endpoints:      agentrpc.ParseEndpointsCSV(config.Agents.Endpoints),
		AuthToken:      config.Agents.AuthToken,
		CACertFile:       config.Agents.CACertFile,
		RequestTimeout: time.Duration(config.Agents.RequestTimeoutMs) * time.Millisecond,
	})
	if err != nil {
		logger.Error("failed to create agent RPC client", slog.String("error", err.Error()))
		return
	}
	defer func() {
		if cErr := agentRPC.Close(); cErr != nil {
			logger.Warn("failed to close agent RPC client", slog.String("error", cErr.Error()))
		}
	}()

	creds := tonstorage.Credentials{
		Login:    config.TONStorage.Login,
		Password: config.TONStorage.Password,
	}
	storage := tonstorage.NewClient(config.TONStorage.BaseURL, config.TONStorage.BagsDirForStorage, &creds)

	// Workers
	cleanerWorker := cleaner.NewWorker(filesRepo, config.System.StoreHistoryDays, logger)
	cleanerWorker = cleaner.NewMetrics(workersRunCount, workersRunDuration, cleanerWorker)

	filesWorker := filesworker.NewWorker(
		filesRepo,
		providerRepo,
		storage,
		agentRPC,
		tonContractsClient,
		config.System.UnpaidFilesLifetimePrivate,
		config.System.PaidFilesLifetime,
		logger,
	)
	filesWorker = filesworker.NewMetrics(workersRunCount, workersRunDuration, filesWorker)

	// Services
	providersSvc := providersService.NewService(
		agentRPC,
		filesRepo,
		storage,
		config.System.MaxAllowedSpanDays,
		config.System.UnpaidFilesLifetimePrivate,
		logger,
	)

	contractsSvc := contractsService.NewService(logger)

	filesSvc := filesService.NewService(
		filesRepo,
		systemRepo,
		storage,
		config.TONStorage.BagsDirForStorage,
		config.System.TotalDiskSpaceAvailable,
		config.System.UnpaidFilesLifetimePublic,
		logger,
	)
	filesSvc = filesService.NewCacheMiddleware(filesSvc)

	seed, err := hex.DecodeString(config.System.AuthPrivateKey)
	if err != nil {
		logger.Error("failed to decode private key", slog.String("error", err.Error()))
		return fmt.Errorf("failed to decode private key: %w", err)
	}

	if len(seed) != ed25519.SeedSize {
		logger.Error("invalid private key length", slog.Int("expected", ed25519.SeedSize), slog.Int("got", len(seed)))
		return fmt.Errorf("invalid private key length: expected %d, got %d", ed25519.SeedSize, len(seed))
	}

	authSvc := auth.New(verifier, ed25519.NewKeyFromSeed(seed), config.System.Host, logger)

	// Start workers
	cancelCtx, cancel := context.WithCancel(context.Background())
	workers := workers.NewWorkers(filesWorker, cleanerWorker, logger)
	go func() {
		if wErr := workers.Start(cancelCtx); wErr != nil {
			logger.Error("failed to start workers", slog.String("error", wErr.Error()))
			err = wErr
			return
		}
	}()

	// HTTP Server
	adminAuthTokens := strings.Split(config.System.AdminAuthTokens, ",")
	app := fiber.New(fiber.Config{
		AppName:      "mytonstorage-backend",
		ReadTimeout:  10 * time.Minute,
		WriteTimeout: 10 * time.Minute,
		BodyLimit:    4 << 30, // 4 GiB

		/*
			DisablePreParseMultipartForm выключает парсинг multipart form на уровне valyala/fasthttp,
			т.к. упираемся в захардкоженный лимит в defaultMaxInMemoryFileSize == 16MB
			Из-за этого у нас возникает ситуация когда мы можем загрузить один или несколько
			больших файлов размером BodyLimit(параметр выше), но не можем загрузить много маленьких файлов
			каждый из которых не привышает лимит в defaultMaxInMemoryFileSize, но общий объем превышает.

			Пишут что может сильно увеличить использование памяти на сервере, нужно последить.
		*/
		DisablePreParseMultipartForm: true,
	})
	server := httpServer.New(
		app,
		filesSvc,
		providersSvc,
		contractsSvc,
		authSvc,
		adminAuthTokens,
		config.Metrics.Namespace,
		config.Metrics.ServerSubsystem,
		logger,
	)

	server.RegisterRoutes()

	go func() {
		if err := app.Listen(":" + config.System.Port); err != nil {
			logger.Error("error starting server", slog.String("err", err.Error()))
		}
	}()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	<-signalChan
	cancel()

	err = app.ShutdownWithTimeout(time.Second * 5)
	if err != nil {
		logger.Error("server shut down error", slog.String("err", err.Error()))
		return err
	}

	return err
}
