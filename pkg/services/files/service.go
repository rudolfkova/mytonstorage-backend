package files

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/xssnick/tonutils-go/address"
	"golang.org/x/exp/utf8string"

	tonstorage "mytonstorage-backend/pkg/clients/ton-storage"
	"mytonstorage-backend/pkg/models"
	v1 "mytonstorage-backend/pkg/models/api/v1"
	"mytonstorage-backend/pkg/models/db"
)

const (
	descriptionsStoreLimit = 1000
)

type service struct {
	files                   filesDb
	system                  systemDb
	tonstorage              storage
	storageDir              string
	totalDiskSpaceAvailable uint64
	unpaidFilesLifetime     time.Duration
	logger                  *slog.Logger
}

type systemDb interface {
	GetParam(ctx context.Context, key string) (value string, err error)
}

type storage interface {
	List(ctx context.Context) (*tonstorage.ListShort, error)
	Create(ctx context.Context, description, path string) (string, error)
	GetBag(ctx context.Context, bagId string) (*tonstorage.BagDetailed, error)
	RemoveBag(ctx context.Context, bagId string, withFiles bool) error
}

type filesDb interface {
	AddBag(ctx context.Context, bag db.BagInfo, userAddr string) error
	RemoveUserBagRelation(ctx context.Context, bagID, userAddress string) (int64, error)
	RemoveUnusedBags(ctx context.Context) (removed []string, err error)
	CanUpload(ctx context.Context, userID string, sec uint64) (bool, error)
	GetUnpaidBags(ctx context.Context, userID string) ([]db.UserBagInfo, error)
	GetNotifyInfo(ctx context.Context, limit int, notifyAttempts int) ([]db.BagStorageContract, error)
	IncreaseAttempts(ctx context.Context, bags []db.BagStorageContract) error
	MarkBagAsPaid(ctx context.Context, bagID, userAddress, storageContract string) (cnt int64, err error)
	GetBagsInfoShort(ctx context.Context, contracts []string) (info []db.BagDescription, err error)
}

type Files interface {
	AddFiles(ctx context.Context, mr *multipart.Reader, size uint64, userAddr string) (info v1.UnpaidBagsResponse, err error)
	DeleteBag(ctx context.Context, bagID string, userAddr string) error
	MarkBagAsPaid(ctx context.Context, bagID, userAddress, storageContract string) (err error)
	GetUnpaidBags(ctx context.Context, userAddr string) (info v1.UnpaidBagsResponse, err error)
	GetBagsInfoShort(ctx context.Context, contracts []string) (info []v1.BagInfoShort, err error)
}

