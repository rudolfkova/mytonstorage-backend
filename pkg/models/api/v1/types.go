package v1

import (
	"github.com/xssnick/tonutils-go/ton/wallet"
)

type PaidBagRequest struct {
	BagID           string `json:"bag_id"`
	StorageContract string `json:"storage_contract"`
}

type DetailsRequest struct {
	ContractsAddresses []string `json:"contracts"`
}

type BagInfoShort struct {
	ContractAddress string `json:"contract_address"`
	BagID           string `json:"bag_id"`
	Description     string `json:"description"`
	Size            uint64 `json:"size"`
}

type UserBagInfo struct {
	BagID       string `json:"bag_id"`
	UserAddress string `json:"user_address"`
	Description string `json:"description"`
	FilesCount  uint64 `json:"files_count"`
	BagSize     uint64 `json:"bag_size"`
	CreatedAt   int64  `json:"created_at"`
}

type LoginInfo struct {
	StateInit []byte                 `json:"state_init"`
	Address   string                 `json:"address"`
	Proof     wallet.TonConnectProof `json:"proof"`
}

type ProviderShort struct {
	Pubkey        string `json:"address"`
	PricePerMBDay uint64 `json:"price_per_mb_day"`
	MaxSpan       uint64 `json:"max_span"`
}

type OffersRequest struct {
	Providers []string `json:"providers"`
	BagID     string   `json:"bag_id"`
	BagSize   uint64   `json:"bag_size"`
	Span      uint32   `json:"span"`
}

type ProviderContractData struct {
	Key          string `json:"key"`
	MinBounty    string `json:"min_bounty"`
	MinSpan      uint64 `json:"min_span"`
	MaxSpan      uint64 `json:"max_span"`
	RatePerMBDay uint64 `json:"price_per_mb_day"`
}

type TopupRequest struct {
	ContractAddress string `json:"address"`
	Amount          uint64 `json:"amount"`
}

type WithdrawRequest struct {
	ContractAddress string `json:"address"`
}

type UnpaidBagsResponse struct {
	Bags        []UserBagInfo `json:"bags"`
	FreeStorage uint64        `json:"free_storage"`
}

type InitStorageContractRequest struct {
	ProvidersKeys []string `json:"providers"`
	BagID         string   `json:"bag_id"`
	OwnerAddress  string   `json:"owner_address"`
	Amount        uint64   `json:"amount"`
	Span          uint32   `json:"span"`
}

type UpdateProvidersRequest struct {
	Providers       []string `json:"providers"`
	ContractAddress string   `json:"address"`
	BagSize         uint64   `json:"bag_size"`
	Amount          uint64   `json:"amount"`
	Span            uint32   `json:"span"`
}

type NotifyProvidersRequest struct {
	Providers       []string `json:"providers"`
	ContractAddress string   `json:"address"`
}

type ProviderOffer struct {
	OfferSpan     uint64 `json:"offer_span"`
	PricePerDay   uint64 `json:"price_per_day"`
	PricePerProof uint64 `json:"price_per_proof"`
	PricePerMB    uint64 `json:"price_per_mb"`

	Provider ProviderContractData `json:"provider"`
}

type ProviderRatesResponse struct {
	Offers   []ProviderOffer   `json:"offers"`
	Declines []ProviderDecline `json:"declines,omitempty"`
}

type ProviderDecline struct {
	ProviderKey string `json:"provider_key"`
	Reason      string `json:"reason"`
}

type Transaction struct {
	Body      string `json:"body"`
	StateInit string `json:"state_init"`
	Address   string `json:"address"`
	Amount    uint64 `json:"amount"`
}
