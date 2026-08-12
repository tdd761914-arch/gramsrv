package giftclaim

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClaimPageUsesHTTPSWalletHandoff(t *testing.T) {
	service := &Service{appName: "InvGram Gifts", publicBaseURL: "https://claim.example"}
	page := httptest.NewRecorder()
	service.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/claim", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("claim page status = %d", page.Code)
	}
	body := page.Body.String()
	for _, expected := range []string{"tc.connector.connect", "tg.openLink", "parsed.protocol!=='https:'", "{request:{tonProof:challenge.payload}}"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("claim page does not contain %q", expected)
		}
	}
	for _, forbidden := range []string{"tc.openModal", "buttonRootId:'wallet'"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("claim page still contains %q", forbidden)
		}
	}

	manifest := httptest.NewRecorder()
	service.ServeHTTP(manifest, httptest.NewRequest(http.MethodGet, "/claim/tonconnect-manifest.json", nil))
	if manifest.Code != http.StatusOK || !strings.Contains(manifest.Body.String(), `"iconUrl":"https://claim.example/custom-fragment/media/gift/owl-1.png"`) {
		t.Fatalf("manifest status=%d body=%q", manifest.Code, manifest.Body.String())
	}
}
