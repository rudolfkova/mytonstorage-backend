package files

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mytonstorage-backend/pkg/models/db"
)

type repository struct {
	db *pgxpool.Pool
}

type Repository interface {
	AddBag(ctx context.Context, bag db.BagInfo, userAddr string) error
	RemoveUserBagRelation(ctx context.Context, bagID, userAddress string) (int64, error)
	RemoveUnpaidBagsRelations(ctx context.Context, sec uint64) (bagids []string, err error)
	RemoveUnusedBags(ctx context.Context) (removed []string, err error)
	RemoveNotifiedBags(ctx context.Context, limit int, sec uint64, maxNotifyAttempts int, maxDownloadChecks int) (removed []string, err error)
	CanUpload(ctx context.Context, userID string, sec uint64) (bool, error)
	GetUnpaidBags(ctx context.Context, userID string) ([]db.UserBagInfo, error)
	IsBagExpired(ctx context.Context, bagID string, userAddress string, sec uint64) (expired bool, err error)
	MarkBagAsPaid(ctx context.Context, bagID, userAddress, storageContract string) (int64, error)
	GetPaidBag(ctx context.Context, userAddress, storageContract string) (db.BagStorageContract, error)

	GetBagsInfoShort(ctx context.Context, bagIDs []string) ([]db.BagDescription, error)

	GetNotifyInfo(ctx context.Context, limit int, notifyAttempts int) ([]db.BagStorageContract, error)
	IncreaseAttempts(ctx context.Context, bags []db.BagStorageContract) error
}

func (r *repository) AddBag(ctx context.Context, bag db.BagInfo, userAddr string) error {
	query := `
		WITH add_file AS (
			INSERT INTO files.bags (bagid, description, size, files_size, created_at)
			VALUES ($1, $2, $3, $4, NOW())
			ON CONFLICT (bagid) DO NOTHING
			RETURNING bagid
		)
		INSERT INTO files.bag_users (bagid, user_address, storage_contract, created_at, updated_at)
		VALUES ($1, $5, NULL, NOW(), NOW())
		ON CONFLICT (bagid, user_address) DO UPDATE 
			SET updated_at = NOW();
	`
	_, err := r.db.Exec(ctx, query, bag.BagID, bag.Description, bag.Size, bag.FilesSize, userAddr)
	return err
}

func (r *repository) RemoveUnusedBags(ctx context.Context) (removed []string, err error) {
	query := `
		WITH to_remove AS (
			SELECT b.bagid
			FROM files.bags b
				LEFT JOIN files.bag_users bu ON b.bagid = bu.bagid
			WHERE bu.bagid IS NULL
		),
		remove AS (
			DELETE FROM files.bags
			WHERE bagid IN (SELECT bagid FROM to_remove)
			RETURNING bagid
		)
		SELECT bagid FROM remove;
	`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var bagID string
		if err := rows.Scan(&bagID); err != nil {
			return nil, err
		}
		removed = append(removed, bagID)
	}

	return removed, nil
}

func (r *repository) RemoveUserBagRelation(ctx context.Context, bagID, userAddress string) (cnt int64, err error) {
	query := `
		DELETE FROM files.bag_users
		WHERE bagid = $1 AND user_address = $2;
	`
	row, err := r.db.Exec(ctx, query, bagID, userAddress)
	if err != nil {
		return
	}

	cnt = row.RowsAffected()

	return
}

func (r *repository) RemoveUnpaidBagsRelations(ctx context.Context, sec uint64) (bagids []string, err error) {
	query := `
		WITH to_remove AS (
			SELECT bu.bagid, bu.user_address
			FROM files.bag_users bu
			WHERE bu.storage_contract IS NULL 
				AND EXTRACT(EPOCH FROM (NOW() - bu.created_at)) > $1
		),
		remove AS (
			DELETE FROM files.bag_users
			WHERE (bagid, user_address) IN (SELECT bagid, user_address FROM to_remove)
			RETURNING bagid
		)
		SELECT DISTINCT bagid FROM remove
	`
	rows, err := r.db.Query(ctx, query, sec)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var bagID string
		if err := rows.Scan(&bagID); err != nil {
			return nil, err
		}
		bagids = append(bagids, bagID)
	}

	return bagids, nil
}

func (r *repository) RemoveNotifiedBags(ctx context.Context, limit int, sec uint64, maxNotifyAttempts int, maxDownloadChecks int) (removed []string, err error) {
	query := `
		WITH cte AS (
			SELECT 
				n.provider_pubkey, 
				n.storage_contract, 
				n.bagid, 
				( 
					(
						(NOT n.notified AND n.notify_attempts >= $1) -- failed to notify after N attempts
						OR (n.notified AND n.download_checks >= $2) -- failed download check after N attempts
						OR (n.size = n.downloaded) -- fully downloaded
					)
					AND EXTRACT(EPOCH FROM (NOW() - n.updated_at)) > $3
				) as can_delete
			FROM providers.notifications n 
		), del AS (
			SELECT c.provider_pubkey, c.storage_contract
			FROM cte c
			WHERE c.bagid IN (
				SELECT bagid 
				FROM cte 
				GROUP BY bagid 
				HAVING MIN(can_delete::int) = 1  -- for one bag all can_delete must be true
			)
			LIMIT $4
		)
		DELETE FROM providers.notifications n
		USING del
		WHERE (n.provider_pubkey, n.storage_contract) = (del.provider_pubkey, del.storage_contract)
		RETURNING n.bagid;`

	rows, err := r.db.Query(ctx, query, maxNotifyAttempts, maxDownloadChecks, sec, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var bagID string
		if err := rows.Scan(&bagID); err != nil {
			return nil, err
		}
		removed = append(removed, bagID)
	}

	return removed, nil
}

