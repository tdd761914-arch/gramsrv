package customfragment

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tvm/cell"

	"telesrv/internal/domain"
)

type fakeLedger struct {
	withdrawal domain.StarGiftWithdrawal
	completed  bool
	owner      string
	item       string
}

func (f *fakeLedger) ResolveWithdrawal(_ context.Context, requestID string) (domain.StarGiftWithdrawal, bool, error) {
	if requestID != f.withdrawal.ProviderRequestID {
		return domain.StarGiftWithdrawal{}, false, nil
	}
	return f.withdrawal, true, nil
}

func (f *fakeLedger) CompleteWithdrawalOnChain(_ context.Context, requestID, owner, item string, _ int) (domain.StarGiftWithdrawal, error) {
	if requestID != f.withdrawal.ProviderRequestID {
		return domain.StarGiftWithdrawal{}, ErrInvalidRequest
	}
	f.completed, f.owner, f.item = true, owner, item
	f.withdrawal.Status = "completed"
	f.withdrawal.Gift.OwnerAddress = owner
	f.withdrawal.Gift.GiftAddress = item
	return f.withdrawal, nil
}

func (f *fakeLedger) UniqueBySlug(_ context.Context, slug string) (domain.UniqueStarGift, bool, error) {
	return f.withdrawal.Gift, slug == f.withdrawal.Gift.Slug, nil
}

type fakeVerifier struct {
	item       string
	collection string
	owner      string
	index      *big.Int
	err        error
}

func (f *fakeVerifier) VerifyMint(_ context.Context, collection string, index *big.Int, owner string) (string, error) {
	f.collection, f.index, f.owner = collection, new(big.Int).Set(index), owner
	return f.item, f.err
}
func (*fakeVerifier) Close() {}

func testAddress(fill byte) *address.Address {
	data := make([]byte, 32)
	for i := range data {
		data[i] = fill
	}
	return address.NewAddress(0, 0, data)
}

func testService(t *testing.T, now time.Time) (*Service, *fakeLedger, *fakeVerifier, *address.Address) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(strings.NewReader(strings.Repeat("s", 64)))
	if err != nil {
		t.Fatal(err)
	}
	collection, owner, item := testAddress(1), testAddress(2), testAddress(3)
	ledger := &fakeLedger{withdrawal: domain.StarGiftWithdrawal{
		ProviderRequestID: "withdraw-token", ExpiresAt: int(now.Add(10 * time.Minute).Unix()), Status: "pending",
		Gift: domain.UniqueStarGift{ID: 42, GiftID: 7, Title: "Crystal Star", Slug: "CrystalStar-42", Num: 42,
			Model:    domain.StarGiftCollectibleAttribute{Name: "Azure"},
			Pattern:  domain.StarGiftCollectibleAttribute{Name: "Comet"},
			Backdrop: domain.StarGiftCollectibleAttribute{Name: "Midnight"}},
	}}
	verifier := &fakeVerifier{item: item.StringRaw()}
	service, err := New(Config{
		PublicBaseURL: "https://grams.example", AppName: "Gramsrv", SigningKey: privateKey,
		GiftCollection: collection.String(), MintAmountNanoton: 80_000_000,
		SubwalletID: defaultSubwalletID, AuthorizationTTL: 5 * time.Minute,
	}, ledger, verifier, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	return service, ledger, verifier, owner
}

