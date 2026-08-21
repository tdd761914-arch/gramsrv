package giftclaim

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/xssnick/tonutils-go/address"
	tonwallet "github.com/xssnick/tonutils-go/ton/wallet"
	"go.uber.org/zap"

	"telesrv/internal/domain"
)

var (
	ErrUnauthorized = errors.New("Mini App authorization failed")
	ErrInvalid      = errors.New("gift claim request is invalid")
	ErrNotOwner     = errors.New("connected wallet is not the current NFT owner")
	ErrExpired      = errors.New("gift claim challenge expired")
)

type Store interface {
	ResolveOnChainGift(context.Context, string) (domain.UniqueStarGift, bool, error)
	ListOnChainGifts(context.Context, int64, int) ([]domain.UniqueStarGift, error)
	ReconcileOnChainOwner(context.Context, int64, string, string, string) (bool, error)
	ProfileUsername(context.Context, int64) (string, error)
	CreateChallenge(context.Context, int64, int64, time.Time, time.Duration) (domain.StarGiftClaimChallenge, error)
	ResolveChallenge(context.Context, string, int64, int) (domain.StarGiftClaimChallenge, bool, error)
	CommitClaim(context.Context, domain.StarGiftOnChainClaim) (domain.StarGiftOnChainClaimResult, error)
}

type Verifier interface {
	VerifyWalletProof(context.Context, string, string, tonwallet.TonConnectProof, []byte, time.Duration) error
	VerifyMint(context.Context, string, *big.Int, string) (string, error)
	CurrentNFTOwner(context.Context, string, *big.Int, string) (string, bool, error)
	Close()
}

type Config struct {
	PublicBaseURL         string
	BotToken              string
	Collection            string
	AppName               string
	ChallengeTTL          time.Duration
	ProofTTL              time.Duration
	InitDataTTL           time.Duration
	OwnershipSyncInterval time.Duration
	OwnershipSyncBatch    int
}

type Service struct {
	publicBaseURL         string
	proofDomain           string
	botToken              string
	collection            string
	appName               string
	challengeTTL          time.Duration
	proofTTL              time.Duration
	initDataTTL           time.Duration
	ownershipSyncInterval time.Duration
	ownershipSyncBatch    int
	store                 Store
	verifier              Verifier
	logger                *zap.Logger
}

type ChallengeResponse struct {
	Payload       string `json:"payload"`
	ExpiresAt     int    `json:"expires_at"`
	GiftTitle     string `json:"gift_title"`
	GiftSlug      string `json:"gift_slug"`
	NFTAddress    string `json:"nft_address"`
	WalletAddress string `json:"wallet_address"`
	OwnerProfile  string `json:"owner_profile"`
}

type ClaimInput struct {
	Payload string `json:"payload"`
	Account struct {
		Address         string `json:"address"`
		Chain           string `json:"chain"`
		PublicKey       string `json:"publicKey"`
		WalletStateInit string `json:"walletStateInit"`
	} `json:"account"`
	Proof struct {
		Timestamp int64 `json:"timestamp"`
		Domain    struct {
			LengthBytes uint32 `json:"lengthBytes"`
			Value       string `json:"value"`
		} `json:"domain"`
		Signature string `json:"signature"`
		Payload   string `json:"payload"`
	} `json:"proof"`
}

type ClaimResponse struct {
	OwnerProfile  string `json:"owner_profile"`
	OwnerUserID   int64  `json:"owner_user_id"`
	WalletAddress string `json:"wallet_address"`
	NFTAddress    string `json:"nft_address"`
	GiftSlug      string `json:"gift_slug"`
}

