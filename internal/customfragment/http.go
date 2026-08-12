package customfragment

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"telesrv/internal/domain"
)

type withdrawalPage struct {
	AppName      string
	RequestID    string
	Title        string
	Slug         string
	Number       int
	Status       string
	ExpiresAt    string
	Pending      bool
	Completed    bool
	OwnerAddress string
	GiftAddress  string
}

var withdrawalTemplate = template.Must(template.New("customfragment-withdrawal").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<meta name="color-scheme" content="dark"><title>{{.Title}} · CustomFragment</title>
<script defer src="https://unpkg.com/@tonconnect/ui@2.3.1/dist/tonconnect-ui.min.js"></script>
<style>
:root{font-family:Inter,ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color:#f7f9ff;background:#080d18}*{box-sizing:border-box}body{min-height:100vh;margin:0;background:radial-gradient(circle at 50% -10%,#1d68a9 0,#101a31 34%,#080d18 70%);padding:calc(24px + env(safe-area-inset-top)) 18px calc(30px + env(safe-area-inset-bottom))}.shell{width:min(100%,520px);margin:0 auto}.brand{display:flex;align-items:center;gap:10px;color:#a9b8d2;font-size:14px;font-weight:700;letter-spacing:.02em;margin:4px 0 22px}.brand i{width:34px;height:34px;border-radius:11px;background:linear-gradient(145deg,#38a6ff,#7868ff);display:grid;place-items:center;color:white;font-style:normal;box-shadow:0 8px 24px #248cff55}.card{position:relative;overflow:hidden;border:1px solid #ffffff18;background:#111a2be6;backdrop-filter:blur(22px);border-radius:28px;padding:28px;box-shadow:0 30px 90px #0008}.orb{width:154px;height:154px;margin:2px auto 24px;border-radius:50%;display:grid;place-items:center;background:linear-gradient(145deg,#49b6ff,#6964ff 60%,#b65cff);box-shadow:0 20px 60px #438bff55,inset 0 1px #fff8;font-size:52px}.eyebrow{text-align:center;color:#68baff;text-transform:uppercase;font-size:12px;font-weight:800;letter-spacing:.12em}.title{text-align:center;margin:8px 0 6px;font-size:clamp(28px,8vw,38px);line-height:1.08}.subtitle{text-align:center;color:#9eabc0;margin:0 0 24px}.facts{display:grid;grid-template-columns:1fr 1fr;gap:10px;margin:20px 0}.fact{background:#ffffff08;border:1px solid #ffffff0d;border-radius:15px;padding:13px}.fact span{display:block;color:#8492aa;font-size:11px;text-transform:uppercase;letter-spacing:.08em;margin-bottom:4px}.fact strong{font-size:14px}.notice{color:#b9c5d8;font-size:13px;line-height:1.5;background:#07101f99;border:1px solid #4ba7ff26;border-radius:14px;padding:13px 14px;margin:16px 0}.actions{display:grid;gap:12px;margin-top:20px}#wallet{display:flex;justify-content:center;min-height:48px}button.primary{appearance:none;border:0;border-radius:14px;padding:15px 18px;background:linear-gradient(135deg,#35a8ff,#665cff);color:white;font:inherit;font-weight:800;cursor:pointer;box-shadow:0 10px 30px #308cff42;transition:transform .15s,opacity .15s}button.primary:active{transform:scale(.98)}button.primary:disabled{opacity:.45;cursor:not-allowed;box-shadow:none}.status{min-height:22px;text-align:center;color:#a8b6ca;font-size:13px;margin-top:14px;overflow-wrap:anywhere}.status.error{color:#ff8f99}.status.done{color:#65e5a2}.chain{display:flex;align-items:center;justify-content:center;gap:7px;color:#75839a;font-size:12px;margin-top:20px}.dot{width:7px;height:7px;border-radius:50%;background:#5fe59a;box-shadow:0 0 12px #5fe59a}.addresses{margin-top:18px;color:#95a3b9;font-size:12px;overflow-wrap:anywhere}.addresses p{padding:10px;background:#ffffff08;border-radius:10px}.expired{text-align:center;color:#ffad78}.footer{text-align:center;color:#5f6d83;font-size:11px;margin:18px 0}
@media(prefers-reduced-motion:no-preference){.orb{animation:float 4s ease-in-out infinite}@keyframes float{50%{transform:translateY(-6px)}}}
</style></head><body><main class="shell" id="app" data-request="{{.RequestID}}"><div class="brand"><i>◆</i><span>{{.AppName}} CustomFragment</span></div><section class="card"><div class="orb">🎁</div><div class="eyebrow">Collectible gift</div><h1 class="title">{{.Title}}</h1><p class="subtitle">{{.Slug}} · #{{.Number}}</p><div class="facts"><div class="fact"><span>Network</span><strong>TON Mainnet</strong></div><div class="fact"><span>Status</span><strong id="state">{{.Status}}</strong></div></div>
{{if .Pending}}<div class="notice">Connect the wallet that should receive this NFT. The mint authorization is bound to that address and expires at {{.ExpiresAt}}.</div><div class="actions"><div id="wallet"></div><button class="primary" id="mint" disabled>Mint gift on TON</button></div><div class="status" id="message">Connect a TON mainnet wallet to continue.</div>{{else if .Completed}}<div class="status done">This gift is on TON mainnet.</div><div class="addresses"><p>Owner: {{.OwnerAddress}}</p><p>NFT: {{.GiftAddress}}</p></div>{{else}}<p class="expired">This withdrawal link is no longer active.</p>{{end}}
<div class="chain"><span class="dot"></span><span>Verified by a TON lite server</span></div></section><div class="footer">A self-hosted Gramsrv mint flow · no official Fragment server</div></main>
{{if .Pending}}<script>
window.addEventListener('DOMContentLoaded',()=>{const root=document.getElementById('app'),button=document.getElementById('mint'),message=document.getElementById('message'),state=document.getElementById('state'),requestID=root.dataset.request;let account=null,busy=false;const say=(text,kind='')=>{message.textContent=text;message.className='status '+kind};if(!window.TON_CONNECT_UI){say('TON Connect failed to load. Reload the page.','error');return}const tc=new TON_CONNECT_UI.TonConnectUI({manifestUrl:location.origin+'/custom-fragment/tonconnect-manifest.json',buttonRootId:'wallet'});tc.onStatusChange(wallet=>{account=wallet&&wallet.account?wallet.account:null;button.disabled=!account||busy;if(account&&String(account.chain)!=='-239'){button.disabled=true;say('Switch the wallet to TON Mainnet.','error')}else if(account){say('Wallet connected. You can mint the gift.')}else{say('Connect a TON mainnet wallet to continue.')}});const post=async(endpoint)=>{const response=await fetch('/custom-fragment/api/gifts/'+encodeURIComponent(requestID)+'/'+endpoint,{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({wallet_address:account.address})});let data={};try{data=await response.json()}catch(_){}return{response,data}};const waitForMint=async()=>{for(let i=0;i<45;i++){const{response,data}=await post('confirm');if(response.ok){state.textContent='completed';say('Gift minted and verified on TON mainnet.','done');button.remove();document.getElementById('wallet').remove();return}if(response.status!==409)throw new Error(data.error||'Could not verify the NFT');say('Transaction sent. Waiting for mainnet finalization…');await new Promise(resolve=>setTimeout(resolve,3000))}throw new Error('Mainnet verification timed out. You can safely retry confirmation.')};button.addEventListener('click',async()=>{if(!account||busy)return;if(String(account.chain)!=='-239'){say('Switch the wallet to TON Mainnet.','error');return}busy=true;button.disabled=true;try{say('Creating a wallet-bound mint authorization…');const{response,data}=await post('intent');if(!response.ok)throw new Error(data.error||'Could not create mint authorization');await tc.sendTransaction({validUntil:data.valid_until,network:data.network,messages:[{address:data.collection_address,amount:data.amount,payload:data.payload}]});await waitForMint()}catch(error){say(error&&error.message?error.message:'Mint cancelled.','error');busy=false;button.disabled=!account}})});
</script>{{end}}</body></html>`))

func (s *Service) ServeWithdrawalPage(w http.ResponseWriter, r *http.Request, requestID string) {
	if s == nil || r.Method != http.MethodGet || !validRequestID(requestID) {
		http.NotFound(w, r)
		return
	}
	withdrawal, found, err := s.ledger.ResolveWithdrawal(r.Context(), requestID)
	if err != nil || !found {
		http.NotFound(w, r)
		return
	}
	page := withdrawalPage{
		AppName: s.appName, RequestID: requestID, Title: withdrawal.Gift.Title,
		Slug: withdrawal.Gift.Slug, Number: withdrawal.Gift.Num, Status: withdrawal.Status,
		ExpiresAt: time.Unix(int64(withdrawal.ExpiresAt), 0).UTC().Format(time.RFC3339),
		Pending:   withdrawal.Status == "pending" && withdrawal.ExpiresAt > int(time.Now().Unix()),
		Completed: withdrawal.Status == "completed", OwnerAddress: withdrawal.Gift.OwnerAddress,
		GiftAddress: withdrawal.Gift.GiftAddress,
	}
	if strings.TrimSpace(page.Title) == "" {
		page.Title = "Gramsrv Gift"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := withdrawalTemplate.Execute(w, page); err != nil {
		s.logger.Warn("render CustomFragment withdrawal", zap.Error(err))
	}
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s == nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case r.Method == http.MethodGet && (r.URL.Path == "/custom-fragment" || r.URL.Path == "/custom-fragment/"):
		s.serveLanding(w)
	case r.Method == http.MethodGet && r.URL.Path == "/custom-fragment/tonconnect-manifest.json":
		s.serveManifest(w)
	case r.Method == http.MethodGet && r.URL.Path == "/custom-fragment/collection.json":
		s.serveCollectionMetadata(w)
	case r.Method == http.MethodGet && r.URL.Path == "/custom-fragment/icon.svg":
		serveIcon(w)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/custom-fragment/metadata/gift/"):
		s.serveGiftMetadata(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/custom-fragment/media/gift/"):
		s.serveGiftImage(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/custom-fragment/api/gifts/"):
		s.serveGiftAPI(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Service) serveGiftAPI(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/custom-fragment/api/gifts/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || !validRequestID(parts[0]) || (parts[1] != "intent" && parts[1] != "confirm") {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var input struct {
		WalletAddress string `json:"wallet_address"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || strings.TrimSpace(input.WalletAddress) == "" {
		writeAPIError(w, ErrInvalidRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if parts[1] == "intent" {
		intent, err := s.Intent(r.Context(), parts[0], input.WalletAddress, time.Now())
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, intent)
		return
	}
	confirmation, err := s.Confirm(r.Context(), parts[0], input.WalletAddress, time.Now())
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, confirmation)
}

func (s *Service) serveManifest(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]string{
		"url": s.publicBaseURL + "/custom-fragment", "name": s.appName + " CustomFragment",
		"iconUrl": s.publicBaseURL + "/custom-fragment/icon.svg",
	})
}

