package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"telesrv/internal/domain"
)

type StarGiftClaimStore struct {
	db *pgxpool.Pool
}

func NewStarGiftClaimStore(db *pgxpool.Pool) *StarGiftClaimStore {
	return &StarGiftClaimStore{db: db}
}

func (s *StarGiftClaimStore) ProfileUsername(ctx context.Context, userID int64) (string, error) {
	var username string
	if s == nil || s.db == nil || userID <= 0 {
		return "", domain.ErrStarGiftOwnerInvalid
	}
	if err := s.db.QueryRow(ctx, `SELECT username FROM users WHERE id=$1`, userID).Scan(&username); err != nil {
		return "", err
	}
	return username, nil
}

func (s *StarGiftClaimStore) ResolveOnChainGift(ctx context.Context, ref string) (domain.UniqueStarGift, bool, error) {
	ref = strings.TrimSpace(ref)
	if s == nil || s.db == nil || ref == "" || len(ref) > 128 {
		return domain.UniqueStarGift{}, false, nil
	}
	var id int64
	err := s.db.QueryRow(ctx, `SELECT id FROM unique_star_gifts
WHERE (lower(slug)=lower($1) OR gift_address=$1)
  AND owner_address<>'' AND gift_address<>'' AND NOT burned`, ref).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.UniqueStarGift{}, false, nil
	}
	if err != nil {
		return domain.UniqueStarGift{}, false, fmt.Errorf("resolve on-chain star gift: %w", err)
	}
	return NewStarGiftStore(s.db).UniqueByID(ctx, id)
}

