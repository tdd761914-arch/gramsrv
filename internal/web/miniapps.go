package web

import (
	"encoding/json"
	"html/template"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"telesrv/internal/domain"
)

// MiniApps serves the local Telegram-style BotFather and Stickers surfaces.
// It intentionally contains no database code: write operations are delegated
// to the configured application services in miniapps_api.go.
type MiniApps struct {
	appName        string
	bots           MiniAppBotManager
	stickers       MiniAppStickerManager
	tokens         MiniAppBotTokenStore
	botFatherToken string
	stickersToken  string
}

// NewMiniAppsHandler preserves the small constructor used by embedders that
// only need the static pages. Configured deployments should use
// NewConfiguredMiniAppsHandler so writes are connected to Gramsrv services.
func NewMiniAppsHandler(appName string) http.Handler {
	return NewConfiguredMiniAppsHandler(MiniAppsConfig{AppName: appName})
}

func NewConfiguredMiniAppsHandler(cfg MiniAppsConfig) http.Handler {
	appName := strings.TrimSpace(cfg.AppName)
	if appName == "" {
		appName = "Gramsrv"
	}
	return &MiniApps{
		appName: appName, bots: cfg.Bots, stickers: cfg.Stickers, tokens: cfg.Tokens,
		botFatherToken: strings.TrimSpace(cfg.BotFatherToken), stickersToken: strings.TrimSpace(cfg.StickersToken),
	}
}

