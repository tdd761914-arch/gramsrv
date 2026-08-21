package web

import (
	"encoding/json"
	"html/template"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// MiniApps is the small, self-hosted surface used by the BotFather and
// Stickers pages.  It deliberately has no Telegram network client: browser
// requests stay on gramsrv and any future write operation can be wired to an
// authenticated server-side service without exposing Bot API credentials.
type MiniApps struct {
	appName string
}

func NewMiniAppsHandler(appName string) http.Handler {
	appName = strings.TrimSpace(appName)
	if appName == "" {
		appName = "Gramsrv"
	}
	return &MiniApps{appName: appName}
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
		writeMiniJSON(w, http.StatusOK, map[string]any{
			"provider": "gramsrv", "mode": "local", "telegram": false,
			"message": "BotFather actions are local stubs until an authenticated server integration is configured.",
		})
	case r.Method == http.MethodPost && r.URL.Path == "/api/miniapps/botfather/validate":
		m.validateBotToken(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/miniapps/stickers":
		writeMiniJSON(w, http.StatusOK, demoStickerSets)
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
		// The response may already have started; there is no safe second body to
		// write. Keep the failure generic and avoid leaking template internals.
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

func (m *MiniApps) stickerSet(w http.ResponseWriter, r *http.Request) {
	shortName := strings.TrimPrefix(r.URL.Path, "/api/miniapps/stickers/")
	for _, set := range demoStickerSets {
		if set.ShortName == shortName {
			writeMiniJSON(w, http.StatusOK, set)
			return
		}
	}
	http.NotFound(w, r)
}

func writeMiniJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

var botTokenPattern = regexp.MustCompile(`^([0-9]{5,12}):[A-Za-z0-9_-]{20,}$`)

type botFatherPage struct{ AppName string }

type stickersPage struct{ AppName string }

type miniStickerSet struct {
	ShortName string `json:"short_name"`
	Title     string `json:"title"`
	Count     int    `json:"count"`
	Emoji     string `json:"emoji"`
	Updated   string `json:"updated"`
}

var demoStickerSets = []miniStickerSet{
	{ShortName: "flashgram", Title: "Flashgram", Count: 24, Emoji: "⚡", Updated: "local demo"},
	{ShortName: "ton-gifts", Title: "TON Gifts", Count: 18, Emoji: "🎁", Updated: "local demo"},
	{ShortName: "classic", Title: "Classic", Count: 36, Emoji: "🙂", Updated: "local demo"},
}

var botFatherTemplate = template.Must(template.New("botfather-miniapp").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover"><meta name="robots" content="noindex,nofollow"><title>BotFather · {{.AppName}}</title>
<style>
:root{font:16px/1.45 Arial,system-ui,sans-serif;color:#17212b;background:#f4f6f8;--accent:#2481cc;--muted:#8794a1;--field:#fff;--line:#d8e0e8}*{box-sizing:border-box}body{margin:0;min-height:100vh;background:linear-gradient(#e9f3fc,#f4f6f8 45%);color:var(--text,#17212b)}.tm-header{height:56px;background:#fff;border-bottom:1px solid var(--line);display:flex;align-items:center;padding:0 max(16px,calc((100% - 720px)/2));gap:12px;position:sticky;top:0;z-index:1}.logo{width:34px;height:34px;border-radius:10px;background:var(--accent);color:#fff;display:grid;place-items:center;font-weight:800}.header-title{font-weight:700}.header-sub{margin-left:auto;color:var(--muted);font-size:13px}.tm-main{max-width:720px;margin:0 auto;padding:22px 16px 42px}.tm-section{background:var(--field);border:1px solid var(--line);border-radius:12px;box-shadow:0 2px 10px #17212b0b;padding:22px}.eyebrow{color:var(--accent);font-size:12px;font-weight:700;text-transform:uppercase;letter-spacing:.08em}h1{font-size:28px;margin:7px 0}.lead,.hint{color:var(--muted)}label{display:block;font-size:13px;font-weight:700;margin:22px 0 7px}.field{width:100%;border:1px solid #c9d3dd;border-radius:8px;background:#fff;padding:12px;font:inherit;color:inherit}.field:focus{border-color:var(--accent);outline:2px solid #2481cc33}.btn{border:0;border-radius:8px;background:var(--accent);color:#fff;padding:12px 18px;font:inherit;font-weight:700;cursor:pointer}.btn:disabled{opacity:.5;cursor:default}.row{display:flex;gap:10px;align-items:center;margin-top:14px}.status{min-height:22px;margin-top:16px;color:var(--muted)}.status.ok{color:#168552}.status.bad{color:#c83d42}.notice{margin-top:20px;padding:12px 14px;border-radius:8px;background:#f0f7fd;border-left:3px solid var(--accent);color:#52606d;font-size:13px}@media(prefers-color-scheme:dark){:root{color:#e8edf2;background:#17212b;--field:#212a33;--line:#35424f;--muted:#aab7c4}body{background:linear-gradient(#1b2d3d,#17212b 50%)}.tm-header,.field{background:#212a33}.notice{background:#263744;color:#b8c6d2}}
</style></head><body><header class="tm-header"><div class="logo">F</div><div class="header-title">BotFather</div><div class="header-sub">{{.AppName}} · local</div></header><main class="tm-main"><section class="tm-section"><div class="eyebrow">Gramsrv mini app</div><h1>BotFather</h1><p class="lead">Telegram-like bot management surface hosted by Gramsrv. This demo validates token shape locally and never sends it to Telegram.</p><label for="token">Bot token</label><input class="field" id="token" type="password" autocomplete="off" spellcheck="false" placeholder="123456789:AA…"><div class="row"><button class="btn" id="validate" type="button">Validate locally</button><span class="hint" id="status" role="status"></span></div><div class="notice" id="mode">Checking local provider…</div></section></main><script>
(()=>{const token=document.getElementById('token'),button=document.getElementById('validate'),status=document.getElementById('status'),mode=document.getElementById('mode');const say=(x,bad)=>{status.textContent=x;status.className='hint '+(bad?'status bad':'status ok')};fetch('/api/miniapps/botfather/status').then(r=>r.json()).then(x=>{mode.textContent=x.message||'Local Gramsrv provider.'}).catch(()=>{mode.textContent='Local Gramsrv provider.'});button.onclick=async()=>{button.disabled=true;say('Checking…');try{const r=await fetch('/api/miniapps/botfather/validate',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({token:token.value})}),x=await r.json();say(x.message||'Invalid token.',!x.valid)}catch(_){say('Local validation failed.',true)}finally{token.value='';button.disabled=false}}})();</script></body></html>`))

var stickersTemplate = template.Must(template.New("stickers-miniapp").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover"><meta name="robots" content="noindex,nofollow"><title>Stickers · {{.AppName}}</title>
<style>
:root{font:16px/1.45 Arial,system-ui,sans-serif;color:#17212b;background:#f4f6f8;--accent:#2481cc;--muted:#8794a1;--line:#d8e0e8;--field:#fff}*{box-sizing:border-box}body{margin:0;min-height:100vh;background:linear-gradient(#e9f3fc,#f4f6f8 45%)}.tm-header{height:56px;background:#fff;border-bottom:1px solid var(--line);display:flex;align-items:center;padding:0 max(16px,calc((100% - 720px)/2));gap:12px;position:sticky;top:0;z-index:1}.logo{width:34px;height:34px;border-radius:10px;background:var(--accent);color:#fff;display:grid;place-items:center;font-weight:800}.header-title{font-weight:700}.header-sub{margin-left:auto;color:var(--muted);font-size:13px}.tm-main{max-width:720px;margin:0 auto;padding:22px 16px 42px}.tm-section{background:var(--field);border:1px solid var(--line);border-radius:12px;box-shadow:0 2px 10px #17212b0b;padding:22px}.eyebrow{color:var(--accent);font-size:12px;font-weight:700;text-transform:uppercase;letter-spacing:.08em}h1{font-size:28px;margin:7px 0}.lead{color:var(--muted)}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(190px,1fr));gap:10px;margin-top:20px}.set{display:block;text-decoration:none;color:inherit;border:1px solid var(--line);border-radius:10px;padding:14px;background:var(--field);transition:border-color .15s,transform .15s}.set:hover{border-color:var(--accent);transform:translateY(-1px)}.emoji{width:52px;height:52px;border-radius:12px;background:#e8f3fc;display:grid;place-items:center;font-size:30px;margin-bottom:10px}.name{font-weight:700}.meta{color:var(--muted);font-size:13px;margin-top:3px}.empty{color:var(--muted);padding:20px 0}@media(prefers-color-scheme:dark){:root{color:#e8edf2;background:#17212b;--field:#212a33;--line:#35424f;--muted:#aab7c4}body{background:linear-gradient(#1b2d3d,#17212b 50%)}.tm-header,.set{background:#212a33}.emoji{background:#2b4252}}
</style></head><body><header class="tm-header"><div class="logo">F</div><div class="header-title">Stickers</div><div class="header-sub">{{.AppName}} · local</div></header><main class="tm-main"><section class="tm-section"><div class="eyebrow">Gramsrv mini app</div><h1>Sticker sets</h1><p class="lead">A self-hosted catalog. Data and links come from the local Gramsrv API; no Telegram CDN is used.</p><div class="grid" id="sets"><div class="empty">Loading local sets…</div></div></section></main><script>
(()=>{const root=document.getElementById('sets');fetch('/api/miniapps/stickers').then(r=>{if(!r.ok)throw new Error();return r.json()}).then(items=>{root.textContent='';items.forEach(item=>{const link=document.createElement('a');link.className='set';link.href='/api/miniapps/stickers/'+encodeURIComponent(item.short_name);link.addEventListener('click',e=>e.preventDefault());const icon=document.createElement('div');icon.className='emoji';icon.textContent=item.emoji||'🙂';const name=document.createElement('div');name.className='name';name.textContent=item.title;const meta=document.createElement('div');meta.className='meta';meta.textContent=String(item.count)+' stickers · '+item.updated;link.append(icon,name,meta);root.append(link)})}).catch(()=>{root.innerHTML='<div class="empty">Local catalog is unavailable.</div>'})})();</script></body></html>`))