func TestIntentBuildsTolkCompatibleSignedPayload(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	service, _, _, owner := testService(t, now)
	intent, err := service.Intent(context.Background(), "withdraw-token", owner.String(), now)
	if err != nil {
		t.Fatal(err)
	}
	if intent.Network != mainnetChain || intent.Amount != "80000000" || intent.WalletAddress != owner.StringRaw() || intent.ItemIndex != "42" {
		t.Fatalf("unexpected intent: %+v", intent)
	}
	boc, err := base64.StdEncoding.DecodeString(intent.Payload)
	if err != nil {
		t.Fatal(err)
	}
	body, err := cell.FromBOC(boc)
	if err != nil {
		t.Fatal(err)
	}
	slice, err := body.BeginParse()
	if err != nil {
		t.Fatal(err)
	}
	opcode, err := slice.LoadUInt(32)
	if err != nil || opcode != mintOpcode {
		t.Fatalf("opcode=%x err=%v", opcode, err)
	}
	signature, err := slice.LoadSlice(512)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := slice.LoadRefCell()
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(service.publicKey, authorization.Hash(), signature) {
		t.Fatal("payload signature does not cover the authorization cell")
	}
	auth, err := authorization.BeginParse()
	if err != nil {
		t.Fatal(err)
	}
	subwallet, _ := auth.LoadUInt(32)
	_, _ = auth.LoadUInt(32)
	validTill, _ := auth.LoadUInt(32)
	index, _ := auth.LoadBigUInt(256)
	parsedOwner, err := auth.LoadAddr()
	if err != nil {
		t.Fatal(err)
	}
	content, err := auth.LoadRefCell()
	if err != nil {
		t.Fatal(err)
	}
	contentSlice, err := content.BeginParse()
	if err != nil {
		t.Fatal(err)
	}
	prefix, _ := contentSlice.LoadUInt(8)
	metadata, err := contentSlice.LoadStringSnake()
	if err != nil {
		t.Fatal(err)
	}
	if subwallet != uint64(defaultSubwalletID) || validTill != uint64(intent.ValidUntil) ||
		index.Cmp(big.NewInt(42)) != 0 || !parsedOwner.Equals(owner) || prefix != 1 ||
		metadata != "https://grams.example/custom-fragment/metadata/gift/CrystalStar-42.json" {
		t.Fatalf("authorization mismatch: subwallet=%x validTill=%d index=%s owner=%s prefix=%d metadata=%q",
			subwallet, validTill, index, parsedOwner.StringRaw(), prefix, metadata)
	}
}

func TestConfirmCompletesOnlyAfterVerifierMatches(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	service, ledger, verifier, owner := testService(t, now)
	confirmation, err := service.Confirm(context.Background(), "withdraw-token", owner.String(), now)
	if err != nil {
		t.Fatal(err)
	}
	if !ledger.completed || verifier.index.Cmp(big.NewInt(42)) != 0 || verifier.owner != owner.StringRaw() ||
		confirmation.Status != "completed" || confirmation.GiftAddress != verifier.item {
		t.Fatalf("confirmation=%+v ledger=%+v verifier=%+v", confirmation, ledger, verifier)
	}
}

func TestHTTPPageAndMetadata(t *testing.T) {
	now := time.Now()
	service, _, _, _ := testService(t, now)
	page := httptest.NewRecorder()
	service.ServeWithdrawalPage(page, httptest.NewRequest(http.MethodGet, "/gift-withdrawal/withdraw-token", nil), "withdraw-token")
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "TON Mainnet") || !strings.Contains(page.Body.String(), "Mint gift on TON") {
		t.Fatalf("page status=%d body=%q", page.Code, page.Body.String())
	}
	if strings.Contains(page.Body.String(), string(service.privateKey)) {
		t.Fatal("withdrawal page exposed the private signing key")
	}

	metadata := httptest.NewRecorder()
	service.ServeHTTP(metadata, httptest.NewRequest(http.MethodGet, "/custom-fragment/metadata/gift/CrystalStar-42.json", nil))
	if metadata.Code != http.StatusOK || !strings.Contains(metadata.Body.String(), `"name":"Crystal Star #42"`) || !strings.Contains(metadata.Body.String(), `"trait_type":"Model"`) {
		t.Fatalf("metadata status=%d body=%q", metadata.Code, metadata.Body.String())
	}
}

func TestParseMainnetAddressRejectsTestnetAndMasterchain(t *testing.T) {
	if _, err := parseMainnetAddress(testAddress(4).Testnet(true).String()); err == nil {
		t.Fatal("accepted a testnet-only friendly address")
	}
	masterchain := address.NewAddress(0, byte(0xff), make([]byte, 32))
	if _, err := parseMainnetAddress(masterchain.StringRaw()); err == nil {
		t.Fatal("accepted a masterchain address")
	}
}