func New(cfg Config, store Store, verifier Verifier, logger *zap.Logger) (*Service, error) {
	base, err := url.Parse(strings.TrimSpace(cfg.PublicBaseURL))
	if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil {
		return nil, fmt.Errorf("gift claim public URL must be HTTPS")
	}
	botID, _, ok := domain.ParseBotToken(strings.TrimSpace(cfg.BotToken))
	if !ok || botID != domain.GiftClaimBotUserID {
		return nil, fmt.Errorf("gift claim bot token must belong to @claim")
	}
	if store == nil || verifier == nil || strings.TrimSpace(cfg.Collection) == "" {
		return nil, fmt.Errorf("gift claim dependencies are incomplete")
	}
	if cfg.ChallengeTTL == 0 {
		cfg.ChallengeTTL = 5 * time.Minute
	}
	if cfg.ProofTTL == 0 {
		cfg.ProofTTL = 5 * time.Minute
	}
	if cfg.InitDataTTL == 0 {
		cfg.InitDataTTL = 15 * time.Minute
	}
	if cfg.OwnershipSyncInterval == 0 {
		cfg.OwnershipSyncInterval = 15 * time.Second
	}
	if cfg.OwnershipSyncBatch == 0 {
		cfg.OwnershipSyncBatch = 100
	}
	if cfg.ChallengeTTL < time.Minute || cfg.ChallengeTTL > 15*time.Minute ||
		cfg.ProofTTL < time.Minute || cfg.ProofTTL > 15*time.Minute || cfg.InitDataTTL > 24*time.Hour ||
		cfg.OwnershipSyncInterval < 5*time.Second || cfg.OwnershipSyncInterval > time.Hour ||
		cfg.OwnershipSyncBatch < 1 || cfg.OwnershipSyncBatch > 1000 {
		return nil, fmt.Errorf("gift claim TTL is invalid")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	appName := strings.TrimSpace(cfg.AppName)
	if appName == "" {
		appName = "InvGram Gifts"
	}
	base.Path, base.RawQuery, base.Fragment = strings.TrimRight(base.Path, "/"), "", ""
	return &Service{
		publicBaseURL: strings.TrimRight(base.String(), "/"), proofDomain: base.Host,
		botToken: strings.TrimSpace(cfg.BotToken), collection: strings.TrimSpace(cfg.Collection), appName: appName,
		challengeTTL: cfg.ChallengeTTL, proofTTL: cfg.ProofTTL, initDataTTL: cfg.InitDataTTL,
		ownershipSyncInterval: cfg.OwnershipSyncInterval, ownershipSyncBatch: cfg.OwnershipSyncBatch,
		store: store, verifier: verifier, logger: logger,
	}, nil
}

func (s *Service) Close() {
	if s != nil && s.verifier != nil {
		s.verifier.Close()
	}
}

func (s *Service) authenticate(initData string, now time.Time) (webAppUser, error) {
	user, err := verifyInitData(initData, s.botToken, s.initDataTTL, now)
	if err != nil {
		return webAppUser{}, fmt.Errorf("%w: %v", ErrUnauthorized, err)
	}
	return user, nil
}

func (s *Service) Challenge(ctx context.Context, initData, giftRef string, now time.Time) (ChallengeResponse, error) {
	user, err := s.authenticate(initData, now)
	if err != nil {
		return ChallengeResponse{}, err
	}
	ref := normalizeGiftRef(giftRef)
	gift, found, err := s.store.ResolveOnChainGift(ctx, ref)
	if err != nil {
		return ChallengeResponse{}, err
	}
	if !found || gift.OwnerAddress == "" || gift.GiftAddress == "" || gift.Burned {
		return ChallengeResponse{}, ErrInvalid
	}
	challenge, err := s.store.CreateChallenge(ctx, user.ID, gift.ID, now, s.challengeTTL)
	if err != nil {
		return ChallengeResponse{}, err
	}
	ownerProfile := ""
	if gift.Host.Type == domain.PeerTypeUser && gift.Host.ID > 0 {
		if username, usernameErr := s.store.ProfileUsername(ctx, gift.Host.ID); usernameErr == nil && strings.TrimSpace(username) != "" {
			ownerProfile = "@" + strings.TrimPrefix(strings.TrimSpace(username), "@")
		} else {
			ownerProfile = fmt.Sprintf("user:%d", gift.Host.ID)
		}
	}
	return ChallengeResponse{Payload: challenge.Payload, ExpiresAt: challenge.ExpiresAt,
		GiftTitle: gift.Title, GiftSlug: gift.Slug, NFTAddress: gift.GiftAddress,
		WalletAddress: gift.OwnerAddress, OwnerProfile: ownerProfile}, nil
}

func (s *Service) Claim(ctx context.Context, initData string, input ClaimInput, now time.Time) (ClaimResponse, error) {
	user, err := s.authenticate(initData, now)
	if err != nil {
		return ClaimResponse{}, err
	}
	if input.Account.Chain != "-239" || input.Payload == "" || input.Proof.Payload != input.Payload {
		return ClaimResponse{}, ErrInvalid
	}
	challenge, found, err := s.store.ResolveChallenge(ctx, input.Payload, user.ID, int(now.Unix()))
	if err != nil {
		return ClaimResponse{}, err
	}
	if !found {
		return ClaimResponse{}, ErrExpired
	}
	expectedPreviousWallet, err := canonicalMainnetAddress(challenge.Unique.OwnerAddress)
	if err != nil {
		// A stale or malformed database projection must never be replaced by a
		// claim. Ownership sync can repair it before the user retries.
		return ClaimResponse{}, ErrInvalid
	}
	walletAddress, err := canonicalMainnetAddress(input.Account.Address)
	if err != nil {
		return ClaimResponse{}, ErrInvalid
	}
	signature, err := decodeBase64(input.Proof.Signature, 64, 64)
	if err != nil {
		return ClaimResponse{}, ErrInvalid
	}
	stateInit, err := decodeBase64(input.Account.WalletStateInit, 1, 32<<10)
	if err != nil {
		return ClaimResponse{}, ErrInvalid
	}
	proof := tonwallet.TonConnectProof{Timestamp: input.Proof.Timestamp, Signature: signature, Payload: input.Proof.Payload}
	proof.Domain.LengthBytes, proof.Domain.Value = input.Proof.Domain.LengthBytes, input.Proof.Domain.Value
	if err := s.verifier.VerifyWalletProof(ctx, walletAddress, s.proofDomain, proof, stateInit, s.proofTTL); err != nil {
		s.logger.Info("TON Proof rejected", zap.Int64("user_id", user.ID), zap.Error(err))
		return ClaimResponse{}, ErrUnauthorized
	}
	itemAddress, err := s.verifier.VerifyMint(ctx, s.collection, big.NewInt(challenge.Unique.ID), walletAddress)
	if err != nil || itemAddress != challenge.Unique.GiftAddress {
		return ClaimResponse{}, ErrNotOwner
	}
	// VerifyMint checks the collection/index/owner tuple. Read the owner once
	// more immediately before the database CAS so a transfer observed between
	// the first chain read and the commit fails closed instead of publishing a
	// stale profile projection. The ownership watcher remains the eventual
	// repair path for a transfer occurring after this final read.
	currentOwner, activeCollection, ownerErr := s.verifier.CurrentNFTOwner(
		ctx, s.collection, big.NewInt(challenge.Unique.ID), itemAddress,
	)
	if ownerErr != nil || !activeCollection {
		return ClaimResponse{}, ErrNotOwner
	}
	currentOwner, err = canonicalMainnetAddress(currentOwner)
	if err != nil || currentOwner != walletAddress {
		return ClaimResponse{}, ErrNotOwner
	}
	result, err := s.store.CommitClaim(ctx, domain.StarGiftOnChainClaim{
		Payload: input.Payload, UserID: user.ID, UniqueGiftID: challenge.Unique.ID,
		ExpectedPreviousWallet: expectedPreviousWallet,
		WalletAddress:          walletAddress, GiftAddress: itemAddress, ClaimedAt: int(now.Unix()),
	})
	if err != nil {
		return ClaimResponse{}, err
	}
	profile := "@" + strings.TrimPrefix(strings.TrimSpace(result.ProfileUsername), "@")
	return ClaimResponse{OwnerProfile: profile, OwnerUserID: user.ID, WalletAddress: walletAddress,
		NFTAddress: itemAddress, GiftSlug: result.Gift.Slug}, nil
}

func normalizeGiftRef(ref string) string {
	ref = strings.TrimSpace(strings.TrimPrefix(ref, "https://"))
	if parsed, err := canonicalMainnetAddress(ref); err == nil {
		return parsed
	}
	return strings.TrimPrefix(ref, "nft/")
}

func canonicalMainnetAddress(raw string) (string, error) {
	var a *address.Address
	var err error
	if strings.Contains(strings.TrimSpace(raw), ":") {
		a, err = address.ParseRawAddr(strings.TrimSpace(raw))
	} else {
		a, err = address.ParseAddr(strings.TrimSpace(raw))
	}
	if err != nil || a == nil || a.IsAddrNone() || a.Workchain() != 0 || a.IsTestnetOnly() {
		return "", errors.New("invalid mainnet address")
	}
	return a.StringRaw(), nil
}

func decodeBase64(value string, minLen, maxLen int) ([]byte, error) {
	value = strings.TrimSpace(value)
	var out []byte
	var err error
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		out, err = encoding.DecodeString(value)
		if err == nil {
			break
		}
	}
	if err != nil || len(out) < minLen || len(out) > maxLen {
		return nil, errors.New("invalid base64")
	}
	return out, nil
}