func (s *service) AddFiles(ctx context.Context, mr *multipart.Reader, size uint64, userAddr string) (info v1.UnpaidBagsResponse, err error) {
	log := s.logger.With(
		slog.String("method", "AddFiles"),
		slog.Uint64("size", size),
		slog.String("user_address", userAddr),
	)

	// Check paids
	canUpload, err := s.files.CanUpload(ctx, userAddr, uint64(s.unpaidFilesLifetime.Seconds()))
	if err != nil {
		log.Error("Failed to get unpaid bags", slog.Any("error", err))
		err = models.NewAppError(models.InternalServerErrorCode, "")
		return
	}

	if !canUpload {
		err = models.NewAppError(models.BadRequestErrorCode, "you have unpaid bags")
		return info, err
	}

	err = s.validateAvailableSpace(ctx, size)
	if err != nil {
		log.Error("Not enough disk space", slog.Any("error", err))
		err = models.NewAppError(models.ServiceUnavailableCode, "")
		return info, err
	}

	maxFilesCount, err := s.getLimits(ctx)
	if err != nil {
		log.Error("Failed to get limits", slog.Any("error", err))
		err = models.NewAppError(models.InternalServerErrorCode, "")
		return info, err
	}

	// Make dir
	id, uErr := uuid.NewV6()
	if uErr != nil {
		log.Error("Failed to generate UUID", slog.Any("error", uErr))
		err = models.NewAppError(models.InternalServerErrorCode, "")
		return info, err
	}

	dstPath := filepath.Join(s.storageDir, id.String())
	if oErr := os.MkdirAll(dstPath, 0755); oErr != nil {
		log.Error("Failed to create directory", slog.Any("error", oErr))
		err = models.NewAppError(models.InternalServerErrorCode, "")
		return info, err
	}

	// Remove the directory if handling an error
	defer func() {
		if err != nil {
			if rmErr := os.RemoveAll(dstPath); rmErr != nil {
				log.Error("Failed to remove directory after error", slog.Any("error", rmErr))
			}
		}
	}()

	// Parse multipart to disk
	description := ""
	rootDir := ""
	lastFileName := ""
	fileCount := 0
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Error("failed to read part", slog.Any("error", err))
			return info, fiber.NewError(fiber.StatusBadRequest, "invalid multipart")
		}

		name := part.FormName()
		if name == "" {
			continue
		}

		if maxFilesCount > 0 && fileCount >= maxFilesCount {
			msg := fmt.Sprintf("too many files (max %d)", maxFilesCount)
			log.Error(msg, "error", err, "file_count", fileCount)
			return info, fiber.NewError(fiber.StatusBadRequest, msg)
		}

		lastFileName = part.Header.Get("Content-Disposition")
		_, params, _ := mime.ParseMediaType(lastFileName)
		lastFileName = params["filename"]

		// Sanitize the filename
		lastFileName, err = sanitizePath(lastFileName)
		if err != nil {
			log.Error("Failed to sanitize filename", "error", err, "filename", lastFileName)
			return info, fiber.NewError(fiber.StatusBadRequest, "invalid filename")
		}

		// Write file to disk
		if lastFileName != "." {
			fileCount++

			fileData, err := io.ReadAll(part)
			if err != nil {
				msg := fmt.Sprintf("failed to read file %s part", lastFileName)
				log.Error(msg, "error", err, "filename", lastFileName)
				return info, fiber.NewError(fiber.StatusBadRequest, msg)
			}

			if strings.Contains(lastFileName, "/") || strings.Contains(lastFileName, "\\") {
				if rootDir == "" {
					rootDir = filepath.Dir(lastFileName)
					if i := strings.Index(rootDir, "/"); i != -1 {
						rootDir = rootDir[:i]
					}
				}
			}

			err = saveFileToDisk(dstPath, lastFileName, fileData)
			if err != nil {
				log.Error("failed to save file to disk", "error", err, "filename", lastFileName)
				return info, fiber.NewError(fiber.StatusInternalServerError, "internal error")
			}
		} else if name == "description" && description == "" {
			buf := new(bytes.Buffer)
			_, err := io.CopyN(buf, part, 10<<20)
			if err != nil && err != io.EOF {
				return info, fiber.NewError(fiber.StatusBadRequest, "description too large")
			}

			description = buf.String()
			a := utf8string.NewString(description)
			if a.RuneCount() > 100 {
				description = a.Slice(0, 100)
			}
		}
	}

	if fileCount == 0 {
		msg := "no files found"
		log.Error(msg, "error", err)
		return info, fiber.NewError(fiber.StatusBadRequest, msg)
	}

	path := filepath.Join(dstPath, rootDir)
	if fileCount == 1 && rootDir == "" {
		path = filepath.Join(path, lastFileName)
	}

	// Save to TON Storage
	bagInfo, err := s.saveToTONStorage(ctx, path, description, log)
	if err != nil {
		return info, err
	}

	bagid := bagInfo.BagID

	// Save bag info to database
	err = s.files.AddBag(ctx, db.BagInfo{
		BagID:       bagid,
		Description: description,
		Size:        bagInfo.BagSize,
		FilesSize:   bagInfo.Size,
	}, userAddr)
	if err != nil {
		log.Error("Failed to save bag info to database", "error", err.Error())
		err = models.NewAppError(models.InternalServerErrorCode, "")
		return info, err
	}

	log.Info("File added successfully", slog.String("bag_id", bagid))

	info.Bags = []v1.UserBagInfo{
		{
			BagID:       bagid,
			UserAddress: userAddr,
			CreatedAt:   time.Now().Unix(),
			Description: description,
			FilesCount:  bagInfo.FilesCount,
			BagSize:     bagInfo.BagSize,
		},
	}
	info.FreeStorage = uint64(s.unpaidFilesLifetime.Seconds())

	return info, nil
}

func (s *service) DeleteBag(ctx context.Context, bagID string, userAddr string) error {
	log := s.logger.With(
		slog.String("method", "DeleteBag"),
		slog.String("bag_id", bagID),
	)

	_, err := s.files.RemoveUserBagRelation(ctx, bagID, userAddr)
	if err != nil {
		log.Error("Failed to remove bag relation", "error", err)
		return models.NewAppError(models.InternalServerErrorCode, "")
	}

	// NOTE: File will be removed automatically by RemoveUnpaidFiles worker
	log.Info("Bag marked to be deleted successfully")

	return nil
}

