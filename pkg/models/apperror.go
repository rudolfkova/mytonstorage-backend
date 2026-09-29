package models

import (
	"fmt"
	"net/http"
)

const (
	NotFoundErrorCode       = http.StatusNotFound
	InternalServerErrorCode = http.StatusInternalServerError
	BadRequestErrorCode     = http.StatusBadRequest
	UnauthorizedErrorCode   = http.StatusUnauthorized
	StorageExpiredCode      = http.StatusGone
	ServiceUnavailableCode  = http.StatusServiceUnavailable
)

// Client-facing error messages (whitelist). Only these strings should reach the API client.
const (
	ErrMsgInternalServerError = "internal server error"

	ErrMsgUnauthorized     = "unauthorized"
	ErrMsgForbidden        = "forbidden"
	ErrMsgInvalidRequest   = "invalid request"
	ErrMsgInvalidAddress   = "invalid address"
	ErrMsgInvalidProof     = "invalid proof"
	ErrMsgInvalidSignature = "invalid signature"
	ErrMsgInvalidSession   = "invalid session"
	ErrMsgReloginRequired  = "relogin required"
	ErrMsgTooManyRequests  = "too many requests, please try again later"

	ErrMsgEmptyRequestBody       = "empty request body"
	ErrMsgRequestBodyTooLarge    = "request body too large"
	ErrMsgInvalidContentType     = "invalid content type"
	ErrMsgNoBoundary             = "no boundary in content type"
	ErrMsgFailedCheckUnpaidBags  = "failed to check unpaid bags"
	ErrMsgYouHaveUnpaidBags      = "you have unpaid bags"
	ErrMsgNotEnoughDiskSpace     = "not enough disk space"
	ErrMsgFailedReadUploadLimits = "failed to read upload limits"
	ErrMsgFailedPrepareUploadDir = "failed to prepare upload directory"
	ErrMsgInvalidMultipart       = "invalid multipart"
	ErrMsgInvalidFilename        = "invalid filename"
	ErrMsgFailedReadFile         = "failed to read file"
	ErrMsgFailedSaveFile         = "failed to save file"
	ErrMsgDescriptionTooLarge    = "description too large"
	ErrMsgNoFilesFound           = "no files found"
	ErrMsgFailedCreateBag        = "failed to create bag in storage"
	ErrMsgFailedGetBagInfo       = "failed to get bag info from storage"
	ErrMsgFailedSaveBag          = "failed to save bag"

	ErrMsgFailedDeleteBag        = "failed to delete bag"
	ErrMsgInvalidContractAddress = "invalid contract address"
	ErrMsgFailedMarkBagPaid      = "failed to mark bag as paid"
	ErrMsgFailedGetUnpaidBags    = "failed to get unpaid bags"
	ErrMsgFailedGetBagDetails    = "failed to get bag details"

	ErrMsgTooManyProvidersRequested     = "too many providers requested"
	ErrMsgProbablyBagExpired            = "probably bag is expired"
	ErrMsgInvalidOwnerAddress           = "invalid owner address"
	ErrMsgFileExpired                   = "file expired"
	ErrMsgBagExpired                    = "bag is expired"
	ErrMsgFailedDecodeBagData           = "failed to decode bag data"
	ErrMsgFailedPrepareDeployData       = "failed to prepare contract deploy data"
	ErrMsgInvalidProviderAddress        = "invalid provider address"
	ErrMsgFailedParseStateInit          = "failed to parse state init"
	ErrMsgFailedSetProviderData         = "failed to set provider data"
	ErrMsgContractNotFound              = "contract not found"
	ErrMsgFailedGetPaidBag              = "failed to get paid bag"
	ErrMsgFailedQueueProviders          = "failed to queue providers"
	ErrMsgSomeProvidersUnavailable      = "some providers unavailable"
	ErrMsgSomeProvidersUnavailableRetry = "some providers unavailable, please, try again"
	ErrMsgFailedFetchProvidersRates     = "failed to fetch providers rates"
)

// ErrMsgTooManyFiles keeps the "too many files" prefix the frontend matches.
func ErrMsgTooManyFiles(max int) string {
	return fmt.Sprintf("too many files (max %d)", max)
}

// AppError — custom error type to handle service layer errors
type AppError struct {
	Code    int
	Message string
}

func (e *AppError) Error() string {
	return fmt.Sprintf("code=%d, message=%s", e.Code, e.Message)
}

func NewAppError(code int, message string) *AppError {
	if message == "" {
		message = ErrMsgInternalServerError
	}
	return &AppError{
		Code:    code,
		Message: message,
	}
}