// ListOnChainGifts returns a bounded page of exported collectibles. The
// verifier independently checks that each item belongs to the active TON
// collection before any record can be changed.
func (s *StarGiftClaimStore) ListOnChainGifts(ctx context.Context, afterID int64, limit int) ([]domain.UniqueStarGift, error) {
	if s == nil || s.db == nil || afterID < 0 || limit <= 0 {
		return []domain.UniqueStarGift{}, nil
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := s.db.Query(ctx, `SELECT u.id,u.slug,u.owner_address,u.gift_address
FROM unique_star_gifts u
JOIN peer_star_gifts p ON p.id=u.source_saved_gift_id
WHERE u.id>$1 AND p.lifecycle_status='exported'
  AND u.owner_address<>'' AND u.gift_address<>'' AND NOT u.burned
ORDER BY u.id LIMIT $2`, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("list on-chain star gifts: %w", err)
	}
	defer rows.Close()
	gifts := make([]domain.UniqueStarGift, 0, limit)
	for rows.Next() {
		var gift domain.UniqueStarGift
		if err := rows.Scan(&gift.ID, &gift.Slug, &gift.OwnerAddress, &gift.GiftAddress); err != nil {
			return nil, fmt.Errorf("scan on-chain star gift: %w", err)
		}
		gifts = append(gifts, gift)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate on-chain star gifts: %w", err)
	}
	return gifts, nil
}

// ReconcileOnChainOwner applies a compare-and-swap after the verifier observes
// an NFT transfer. It updates the wallet field and detaches the collectible
// from the former profile by returning its host to the @relayer service user.
func (s *StarGiftClaimStore) ReconcileOnChainOwner(ctx context.Context, uniqueGiftID int64, giftAddress, previousWallet, wallet string) (bool, error) {
	if s == nil || s.db == nil || uniqueGiftID <= 0 || strings.TrimSpace(giftAddress) == "" ||
		strings.TrimSpace(previousWallet) == "" || strings.TrimSpace(wallet) == "" || previousWallet == wallet {
		return false, domain.ErrStarGiftInvalid
	}
	result, err := s.db.Exec(ctx, `UPDATE unique_star_gifts u
SET owner_address=$4,host_peer_type='user',host_peer_id=$5,updated_at=now()
FROM peer_star_gifts p
WHERE u.id=$1 AND u.gift_address=$2 AND u.owner_address=$3 AND NOT u.burned
  AND p.id=u.source_saved_gift_id AND p.lifecycle_status='exported'`,
		uniqueGiftID, giftAddress, previousWallet, wallet, domain.GiftRelayerUserID)
	if err != nil {
		return false, fmt.Errorf("reconcile on-chain star gift owner: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

func (s *StarGiftClaimStore) CreateChallenge(ctx context.Context, userID, uniqueGiftID int64, now time.Time, ttl time.Duration) (domain.StarGiftClaimChallenge, error) {
	if s == nil || s.db == nil || userID <= 0 || uniqueGiftID <= 0 || ttl <= 0 {
		return domain.StarGiftClaimChallenge{}, domain.ErrStarGiftInvalid
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return domain.StarGiftClaimChallenge{}, fmt.Errorf("generate star gift claim payload: %w", err)
	}
	payload := hex.EncodeToString(random[:])
	expiresAt := int(now.Add(ttl).Unix())
	if _, err := s.db.Exec(ctx, `INSERT INTO star_gift_claim_challenges(payload,user_id,unique_gift_id,expires_at)
VALUES($1,$2,$3,$4)`, payload, userID, uniqueGiftID, expiresAt); err != nil {
		return domain.StarGiftClaimChallenge{}, fmt.Errorf("create star gift claim challenge: %w", err)
	}
	unique, found, err := NewStarGiftStore(s.db).UniqueByID(ctx, uniqueGiftID)
	if err != nil || !found {
		return domain.StarGiftClaimChallenge{}, domain.ErrStarGiftNotFound
	}
	return domain.StarGiftClaimChallenge{Payload: payload, UserID: userID, Unique: unique, ExpiresAt: expiresAt}, nil
}

func (s *StarGiftClaimStore) ResolveChallenge(ctx context.Context, payload string, userID int64, now int) (domain.StarGiftClaimChallenge, bool, error) {
	payload = strings.TrimSpace(payload)
	if s == nil || s.db == nil || len(payload) != 64 || userID <= 0 || now <= 0 {
		return domain.StarGiftClaimChallenge{}, false, nil
	}
	var uniqueID int64
	var expiresAt int
	err := s.db.QueryRow(ctx, `SELECT unique_gift_id,expires_at FROM star_gift_claim_challenges
WHERE payload=$1 AND user_id=$2 AND consumed_at IS NULL AND expires_at>$3`, payload, userID, now).
		Scan(&uniqueID, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StarGiftClaimChallenge{}, false, nil
	}
	if err != nil {
		return domain.StarGiftClaimChallenge{}, false, fmt.Errorf("resolve star gift claim challenge: %w", err)
	}
	unique, found, err := NewStarGiftStore(s.db).UniqueByID(ctx, uniqueID)
	if err != nil || !found {
		return domain.StarGiftClaimChallenge{}, false, err
	}
	return domain.StarGiftClaimChallenge{Payload: payload, UserID: userID, Unique: unique, ExpiresAt: expiresAt}, true, nil
}

func (s *StarGiftClaimStore) CommitClaim(ctx context.Context, req domain.StarGiftOnChainClaim) (domain.StarGiftOnChainClaimResult, error) {
	if s == nil || s.db == nil || len(req.Payload) != 64 || req.UserID <= 0 || req.UniqueGiftID <= 0 ||
		strings.TrimSpace(req.ExpectedPreviousWallet) == "" || strings.TrimSpace(req.WalletAddress) == "" ||
		strings.TrimSpace(req.GiftAddress) == "" || req.ClaimedAt <= 0 {
		return domain.StarGiftOnChainClaimResult{}, domain.ErrStarGiftInvalid
	}
	var previousWallet, username string
	err := withTx(ctx, s.db, "claim on-chain star gift", func(tx pgx.Tx) error {
		var challengeGiftID int64
		if err := tx.QueryRow(ctx, `UPDATE star_gift_claim_challenges SET consumed_at=$3
WHERE payload=$1 AND user_id=$2 AND consumed_at IS NULL AND expires_at>$3
RETURNING unique_gift_id`, req.Payload, req.UserID, req.ClaimedAt).Scan(&challengeGiftID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrStarGiftUnavailable
			}
			return err
		}
		if challengeGiftID != req.UniqueGiftID {
			return domain.ErrStarGiftInvalid
		}
		var giftAddress string
		var burned bool
		if err := tx.QueryRow(ctx, `SELECT owner_address,gift_address,burned FROM unique_star_gifts
WHERE id=$1 FOR UPDATE`, req.UniqueGiftID).Scan(&previousWallet, &giftAddress, &burned); err != nil {
			return err
		}
		if burned || previousWallet == "" || previousWallet != req.ExpectedPreviousWallet || giftAddress != req.GiftAddress {
			return domain.ErrStarGiftUnavailable
		}
		if err := tx.QueryRow(ctx, `SELECT username FROM users WHERE id=$1`, req.UserID).Scan(&username); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE unique_star_gifts
SET owner_peer_type=NULL,owner_peer_id=NULL,owner_address=$2,
    host_peer_type='user',host_peer_id=$3,updated_at=now()
WHERE id=$1`, req.UniqueGiftID, req.WalletAddress, req.UserID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO star_gift_claim_history(
unique_gift_id,profile_user_id,wallet_address,previous_wallet_address,gift_address,claimed_at,challenge_payload)
VALUES($1,$2,$3,$4,$5,$6,$7)`, req.UniqueGiftID, req.UserID, req.WalletAddress,
			previousWallet, req.GiftAddress, req.ClaimedAt, req.Payload)
		return err
	})
	if err != nil {
		return domain.StarGiftOnChainClaimResult{}, err
	}
	gift, found, err := NewStarGiftStore(s.db).UniqueByID(ctx, req.UniqueGiftID)
	if err != nil || !found {
		return domain.StarGiftOnChainClaimResult{}, domain.ErrStarGiftNotFound
	}
	return domain.StarGiftOnChainClaimResult{Gift: gift, PreviousWallet: previousWallet, ProfileUsername: username}, nil
}
