package httpServer

import (
	"context"
	"log/slog"
	"mime/multipart"

	"github.com/gofiber/fiber/v2"

	v1 "mytonstorage-backend/pkg/models/api/v1"
)

type files interface {
	AddFiles(ctx context.Context, mr *multipart.Reader, size uint64, userAddr string) (info v1.UnpaidBagsResponse, err error)
	DeleteBag(ctx context.Context, bagID string, userAddr string) error
	MarkBagAsPaid(ctx context.Context, bagID, userAddress, storageContract string) (err error)
	GetUnpaidBags(ctx context.Context, userAddr string) (info v1.UnpaidBagsResponse, err error)
	GetBagsInfoShort(ctx context.Context, bagIDs []string) (descriptions []v1.BagInfoShort, err error)
}

type contracts interface {
	TopupBalance(ctx context.Context, userAddress string, req v1.TopupRequest) (resp v1.Transaction, err error)
	WithdrawBalance(ctx context.Context, userAddress string, req v1.WithdrawRequest) (resp v1.Transaction, err error)
}

type providers interface {
	FetchProvidersRates(ctx context.Context, req v1.OffersRequest) (resp v1.ProviderRatesResponse, err error)
	FetchProvidersRatesBySize(ctx context.Context, providers []string, bagSize uint64, span uint32) (resp v1.ProviderRatesResponse)
	InitStorageContract(ctx context.Context, info v1.InitStorageContractRequest, providers []v1.ProviderShort) (resp v1.Transaction, err error)
	EditStorageContract(ctx context.Context, address string, amount uint64, providers []v1.ProviderShort) (resp v1.Transaction, err error)
}

type auth interface {
	GetData() string
	Login(ctx context.Context, info v1.LoginInfo) (sessionID string, err error)
	Authenticate(ctx context.Context, signature, sessionData string) (addr string, err error)
}

type errorResponse struct {
	Error string `json:"error"`
}

type handler struct {
	server          *fiber.App
	logger          *slog.Logger
	files           files
	providers       providers
	contracts       contracts
	auth            auth
	namespace       string
	subsystem       string
	adminAuthTokens map[string]struct{}
}

func New(
	server *fiber.App,
	files files,
	providers providers,
	contracts contracts,
	auth auth,
	adminAuthTokens []string,
	namespace string,
	subsystem string,
	logger *slog.Logger,
) *handler {
	adminTokensMap := make(map[string]struct{})
	for _, token := range adminAuthTokens {
		adminTokensMap[token] = struct{}{}
	}

	h := &handler{
		server:          server,
		files:           files,
		providers:       providers,
		contracts:       contracts,
		auth:            auth,
		namespace:       namespace,
		subsystem:       subsystem,
		adminAuthTokens: adminTokensMap,
		logger:          logger,
	}

	return h
}