func (s *Service) serveCollectionMetadata(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, http.StatusOK, map[string]any{
		"name":         s.collectionName,
		"description":  "Unique collectible gifts exported from InvGram/Gramsrv to TON mainnet.",
		"image":        s.publicBaseURL + "/custom-fragment/icon.svg",
		"external_url": s.publicBaseURL + "/custom-fragment",
	})
}

func (s *Service) serveLanding(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>CustomFragment</title><style>body{font:16px/1.5 system-ui;background:#0b1220;color:#eef5ff;max-width:680px;margin:12vh auto;padding:24px}code{overflow-wrap:anywhere;color:#76c4ff}</style><h1>%s</h1><p>Self-hosted TON mainnet withdrawal for unique Gramsrv gifts. Open the private withdrawal URL returned by <code>payments.getStarGiftWithdrawalUrl</code>.</p><p>Collection: <code>%s</code></p><p>Signing public key: <code>%s</code></p>`, template.HTMLEscapeString(s.collectionName), s.collection.StringRaw(), s.PublicKeyHex())
}

func (s *Service) serveGiftMetadata(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/custom-fragment/metadata/gift/"), ".json")
	gift, ok := s.resolvePublicGift(w, r, slug)
	if !ok {
		return
	}
	title := strings.TrimSpace(gift.Title)
	if title == "" {
		title = "Gramsrv Gift"
	}
	attributes := []map[string]string{
		{"trait_type": "Model", "value": gift.Model.Name},
		{"trait_type": "Pattern", "value": gift.Pattern.Name},
		{"trait_type": "Backdrop", "value": gift.Backdrop.Name},
		{"trait_type": "Number", "value": strconv.Itoa(gift.Num)},
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":         fmt.Sprintf("%s #%d", title, gift.Num),
		"description":  "A unique collectible gift exported from " + s.appName + " to TON mainnet.",
		"image":        s.publicBaseURL + path.Join("/custom-fragment/media/gift/", gift.Slug+".svg"),
		"external_url": s.publicBaseURL + path.Join("/nft/", gift.Slug),
		"attributes":   attributes,
	})
}

