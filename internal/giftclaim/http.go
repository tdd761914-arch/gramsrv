package giftclaim

import (
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"strings"
	"time"
)

var claimTemplate = template.Must(template.New("gift-claim").Parse(`<!doctype html>
<html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<meta name="color-scheme" content="dark"><title>Claim · {{.}}</title>
<script src="https://telegram.org/js/telegram-web-app.js"></script>
<script defer src="https://unpkg.com/@tonconnect/ui@2.3.1/dist/tonconnect-ui.min.js"></script>
<style>:root{font-family:Inter,system-ui,-apple-system,sans-serif;color:#f7f9ff;background:#080d18}*{box-sizing:border-box}body{margin:0;min-height:100vh;padding:calc(22px + env(safe-area-inset-top)) 16px;background:radial-gradient(circle at 50% -10%,#18568f,#0b1426 42%,#080d18 75%)}main{max-width:520px;margin:auto}.brand{font-weight:800;color:#77bdff;margin:4px 4px 18px}.card{background:#111b2de8;border:1px solid #ffffff18;border-radius:26px;padding:24px;box-shadow:0 28px 80px #0008}.gift{text-align:center;font-size:54px}.eyebrow{text-align:center;color:#67b8ff;font-size:12px;font-weight:800;letter-spacing:.11em;text-transform:uppercase}h1{text-align:center;margin:7px 0;font-size:30px}p.lead{text-align:center;color:#9facbf;line-height:1.5;margin:0 0 20px}label{display:block;color:#aebbd0;font-size:12px;font-weight:700;margin:14px 0 7px}input{width:100%;border:1px solid #ffffff1c;background:#07101f;color:white;padding:14px;border-radius:14px;font:inherit;outline:none}input:focus{border-color:#49aaff}button{width:100%;border:0;border-radius:14px;padding:15px;margin-top:14px;background:linear-gradient(135deg,#38aaff,#685cff);color:#fff;font:inherit;font-weight:800;cursor:pointer}button:disabled{opacity:.45}.wallet{display:flex;justify-content:center;margin:13px 0}.status{min-height:22px;text-align:center;margin-top:14px;color:#a6b4c8;font-size:13px;overflow-wrap:anywhere}.status.error{color:#ff929c}.status.done{color:#64e3a0}.facts{display:none;gap:9px;margin-top:17px}.facts.show{display:grid}.fact{background:#ffffff08;border-radius:13px;padding:12px;overflow-wrap:anywhere}.fact span{display:block;color:#8190a8;font-size:10px;text-transform:uppercase;letter-spacing:.08em;margin-bottom:5px}.fact strong{font-size:12px}.note{color:#74839a;font-size:11px;text-align:center;margin-top:18px}</style></head>
<body><main><div class="brand">◆ {{.}} · @claim</div><section class="card"><div class="gift">🎁</div><div class="eyebrow">TON Proof</div><h1>Закрепить NFT</h1><p class="lead">Подтвердите текущий TON-кошелёк и прикрепите подарок к своему профилю Gramsrv.</p><label for="gift">Slug или адрес NFT</label><input id="gift" autocomplete="off" placeholder="owl-1 или EQ…"><div class="wallet" id="wallet"></div><button id="claim">Подтвердить владение</button><div class="status" id="status">Откройте приложение через профиль @claim.</div><div class="facts" id="facts"><div class="fact"><span>Владелец</span><strong id="owner">—</strong></div><div class="fact"><span>Адрес кошелька</span><strong id="walletAddress">—</strong></div><div class="fact"><span>Адрес NFT</span><strong id="nftAddress">—</strong></div></div><div class="note">Mainnet · одноразовый TON Proof · владение NFT проверяется через lite server</div></section></main>
<script>window.addEventListener('DOMContentLoaded',()=>{const tg=window.Telegram&&Telegram.WebApp;tg&&tg.ready();const input=document.getElementById('gift'),button=document.getElementById('claim'),status=document.getElementById('status'),facts=document.getElementById('facts'),owner=document.getElementById('owner'),walletAddress=document.getElementById('walletAddress'),nftAddress=document.getElementById('nftAddress');const params=new URLSearchParams(location.search);input.value=params.get('gift')||'';const initData=(tg&&tg.initData)||params.get('tgWebAppData')||'';let challenge=null,busy=false,tc=null;const say=(text,kind='')=>{status.textContent=text;status.className='status '+kind};const show=(data)=>{facts.classList.add('show');owner.textContent=data.owner_profile||'—';walletAddress.textContent=data.wallet_address||'—';nftAddress.textContent=data.nft_address||'—'};const api=async(path,body)=>{const response=await fetch('/claim/api/'+path,{method:'POST',headers:{'content-type':'application/json','x-telegram-init-data':initData},body:JSON.stringify(body)});let data={};try{data=await response.json()}catch(_){}if(!response.ok)throw new Error(data.error||'Ошибка запроса');return data};if(!initData){button.disabled=true;say('Откройте Mini App из профиля @claim.','error');return}if(!window.TON_CONNECT_UI){button.disabled=true;say('TON Connect не загрузился. Обновите страницу.','error');return}tc=new TON_CONNECT_UI.TonConnectUI({manifestUrl:location.origin+'/claim/tonconnect-manifest.json',buttonRootId:'wallet'});tc.onStatusChange(async wallet=>{if(!busy||!challenge||!wallet)return;const item=wallet.connectItems&&wallet.connectItems.tonProof;if(!item||!item.proof){say('Кошелёк не вернул TON Proof. Повторите подключение.','error');busy=false;button.disabled=false;return}try{say('Проверяю владельца NFT в TON mainnet…');const result=await api('verify',{payload:challenge.payload,account:wallet.account,proof:item.proof});show(result);say('Подарок закреплён в вашем профиле.','done')}catch(error){say(error.message||'Claim не выполнен.','error')}finally{busy=false;challenge=null;button.disabled=false}});button.addEventListener('click',async()=>{if(busy)return;const gift=input.value.trim();if(!gift){say('Введите slug или адрес NFT.','error');return}busy=true;button.disabled=true;try{say('Создаю одноразовый TON Proof…');challenge=await api('challenge',{gift});show(challenge);if(tc.connected)await tc.disconnect();tc.setConnectRequestParameters({state:'ready',value:{tonProof:challenge.payload}});say('Подключите кошелёк, на котором находится NFT.');await tc.openModal()}catch(error){say(error.message||'Не удалось начать claim.','error');busy=false;challenge=null;button.disabled=false}})});</script></body></html>`))

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s == nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case r.Method == http.MethodGet && (r.URL.Path == "/claim" || r.URL.Path == "/claim/"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = claimTemplate.Execute(w, s.appName)
	case r.Method == http.MethodGet && r.URL.Path == "/claim/tonconnect-manifest.json":
		writeJSON(w, http.StatusOK, map[string]string{"url": s.publicBaseURL + "/claim", "name": s.appName + " Claim", "iconUrl": s.publicBaseURL + "/claim/icon.svg"})
	case r.Method == http.MethodGet && r.URL.Path == "/claim/icon.svg":
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512"><defs><linearGradient id="g"><stop stop-color="#35aaff"/><stop offset="1" stop-color="#735cff"/></linearGradient></defs><rect width="512" height="512" rx="120" fill="url(#g)"/><path d="M134 218h244v190H134zM112 166h288v76H112zM244 166h24v242h-24z" fill="white"/><path d="M256 166c-85-2-93-100-39-103 40-2 39 56 39 103zm0 0c85-2 93-100 39-103-40-2-39 56-39 103z" fill="none" stroke="white" stroke-width="22"/></svg>`))
	case r.Method == http.MethodPost && r.URL.Path == "/claim/api/challenge":
		s.serveChallenge(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/claim/api/verify":
		s.serveVerify(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Service) serveChallenge(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Gift string `json:"gift"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := s.Challenge(r.Context(), r.Header.Get("X-Telegram-Init-Data"), input.Gift, time.Now())
	if err != nil {
		writeClaimError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Service) serveVerify(w http.ResponseWriter, r *http.Request) {
	var input ClaimInput
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := s.Claim(r.Context(), r.Header.Get("X-Telegram-Init-Data"), input, time.Now())
	if err != nil {
		writeClaimError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeClaimError(w, ErrInvalid)
		return false
	}
	return true
}

func writeClaimError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, ErrExpired):
		status = http.StatusConflict
	case errors.Is(err, ErrNotOwner):
		status = http.StatusForbidden
	}
	writeJSON(w, status, map[string]string{"error": strings.TrimSpace(err.Error())})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