func (s *service) MarkBagAsPaid(ctx context.Context, bagID, userAddress, storageContract string) (err error) {
	log := s.logger.With(
		slog.String("method", "MarkBagAsPaid"),
		slog.String("bag_id", bagID),
	)

	addr, err := address.ParseAddr(storageContract)
	if err != nil {
		log.Error("Failed to parse storage contract address", "error", err)
		return models.NewAppError(models.BadRequestErrorCode, "invalid contract address")
	}

	_, err = s.files.MarkBagAsPaid(ctx, bagID, userAddress, addr.String())
	if err != nil {
		log.Error("Failed to mark bag as paid", "error", err)
		return models.NewAppError(models.InternalServerErrorCode, "")
	}

	log.Info("Bag deleted by user successfully", slog.String("bag_id", bagID))
	return nil
}

func (s *service) GetUnpaidBags(ctx context.Context, userAddr string) (info v1.UnpaidBagsResponse, err error) {
	log := s.logger.With(
		slog.String("method", "GetUnpaidBags"),
		slog.String("user_address", userAddr),
	)

	unpaidBags, err := s.files.GetUnpaidBags(ctx, userAddr)
	if err != nil {
		log.Error("Failed to get unpaid bags", "error", err)
		err = models.NewAppError(models.InternalServerErrorCode, "")
		return
	}

	info.Bags = make([]v1.UserBagInfo, 0, len(unpaidBags))
	for _, bag := range unpaidBags {
		bagDetails, sErr := s.tonstorage.GetBag(ctx, bag.BagID)
		if sErr != nil {
			log.Error("Failed to get bag details", slog.Any("error", sErr))
			continue
		}

		info.Bags = append(info.Bags, v1.UserBagInfo{
			BagID:       bag.BagID,
			UserAddress: bag.UserAddress,
			CreatedAt:   bag.CreatedAt,
			Description: bagDetails.Description,
			FilesCount:  bagDetails.FilesCount,
			BagSize:     bagDetails.BagSize,
		})
	}

	info.FreeStorage = uint64(s.unpaidFilesLifetime.Seconds())

	return
}

func (s *service) GetBagsInfoShort(ctx context.Context, contracts []string) (info []v1.BagInfoShort, err error) {
	log := s.logger.With(
		slog.String("method", "GetBagsInfoShort"),
		slog.Int("bag_ids_count", len(contracts)),
	)

	if len(contracts) > descriptionsStoreLimit {
		contracts = contracts[:descriptionsStoreLimit]
	}

	desc, err := s.files.GetBagsInfoShort(ctx, contracts)
	if err != nil {
		log.Error("Failed to get bag descriptions", "error", err)
		return nil, models.NewAppError(models.InternalServerErrorCode, "")
	}

	info = make([]v1.BagInfoShort, 0, len(desc))
	for _, d := range desc {
		info = append(info, v1.BagInfoShort{
			ContractAddress: d.ContractAddress,
			BagID:           d.BagID,
			Description:     d.Description,
			Size:            d.Size,
		})
	}

	return info, nil
}

func (s *service) saveToTONStorage(ctx context.Context, path, description string, log *slog.Logger) (info *tonstorage.BagDetailed, err error) {
	// Save file(s) to TON Storage
	bagid, err := s.tonstorage.Create(ctx, description, path)
	if err != nil {
		log.Error("Failed to create file in storage", slog.Any("error", err))
		err = models.NewAppError(models.InternalServerErrorCode, "")
		return
	}

	// Get info
	info, err = s.tonstorage.GetBag(ctx, bagid)
	if err != nil {
		log.Error("Failed to get bag info", "error", err.Error())
		err = models.NewAppError(models.InternalServerErrorCode, "")
		return
	}

	return
}

func NewService(
	files filesDb,
	system systemDb,
	storage storage,
	storageDir string,
	totalDiskSpaceAvailable uint64,
	unpaidFilesLifetime time.Duration,
	logger *slog.Logger,
) Files {
	return &service{
		files:                   files,
		system:                  system,
		tonstorage:              storage,
		storageDir:              storageDir,
		totalDiskSpaceAvailable: totalDiskSpaceAvailable,
		unpaidFilesLifetime:     unpaidFilesLifetime,
		logger:                  logger,
	}
}