func (s *Service) serveGiftImage(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/custom-fragment/media/gift/"), ".svg")
	gift, ok := s.resolvePublicGift(w, r, slug)
	if !ok {
		return
	}
	title := template.HTMLEscapeString(gift.Title)
	if title == "" {
		title = "Gramsrv Gift"
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" width="1024" height="1024" viewBox="0 0 1024 1024"><defs><radialGradient id="b"><stop stop-color="#51b8ff"/><stop offset=".52" stop-color="#635cff"/><stop offset="1" stop-color="#10192d"/></radialGradient></defs><rect width="1024" height="1024" rx="96" fill="url(#b)"/><circle cx="512" cy="430" r="230" fill="#fff" opacity=".13"/><text x="512" y="510" text-anchor="middle" font-size="230">🎁</text><text x="512" y="770" text-anchor="middle" fill="white" font-family="system-ui,sans-serif" font-size="64" font-weight="700">%s</text><text x="512" y="850" text-anchor="middle" fill="#d9e7ff" font-family="system-ui,sans-serif" font-size="46">#%d · GRAMSRV</text></svg>`, title, gift.Num)
}

func (s *Service) resolvePublicGift(w http.ResponseWriter, r *http.Request, slug string) (domain.UniqueStarGift, bool) {
	if !validGiftSlug(slug) {
		http.NotFound(w, r)
		return domain.UniqueStarGift{}, false
	}
	gift, found, err := s.ledger.UniqueBySlug(r.Context(), slug)
	if err != nil || !found || gift.Burned {
		http.NotFound(w, r)
		return domain.UniqueStarGift{}, false
	}
	return gift, true
}

func validGiftSlug(slug string) bool {
	if slug == "" || len(slug) > domain.MaxStarGiftSlugBytes {
		return false
	}
	for _, char := range slug {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '.' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func serveIcon(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="512" height="512" viewBox="0 0 512 512"><defs><linearGradient id="g" x2="1" y2="1"><stop stop-color="#35adff"/><stop offset="1" stop-color="#725cff"/></linearGradient></defs><rect width="512" height="512" rx="118" fill="url(#g)"/><path fill="white" d="M129 196h254v205H129zM107 144h298v76H107z"/><path fill="none" stroke="#5c63f0" stroke-width="34" d="M256 144v257M178 144c-49-55 38-99 78 0m78 0c49-55-38-99-78 0"/></svg>`))
}

func writeAPIError(w http.ResponseWriter, err error) {
	status, message := http.StatusInternalServerError, "CustomFragment is temporarily unavailable"
	switch {
	case errors.Is(err, ErrInvalidRequest):
		status, message = http.StatusBadRequest, "Invalid withdrawal or wallet address"
	case errors.Is(err, ErrAlreadyExported):
		status, message = http.StatusConflict, "This gift is already exported"
	case errors.Is(err, ErrNotFinalized):
		status, message = http.StatusConflict, "NFT not finalized yet"
	case errors.Is(err, ErrUnavailable):
		status, message = http.StatusGone, "This withdrawal is no longer available"
	}
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
