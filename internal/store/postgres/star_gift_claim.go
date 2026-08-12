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
		strings.TrimSpace(req.WalletAddress) == "" || strings.TrimSpace(req.GiftAddress) == "" || req.ClaimedAt <= 0 {
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
		if burned || previousWallet == "" || giftAddress != req.GiftAddress {
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