func (m *MiniApps) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if m == nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case r.Method == http.MethodGet && (r.URL.Path == "/botfather" || r.URL.Path == "/botfather/"):
		m.servePage(w, botFatherTemplate, botFatherPage{AppName: m.appName})
	case r.Method == http.MethodGet && (r.URL.Path == "/stickers" || r.URL.Path == "/stickers/"):
		m.servePage(w, stickersTemplate, stickersPage{AppName: m.appName})
	case r.Method == http.MethodGet && r.URL.Path == "/api/miniapps/botfather/status":
		_, tokenErr := m.botToken(r.Context(), domain.BotFatherUserID)
		configured := m.bots != nil && tokenErr == nil
		message := "Bot management is configured on Gramsrv."
		if !configured {
			message = "Bot management needs a server-side Mini App bot token."
		}
		writeMiniJSON(w, http.StatusOK, map[string]any{
			"provider": "gramsrv", "mode": "database", "telegram": false,
			"configured": configured, "message": message,
		})
	case r.Method == http.MethodPost && r.URL.Path == "/api/miniapps/botfather/validate":
		m.validateBotToken(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/miniapps/botfather/bots":
		m.listBots(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/miniapps/botfather/bots":
		m.createBot(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/miniapps/stickers":
		m.listStickerCatalog(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/miniapps/stickers/mine":
		m.listMyStickerSets(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/miniapps/stickers":
		m.createStickerSet(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/miniapps/stickers/"):
		m.stickerSet(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (m *MiniApps) servePage(w http.ResponseWriter, page *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := page.Execute(w, data); err != nil {
		return
	}
}

func (m *MiniApps) validateBotToken(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<10)
	var input struct {
		Token string `json:"token"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeMiniJSON(w, http.StatusBadRequest, map[string]any{"valid": false, "error": "invalid request"})
		return
	}
	token := strings.TrimSpace(input.Token)
	match := botTokenPattern.FindStringSubmatch(token)
	if match == nil {
		writeMiniJSON(w, http.StatusOK, map[string]any{"valid": false, "message": "Token format is not valid."})
		return
	}
	botID, _ := strconv.ParseInt(match[1], 10, 64)
	writeMiniJSON(w, http.StatusOK, map[string]any{
		"valid": true, "bot_id": botID,
		"message": "Format is valid. No request was sent to Telegram and the token was not stored.",
	})
}

func writeMiniJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeMiniAPIError(w http.ResponseWriter, status int, message string) {
	writeMiniJSON(w, status, map[string]any{"error": message})
}

var botTokenPattern = regexp.MustCompile(`^([0-9]{5,12}):[A-Za-z0-9_-]{20,}$`)

type botFatherPage struct{ AppName string }
type stickersPage struct{ AppName string }

var botFatherTemplate = template.Must(template.New("botfather-miniapp").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover"><meta name="robots" content="noindex,nofollow"><title>BotFather · {{.AppName}}</title>
<style>
:root{font:16px/1.45 Arial,system-ui,sans-serif;color:#17212b;background:#f4f6f8;--accent:#2481cc;--muted:#8794a1;--field:#fff;--line:#d8e0e8}*{box-sizing:border-box}body{margin:0;min-height:100vh;background:linear-gradient(#e9f3fc,#f4f6f8 45%)}.tm-header{height:56px;background:#fff;border-bottom:1px solid var(--line);display:flex;align-items:center;padding:0 max(16px,calc((100% - 720px)/2));gap:12px;position:sticky;top:0;z-index:1}.logo{width:34px;height:34px;border-radius:10px;background:var(--accent);color:#fff;display:grid;place-items:center;font-weight:800}.header-title{font-weight:700}.header-sub{margin-left:auto;color:var(--muted);font-size:13px}.tm-main{max-width:720px;margin:0 auto;padding:22px 16px 42px}.tm-section{background:var(--field);border:1px solid var(--line);border-radius:12px;box-shadow:0 2px 10px #17212b0b;padding:22px;margin-bottom:14px}.eyebrow{color:var(--accent);font-size:12px;font-weight:700;text-transform:uppercase;letter-spacing:.08em}h1{font-size:28px;margin:7px 0}.lead,.hint{color:var(--muted)}label{display:block;font-size:13px;font-weight:700;margin:16px 0 7px}.field{width:100%;border:1px solid #c9d3dd;border-radius:8px;background:#fff;padding:12px;font:inherit;color:inherit}.field:focus{border-color:var(--accent);outline:2px solid #2481cc33}.btn{border:0;border-radius:8px;background:var(--accent);color:#fff;padding:12px 18px;font:inherit;font-weight:700;cursor:pointer}.btn:disabled{opacity:.5;cursor:default}.row{display:flex;gap:10px;align-items:center;margin-top:14px;flex-wrap:wrap}.status{min-height:22px;margin-top:12px;color:var(--muted)}.status.ok{color:#168552}.status.bad{color:#c83d42}..notice{margin-top:20px;padding:12px 14px;border-radius:8px;background:#f0f7fd;border-left:3px solid var(--accent);color:#52606d;font-size:13px}.token{display:none;margin-top:16px;padding:12px;border-radius:8px;background:#fff7df;border:1px solid #e9cf84;word-break:break-all;font-family:monospace;font-size:13px}.token.show{display:block}.bot{display:flex;justify-content:space-between;gap:10px;padding:12px 0;border-top:1px solid var(--line)}.bot:first-child{border-top:0}.bot-name{font-weight:700}.bot-meta{color:var(--muted);font-size:13px}@media(prefers-color-scheme:dark){:root{color:#e8edf2;background:#17212b;--field:#212a33;--line:#35424f;--muted:#aab7c4}body{background:linear-gradient(#1b2d3d,#17212b 50%)}.tm-header,.field{background:#212a33}.notice{background:#263744;color:#b8c6d2}.token{background:#4a3d20;border-color:#846f32}}
</style></head><body><header class="tm-header"><div class="logo">F</div><div class="header-title">BotFather</div><div class="header-sub">{{.AppName}} · Gramsrv</div></header><main class="tm-main"><section class="tm-section"><div class="eyebrow">Gramsrv mini app</div><h1>Create a bot</h1><p class="lead">The account and token are created by Gramsrv. Your Telegram identity is taken from signed Mini App data; it is never accepted from the form.</p><form id="create"><label for="name">Bot name</label><input class="field" id="name" name="name" maxlength="64" required autocomplete="off"><label for="username">Username</label><input class="field" id="username" name="username" maxlength="32" required autocomplete="off" placeholder="my_new_bot"><div class="row"><button class="btn" id="createButton" type="submit">Create bot</button><span class="hint" id="status" role="status"></span></div></form><div class="token" id="tokenBox"><strong>Copy this token now:</strong><br><span id="token"></span></div><div class="notice" id="mode">Checking Gramsrv…</div></section><section class="tm-section"><h2>Your bots</h2><div id="bots"><span class="hint">Open this app from a signed Telegram Mini App to load your bots.</span></div></section></main><script>
(()=>{const q=s=>document.querySelector(s),form=q('#create'),button=q('#createButton'),status=q('#status'),mode=q('#mode'),bots=q('#bots'),tokenBox=q('#tokenBox'),token=q('#token');const tg=window.Telegram&&window.Telegram.WebApp;const initData=(tg&&tg.initData)||new URLSearchParams(location.search).get('tgWebAppData')||'';if(tg&&tg.ready)tg.ready();const say=(x,bad)=>{status.textContent=x;status.className='hint status '+(bad?'bad':'ok')};const api=async(path,opts={})=>{opts.headers=Object.assign({'x-telegram-init-data':initData},opts.headers||{});const r=await fetch(path,opts);let x={};try{x=await r.json()}catch(_){}if(!r.ok)throw new Error(x.error||'Request failed');return x};fetch('/api/miniapps/botfather/status').then(r=>r.json()).then(x=>{mode.textContent=x.message}).catch(()=>{mode.textContent='Gramsrv status is unavailable.'});const render=x=>{bots.textContent='';if(!x.bots||!x.bots.length){bots.innerHTML='<span class="hint">No bots yet.</span>';return}x.bots.forEach(b=>{const row=document.createElement('div');row.className='bot';const n=document.createElement('div');n.className='bot-name';n.textContent=b.name||'Unnamed';const meta=document.createElement('div');meta.className='bot-meta';meta.textContent='@'+(b.username||'')+' · '+b.id;row.append(n,meta);bots.append(row)})};const load=()=>{if(!initData)return;api('/api/miniapps/botfather/bots').then(render).catch(e=>{bots.textContent=e.message})};form.onsubmit=async e=>{e.preventDefault();if(!initData){say('Open this page inside a signed Telegram Mini App.',true);return}button.disabled=true;tokenBox.classList.remove('show');say('Creating…');try{const x=await api('/api/miniapps/botfather/bots',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({name:q('#name').value,username:q('#username').value})});token.textContent=x.token;tokenBox.classList.add('show');say('Bot created. Token is shown once.');form.reset();load()}catch(err){say(err.message,true)}finally{button.disabled=false}};load()})();</script></body></html>`))

var stickersTemplate = template.Must(template.New("stickers-miniapp").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover"><meta name="robots" content="noindex,nofollow"><title>Stickers · {{.AppName}}</title>
<style>
:root{font:16px/1.45 Arial,system-ui,sans-serif;color:#17212b;background:#f4f6f8;--accent:#2481cc;--muted:#8794a1;--line:#d8e0e8;--field:#fff}*{box-sizing:border-box}body{margin:0;min-height:100vh;background:linear-gradient(#e9f3fc,#f4f6f8 45%)}.tm-header{height:56px;background:#fff;border-bottom:1px solid var(--line);display:flex;align-items:center;padding:0 max(16px,calc((100% - 760px)/2));gap:12px;position:sticky;top:0;z-index:1}.logo{width:34px;height:34px;border-radius:10px;background:var(--accent);color:#fff;display:grid;place-items:center;font-weight:800}.header-title{font-weight:700}.header-sub{margin-left:auto;color:var(--muted);font-size:13px}.tm-main{max-width:760px;margin:0 auto;padding:22px 16px 42px}.tm-section{background:var(--field);border:1px solid var(--line);border-radius:12px;box-shadow:0 2px 10px #17212b0b;padding:22px;margin-bottom:14px}.eyebrow{color:var(--accent);font-size:12px;font-weight:700;text-transform:uppercase;letter-spacing:.08em}h1{font-size:28px;margin:7px 0}h2{font-size:20px;margin:0 0 12px}.lead,.hint{color:var(--muted)}label{display:block;font-size:13px;font-weight:700;margin:14px 0 7px}.field{width:100%;border:1px solid #c9d3dd;border-radius:8px;background:#fff;padding:11px;font:inherit;color:inherit}.field:focus{border-color:var(--accent);outline:2px solid #2481cc33}.row{display:flex;gap:10px;align-items:center;margin-top:14px;flex-wrap:wrap}.btn{border:0;border-radius:8px;background:var(--accent);color:#fff;padding:12px 18px;font:inherit;font-weight:700;cursor:pointer}.btn:disabled{opacity:.5}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(190px,1fr));gap:10px;margin-top:20px}.set{border:1px solid var(--line);border-radius:10px;padding:14px;background:var(--field)}.emoji{width:52px;height:52px;border-radius:12px;background:#e8f3fc;display:grid;place-items:center;font-size:30px;margin-bottom:10px}.name{font-weight:700}.meta{color:var(--muted);font-size:13px;margin-top:3px}.status{min-height:22px;color:var(--muted)}.status.bad{color:#c83d42}.notice{margin-top:16px;padding:12px 14px;border-radius:8px;background:#f0f7fd;border-left:3px solid var(--accent);color:#52606d;font-size:13px}textarea{min-height:112px;resize:vertical}@media(prefers-color-scheme:dark){:root{color:#e8edf2;background:#17212b;--field:#212a33;--line:#35424f;--muted:#aab7c4}body{background:linear-gradient(#1b2d3d,#17212b 50%)}.tm-header,.field,.set{background:#212a33}.emoji{background:#2b4252}.notice{background:#263744;color:#b8c6d2}}
</style></head><body><header class="tm-header"><div class="logo">F</div><div class="header-title">Stickers</div><div class="header-sub">{{.AppName}} · Gramsrv</div></header><main class="tm-main"><section class="tm-section"><div class="eyebrow">Gramsrv mini app</div><h1>Create a sticker set</h1><p class="lead">Use document IDs and access hashes of stickers already uploaded to your Gramsrv account. The server checks ownership and sticker material in one DB-backed operation.</p><form id="create"><label for="title">Title</label><input class="field" id="title" maxlength="64" required autocomplete="off"><label for="short">Short name (optional)</label><input class="field" id="short" maxlength="32" autocomplete="off" placeholder="my_sticker_pack"><label for="kind">Type</label><select class="field" id="kind"><option value="stickers">Stickers</option><option value="emoji">Custom emoji</option><option value="masks">Masks</option></select><label for="items">Sticker documents</label><textarea class="field" id="items" required placeholder="document_id:access_hash:emoji:keywords\n123:456:🙂:happy,cats"></textarea><div class="row"><button class="btn" id="createButton" type="submit">Create set</button><span class="status" id="status" role="status"></span></div></form><div class="notice">Access hashes are checked server-side. A guessed ID, another user's document, duplicate item, or stale hash is rejected.</div></section><section class="tm-section"><h2>Your sets</h2><div class="grid" id="mine"><div class="hint">Open this app from a signed Telegram Mini App to load your sets.</div></div></section><section class="tm-section"><h2>Public catalog</h2><div class="grid" id="sets"><div class="hint">Loading…</div></div></section></main><script>
(()=>{const q=s=>document.querySelector(s),status=q('#status'),button=q('#createButton'),mine=q('#mine'),root=q('#sets');const tg=window.Telegram&&window.Telegram.WebApp;const initData=(tg&&tg.initData)||new URLSearchParams(location.search).get('tgWebAppData')||'';if(tg&&tg.ready)tg.ready();const api=async(path,opts={})=>{opts.headers=Object.assign({'x-telegram-init-data':initData},opts.headers||{});const r=await fetch(path,opts);let x={};try{x=await r.json()}catch(_){}if(!r.ok)throw new Error(x.error||'Request failed');return x};const view=(items,el)=>{el.textContent='';if(!items||!items.length){el.innerHTML='<div class="hint">No sets yet.</div>';return}items.forEach(item=>{const card=document.createElement('div');card.className='set';const icon=document.createElement('div');icon.className='emoji';icon.textContent=item.kind==='emoji'?'😀':item.kind==='masks'?'🎭':'🙂';const name=document.createElement('div');name.className='name';name.textContent=item.title||item.short_name;const meta=document.createElement('div');meta.className='meta';meta.textContent=(item.count||0)+' stickers · '+item.kind;card.append(icon,name,meta);el.append(card)})};const loadCatalog=()=>api('/api/miniapps/stickers').then(x=>view(x,root)).catch(()=>{root.innerHTML='<div class="hint">Public catalog is unavailable.</div>'});const loadMine=()=>{if(initData)api('/api/miniapps/stickers/mine').then(x=>view(x.sets,mine)).catch(e=>{mine.textContent=e.message})};q('#create').onsubmit=async e=>{e.preventDefault();if(!initData){status.textContent='Open this page inside a signed Telegram Mini App.';status.className='status bad';return}const items=[];for(const raw of q('#items').value.split(/\r?\n/)){const line=raw.trim();if(!line)continue;const p=line.split(':');if(p.length<3||!/^\d+$/.test(p[0])||!/^\d+$/.test(p[1])){status.textContent='Use document_id:access_hash:emoji:keywords';status.className='status bad';return}items.push({document_id:Number(p[0]),document_access_hash:Number(p[1]),emoji:p[2],keywords:p.slice(3).join(':')})}button.disabled=true;status.textContent='Creating…';status.className='status';try{await api('/api/miniapps/stickers',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({title:q('#title').value,short_name:q('#short').value,kind:q('#kind').value,items})});status.textContent='Sticker set created.';q('#create').reset();loadMine();loadCatalog()}catch(err){status.textContent=err.message;status.className='status bad'}finally{button.disabled=false}};loadCatalog();loadMine()})();</script></body></html>`))