func (r *repository) CanUpload(ctx context.Context, userID string, sec uint64) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1
			FROM files.bag_users
			WHERE user_address = $1 
				AND storage_contract IS NULL 
				AND (NOW() - created_at) < $2
		)
	`

	var hasUnpaid bool
	err := r.db.QueryRow(ctx, query, userID, time.Duration(sec)*time.Second).Scan(&hasUnpaid)
	if err != nil {
		return false, err
	}

	return !hasUnpaid, nil
}

func (r *repository) GetUnpaidBags(ctx context.Context, userID string) ([]db.UserBagInfo, error) {
	var bags []db.UserBagInfo
	query := `
		SELECT bagid, user_address, created_at
		FROM files.bag_users
		WHERE user_address = $1 AND storage_contract IS NULL;
	`
	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var bag db.UserBagInfo
		var createdAt *time.Time
		if err := rows.Scan(&bag.BagID, &bag.UserAddress, &createdAt); err != nil {
			return nil, err
		}
		bag.CreatedAt = createdAt.Unix()
		bags = append(bags, bag)
	}

	return bags, nil
}

func (r *repository) IsBagExpired(ctx context.Context, bagID string, userAddress string, sec uint64) (expired bool, err error) {
	query := `
		SELECT EXISTS (
			SELECT 1
			FROM files.bag_users
			WHERE bagid = $1 
				AND user_address = $2 
				AND storage_contract IS NULL 
				AND EXTRACT(EPOCH FROM (NOW() - created_at)) > $3
		);
	`
	err = r.db.QueryRow(ctx, query, bagID, userAddress, sec).Scan(&expired)
	return
}

func (r *repository) MarkBagAsPaid(ctx context.Context, bagID, userAddress, storageContract string) (cnt int64, err error) {
	query := `
		UPDATE files.bag_users
		SET storage_contract = $3
		WHERE bagid = $1 AND user_address = $2
		RETURNING 1;
	`
	row, err := r.db.Exec(ctx, query, bagID, userAddress, storageContract)
	if err != nil {
		return
	}

	cnt = row.RowsAffected()

	return
}

func (r *repository) GetPaidBag(ctx context.Context, userAddress, storageContract string) (info db.BagStorageContract, err error) {
	query := `
		SELECT bu.bagid, bu.storage_contract, b.files_size
		FROM files.bag_users bu
			JOIN files.bags b ON b.bagid = bu.bagid
		WHERE bu.user_address = $1
			AND bu.storage_contract = $2
		LIMIT 1;
	`
	err = r.db.QueryRow(ctx, query, userAddress, storageContract).Scan(&info.BagID, &info.StorageContract, &info.FilesSize)
	if err == pgx.ErrNoRows {
		err = nil
	}
	return
}

func (r *repository) GetBagsInfoShort(ctx context.Context, contracts []string) (descriptions []db.BagDescription, err error) {
	query := `
		SELECT bu.storage_contract, b.bagid, b.description, b.size
		FROM files.bag_users bu 
			JOIN files.bags b ON b.bagid = bu.bagid
		WHERE bu.storage_contract = ANY($1::text[])
	`

	rows, err := r.db.Query(ctx, query, contracts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var desc db.BagDescription
		if err := rows.Scan(&desc.ContractAddress, &desc.BagID, &desc.Description, &desc.Size); err != nil {
			return nil, err
		}
		descriptions = append(descriptions, desc)
	}

	return descriptions, nil
}

func (r *repository) GetNotifyInfo(ctx context.Context, limit int, notifyAttempts int) (resp []db.BagStorageContract, err error) {
	var info db.BagStorageContract
	query := `
		SELECT bu.bagid, bu.storage_contract, b.files_size
		FROM files.bag_users bu
			JOIN files.bags b ON b.bagid=bu.bagid
		WHERE bu.storage_contract IS NOT NULL 
			AND bu.notify_attempts >= 0 
			AND bu.notify_attempts < $2
		LIMIT $1;
	`
	rows, err := r.db.Query(ctx, query, limit, notifyAttempts)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		if sErr := rows.Scan(&info.BagID, &info.StorageContract, &info.FilesSize); sErr != nil {
			err = sErr
			return
		}
		resp = append(resp, info)
	}
	return resp, nil
}

func (r *repository) IncreaseAttempts(ctx context.Context, bags []db.BagStorageContract) (err error) {
	query := `
		WITH cte AS (
			SELECT x.bagid, x.storage_contract
			FROM jsonb_array_elements($1::jsonb) AS x(bagid, storage_contract)
		)
		UPDATE files.bag_users
		SET notify_attempts = notify_attempts + 1
		WHERE (bagid, storage_contract) IN (SELECT bagid, storage_contract FROM cte)
	`
	_, err = r.db.Exec(ctx, query, bags)
	return
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{
		db: db,
	}
}
