'use strict';

// Screenmit on the web: reads the Screenmit folder in the user's own Google
// Drive straight from the browser (no server). The layout and rules are the
// ones the PC app uses (internal/drive/sync.go):
//
//   Screenmit/            appProperties screenmit=root
//     settings.json       {retentionDays, updatedAt}
//     <shot>.png          screenmit=shot, id, pc, t, w, h, app, exe, pin, u, thumb
//                         description = {"title", "note"}
//     .thumbs/<id>.jpg    screenmit=thumb
//
// Without Google (no client id, not signed in, or Drive unreachable) it
// shows this PC's screenshots instead, from the Screenmit running here: the PC
// app's "웹에서 열기" link hands over its port and key once (#local=port.key).
//
// Loaded before app.js; app.js picks this up as its data source.
(() => {
  const CFG = Object.assign({
    clientId: '',
    authUrl: 'https://accounts.google.com/o/oauth2/v2/auth',
    apiBase: 'https://www.googleapis.com',
    revokeUrl: 'https://oauth2.googleapis.com/revoke',
    version: 'web',
    siteUrl: '', // canonical address; the Google redirect URI
  }, window.SCREENMIT_CONFIG || {});
  const redirectUri = () => CFG.siteUrl || location.origin + location.pathname;
  const SCOPE = 'https://www.googleapis.com/auth/drive.file';
  const KEY = 'screenmit.web.';
  const THUMB_CACHE = 'screenmit-thumbs-v1';
  const RETENTION = [3, 7, 14, 30];

  const store = {
    get(k) { try { return localStorage.getItem(KEY + k); } catch (e) { return null; } },
    set(k, v) { try { if (v == null) localStorage.removeItem(KEY + k); else localStorage.setItem(KEY + k, v); } catch (e) { /* private mode */ } },
  };
  const clientId = () => CFG.clientId || store.get('clientId') || '';
  const enc = encodeURIComponent;
  const escH = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

  // ---------- sign-in: OAuth by redirect; the token lasts an hour ----------
  let token = null;
  try { token = JSON.parse(store.get('token') || 'null'); } catch (e) { token = null; }
  let loginError = '';

  (function takeRedirect() {
    const h = location.hash;
    if (!h.includes('access_token=') && !h.includes('error=')) return;
    const p = new URLSearchParams(h.slice(1));
    let want = null;
    try { want = sessionStorage.getItem(`${KEY}state`); sessionStorage.removeItem(`${KEY}state`); } catch (e) { /* ignore */ }
    if (p.get('error')) {
      loginError = p.get('error') === 'access_denied' ? '구글 로그인을 취소했어요' : `구글 로그인 오류: ${p.get('error')}`;
    } else if (!want || p.get('state') !== want) {
      loginError = '로그인 응답이 맞지 않아요. 다시 눌러 주세요';
    } else {
      token = { access: p.get('access_token'), exp: Date.now() + (Number(p.get('expires_in') || 3600) - 60) * 1000 };
      store.set('token', JSON.stringify(token));
    }
    history.replaceState(null, '', location.pathname + location.search);
  })();

  // "#local=<port>.<key>" from the PC app's "웹에서 열기" link.
  function takeLocalLink() {
    const m = location.hash.match(/(?:^#|&)local=(\d{2,5})\.([0-9a-f]{32,})/);
    if (!m) return false;
    store.set('local', `${m[1]}.${m[2]}`);
    history.replaceState(null, '', location.pathname + location.search);
    return true;
  }
  takeLocalLink();
  // The link opened in a tab that already shows this page.
  window.addEventListener('hashchange', () => { if (takeLocalLink()) location.reload(); });
  function localConn() {
    const v = store.get('local') || '';
    const m = v.match(/^(\d{2,5})\.([0-9a-f]{32,})$/);
    return m ? { base: `http://127.0.0.1:${m[1]}`, key: m[2] } : null;
  }

  const valid = () => !!(token && token.access && Date.now() < token.exp);
  function dropToken() { token = null; store.set('token', null); }

  function randomState() {
    const a = new Uint8Array(16);
    crypto.getRandomValues(a);
    return Array.from(a, (b) => b.toString(16).padStart(2, '0')).join('');
  }

  function login() {
    const st = randomState();
    try { sessionStorage.setItem(`${KEY}state`, st); } catch (e) { /* ignore */ }
    const q = new URLSearchParams({
      client_id: clientId(), redirect_uri: redirectUri(), response_type: 'token',
      scope: SCOPE, include_granted_scopes: 'true', state: st,
    });
    const hint = store.get('email');
    if (hint) q.set('login_hint', hint);
    location.assign(`${CFG.authUrl}?${q}`);
  }

  async function logout() {
    const t = token && token.access;
    dropToken();
    store.set('email', null);
    try { await caches.delete(THUMB_CACHE); } catch (e) { /* ignore */ }
    if (t) { try { await fetch(`${CFG.revokeUrl}?token=${enc(t)}`, { method: 'POST', mode: 'no-cors' }); } catch (e) { /* best effort */ } }
    location.reload();
  }

  const authError = (msg) => Object.assign(new Error(msg || ''), { unauthorized: true });

  // ---------- Drive REST ----------
  async function call(method, path, opt = {}) {
    if (!valid()) { const had = !!token; dropToken(); throw authError(had ? '로그인 시간이 지났어요. 다시 눌러 주세요' : ''); }
    let r;
    try {
      r = await fetch(CFG.apiBase + path, { method, headers: Object.assign({ Authorization: `Bearer ${token.access}` }, opt.headers || {}), body: opt.body });
    } catch (e) {
      throw Object.assign(new Error('인터넷 연결을 확인해 주세요'), { offline: true });
    }
    if (r.status === 401) { dropToken(); throw authError('로그인 시간이 지났어요. 다시 눌러 주세요'); }
    if (!r.ok) {
      let msg = `드라이브 오류 (${r.status})`;
      try { const j = await r.json(); if (j.error && j.error.message) msg += ` ${j.error.message}`; } catch (e) { /* ignore */ }
      throw Object.assign(new Error(msg), { status: r.status });
    }
    if (opt.raw) return r;
    if (r.status === 204) return null;
    return r.json();
  }
  const jsonBody = (o) => ({ headers: { 'Content-Type': 'application/json; charset=UTF-8' }, body: JSON.stringify(o) });

  async function list(q, fields) {
    const out = [];
    let page = '';
    do {
      const qs = new URLSearchParams({ q, fields: `nextPageToken,files(${fields})`, pageSize: '1000', spaces: 'drive', orderBy: 'createdTime' });
      if (page) qs.set('pageToken', page);
      const j = await call('GET', `/drive/v3/files?${qs}`);
      out.push(...(j.files || []));
      page = j.nextPageToken || '';
    } while (page);
    return out;
  }
  const oldest = (fs) => fs.slice().sort((a, b) => (a.createdTime < b.createdTime ? -1 : 1))[0];

  // ---------- the collection ----------
  let email = store.get('email') || '';
  let root = '';
  let settingsId = '';
  let settings = { retentionDays: 14, updatedAt: 0 };
  let shots = new Map();

  const ymd = (d) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  function dayDiff(a, b) {
    const p = (s) => { const [y, m, d] = s.split('-').map(Number); return Date.UTC(y, m - 1, d); };
    return Math.round((p(a) - p(b)) / 86400000);
  }

  async function findRoot() {
    if (root) return root;
    const rs = await list("appProperties has { key='screenmit' and value='root' } and trashed = false", 'id,createdTime');
    root = rs.length ? oldest(rs).id : '';
    return root;
  }

  async function readSettings() {
    if (!settingsId) {
      const fs = await list(`'${root}' in parents and appProperties has { key='screenmit' and value='settings' } and trashed = false`, 'id,createdTime');
      settingsId = fs.length ? oldest(fs).id : '';
    }
    if (!settingsId) return;
    try {
      const r = await call('GET', `/drive/v3/files/${enc(settingsId)}?alt=media`, { raw: true });
      const j = await r.json();
      if (j && j.retentionDays > 0) settings = { retentionDays: j.retentionDays, updatedAt: j.updatedAt || 0 };
    } catch (e) {
      if (e.status === 404) settingsId = '';
      else throw e;
    }
  }

  async function driveState() {
    if (!email) {
      const a = await call('GET', '/drive/v3/about?fields=user(emailAddress)');
      email = (a.user && a.user.emailAddress) || '';
      store.set('email', email);
    }
    const today = ymd(new Date());
    const next = new Map();
    if (await findRoot()) {
      await readSettings();
      let files;
      try {
        files = await list(`'${root}' in parents and appProperties has { key='screenmit' and value='shot' } and trashed = false`,
          'id,name,size,createdTime,description,appProperties');
      } catch (e) {
        if (e.status === 404) { root = ''; settingsId = ''; }
        throw e;
      }
      for (const f of files) {
        const p = f.appProperties || {};
        if (!p.id || next.has(p.id)) continue;
        let d = {};
        try { d = JSON.parse(f.description || '{}') || {}; } catch (e) { d = {}; }
        const t = new Date(Number(p.t) || Date.parse(f.createdTime));
        const day = ymd(t);
        const pinned = p.pin === '1';
        if (!pinned && settings.retentionDays - dayDiff(today, day) <= 0) continue; // past its time; a PC removes it
        next.set(p.id, {
          id: p.id, day, time: t.toISOString(), w: +p.w || 0, h: +p.h || 0, bytes: +f.size || 0,
          app: p.app || '', exe: p.exe || '', title: d.title || '', note: d.note || '', pinned, pc: p.pc || '',
          remote: true, synced: true, name: f.name || `Screenmit_${day}.png`, driveId: f.id, thumbId: p.thumb || '',
        });
      }
    }
    shots = next;
    const list_ = [...shots.values()].sort((a, b) => (a.time !== b.time ? (a.time < b.time ? 1 : -1) : (a.id < b.id ? -1 : 1)));
    return {
      version: CFG.version, platform: 'web', paused: false, pc: null, today, email,
      settings: { retentionDays: settings.retentionDays, retentionChoices: RETENTION, hotkey: 'off' },
      drive: { on: true, configured: true, status: { email, connected: true, syncing: false, lastSync: new Date().toISOString(), pcs: [] } },
      stats: { count: list_.length, bytes: list_.reduce((n, s) => n + s.bytes, 0), local: 0 },
      shots: list_.map((s) => ({ ...s })),
    };
  }

  function shot(id) {
    const s = shots.get(id);
    if (!s) throw new Error('없는 스크린샷이에요. 새로 고침해 주세요');
    return s;
  }

  // ---------- thumbnails: fetched with the token, kept in the browser cache ----------
  const thumbURLs = new Map(); // thumbId -> object URL
  const inflight = new Map();
  let running = 0;
  const queue = [];
  const slot = () => new Promise((res) => { if (running < 6) { running++; res(); } else queue.push(res); });
  const release = () => { const n = queue.shift(); if (n) n(); else running--; };
  let cacheP = null;
  const openCache = () => {
    if (!cacheP) cacheP = (window.caches ? caches.open(THUMB_CACHE) : Promise.reject(new Error('no cache'))).catch(() => null);
    return cacheP;
  };

  // key names the thumbnail for good (Drive file id, or the shot id on this
  // PC: ids are picture hashes), so cached copies never go stale.
  function thumbURL(key, fetchBlob) {
    if (!key) return Promise.reject(new Error('no thumbnail'));
    if (thumbURLs.has(key)) return Promise.resolve(thumbURLs.get(key));
    if (inflight.has(key)) return inflight.get(key);
    const p = (async () => {
      const c = await openCache();
      const ck = `${location.origin}/__thumb/${enc(key)}`;
      let blob = null;
      if (c) { const hit = await c.match(ck); if (hit) blob = await hit.blob(); }
      if (!blob) {
        await slot();
        try { blob = await fetchBlob(); } finally { release(); }
        if (c) c.put(ck, new Response(blob, { headers: { 'Content-Type': 'image/jpeg' } })).catch(() => {});
      }
      const url = URL.createObjectURL(blob);
      thumbURLs.set(key, url);
      return url;
    })();
    inflight.set(key, p);
    p.finally(() => inflight.delete(key)).catch(() => {});
    return p;
  }

  let observer = null;
  function showThumb(img) {
    const box = img.parentElement;
    const id = img.dataset.thumb;
    thumbURL(cur().thumbKey(id), () => cur().thumbBlob(id)).then((u) => { img.src = u; }).catch(() => {})
      .finally(() => { if (box) box.classList.remove('loading'); });
  }
  function attachThumbs(rootEl) {
    if (!observer && 'IntersectionObserver' in window) {
      observer = new IntersectionObserver((entries) => {
        for (const e of entries) if (e.isIntersecting) { observer.unobserve(e.target); showThumb(e.target); }
      }, { rootMargin: '800px 0px' });
    }
    rootEl.querySelectorAll('img[data-thumb]').forEach((img) => {
      if (img.getAttribute('src')) return;
      img.parentElement.classList.add('loading');
      if (observer) observer.observe(img); else showThumb(img);
    });
  }

  // ---------- full pictures ----------
  const fulls = new Map(); // "source:id" -> object URL, the 20 most recent
  async function full(id) {
    const k = `${mode}:${id}`;
    if (fulls.has(k)) { const u = fulls.get(k); fulls.delete(k); fulls.set(k, u); return u; }
    const url = URL.createObjectURL(await cur().fullBlob(id));
    fulls.set(k, url);
    while (fulls.size > 20) {
      const [k, u] = fulls.entries().next().value;
      fulls.delete(k);
      URL.revokeObjectURL(u);
    }
    return url;
  }
  const fullBlob = (id) => full(id).then((u) => fetch(u)).then((r) => r.blob())
    .then((b) => (b.type === 'image/png' ? b : new Blob([b], { type: 'image/png' })));

  async function driveCopy(id) {
    if (!navigator.clipboard || !window.ClipboardItem) throw new Error('이 브라우저는 그림 복사를 못 해요. 저장을 써 주세요');
    const blobP = fullBlob(id);
    try {
      await navigator.clipboard.write([new ClipboardItem({ 'image/png': blobP })]); // Safari wants the promise
    } catch (e) {
      await navigator.clipboard.write([new ClipboardItem({ 'image/png': await blobP })]);
    }
  }

  async function download(s) {
    const blob = await fullBlob(s.id);
    const file = new File([blob], s.name, { type: 'image/png' });
    if (matchMedia('(pointer: coarse)').matches && navigator.canShare && navigator.canShare({ files: [file] })) {
      try { await navigator.share({ files: [file] }); return; } catch (e) { if (e.name === 'AbortError') return; }
    }
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = s.name;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(a.href), 30000);
  }

  // ---------- changes (same rules as the PC app: newest "u" wins) ----------
  async function pin(id, v) {
    const s = shot(id);
    const u = Date.now();
    await call('PATCH', `/drive/v3/files/${enc(s.driveId)}?fields=id`, jsonBody({ appProperties: { pin: v ? '1' : '0', u: String(u) } }));
    s.pinned = v;
  }

  async function note(id, text) {
    const s = shot(id);
    const u = Date.now();
    await call('PATCH', `/drive/v3/files/${enc(s.driveId)}?fields=id`, jsonBody({
      description: JSON.stringify({ title: s.title, note: text }), appProperties: { u: String(u) },
    }));
    s.note = text;
  }

  async function del(id) {
    const s = shots.get(id);
    if (!s) return;
    try { await call('DELETE', `/drive/v3/files/${enc(s.driveId)}`); } catch (e) { if (e.status !== 404) throw e; }
    if (s.thumbId) { try { await call('DELETE', `/drive/v3/files/${enc(s.thumbId)}`); } catch (e) { /* the PCs tidy up */ } }
    shots.delete(id);
  }

  async function saveSettings(patch) {
    if (patch.retentionDays == null) return;
    const next = { retentionDays: Number(patch.retentionDays), updatedAt: Date.now() };
    if (!RETENTION.includes(next.retentionDays)) throw new Error('보관 기간이 맞지 않아요');
    const body = JSON.stringify(next);
    if (settingsId) {
      await call('PATCH', `/upload/drive/v3/files/${enc(settingsId)}?uploadType=media&fields=id`, { headers: { 'Content-Type': 'application/json' }, body });
    } else if (await findRoot()) {
      const b = `screenmit-${randomState()}`;
      const meta = JSON.stringify({ name: 'settings.json', parents: [root], mimeType: 'application/json', appProperties: { screenmit: 'settings' } });
      const multipart = `--${b}\r\nContent-Type: application/json; charset=UTF-8\r\n\r\n${meta}\r\n--${b}\r\nContent-Type: application/json\r\n\r\n${body}\r\n--${b}--\r\n`;
      const f = await call('POST', '/upload/drive/v3/files?uploadType=multipart&fields=id', { headers: { 'Content-Type': `multipart/related; boundary=${b}` }, body: multipart });
      settingsId = f.id;
    } else {
      throw new Error('드라이브에 Screenmit 폴더가 아직 없어요');
    }
    settings = next;
  }

  function live(onChange) {
    let last = Date.now();
    setInterval(() => {
      if (document.hidden) return;
      const every = mode === 'pc' ? 15000 : 60000;
      if (Date.now() - last >= every - 500) { last = Date.now(); onChange(); }
    }, 15000);
    document.addEventListener('visibilitychange', () => { if (!document.hidden) onChange(); });
    window.addEventListener('online', () => onChange());
  }

  // ---------- sign-in screen ----------
  function gateHTML(err) {
    const errLine = err ? `<p class="err">${escH(err)}</p>` : '';
    if (!clientId()) {
      return `<div class="gate"><img src="assets/icon.svg" alt=""><h2>Screenmit</h2>
        <p>이 페이지를 쓰려면 Google Cloud 의 <b>웹 애플리케이션</b> 클라이언트 ID 가 필요해요.</p>
        <ol>
          <li>PC용 Screenmit 과 <b>같은 프로젝트</b>에서 클라이언트 만들기 → 유형 <b>웹 애플리케이션</b></li>
          <li>승인된 JavaScript 원본: <code>${escH(new URL(redirectUri()).origin)}</code></li>
          <li>승인된 리디렉션 URI: <code>${escH(redirectUri())}</code></li>
          <li>만든 클라이언트 ID 를 아래에 붙여 넣기 (이 브라우저에만 저장돼요)</li>
        </ol>
        <input id="clientIdInput" placeholder="….apps.googleusercontent.com" spellcheck="false" autocomplete="off">
        <button type="button" class="btn primary" id="clientIdSave">저장</button>${errLine}</div>`;
    }
    const hint = store.get('email');
    return `<div class="gate"><img src="assets/icon.svg" alt=""><h2>Screenmit</h2>
      <p>회사·집 PC에서 모은 스크린샷을 어디서든 봐요.</p>
      <button type="button" class="btn primary" id="loginBtn">구글 계정으로 보기</button>
      ${hint ? `<small>${escH(hint)}</small>` : ''}
      <small>내 드라이브에서 Screenmit 이 만든 파일만 읽어요. 서버를 거치지 않아요. <a href="privacy.html">개인정보처리방침</a></small>${errLine}</div>`;
  }

  function wireGate(el) {
    const b = el.querySelector('#loginBtn');
    if (b) b.addEventListener('click', login);
    const save = el.querySelector('#clientIdSave');
    if (save) {
      save.addEventListener('click', () => {
        const v = el.querySelector('#clientIdInput').value.trim();
        if (!/^[\w.-]+\.apps\.googleusercontent\.com$/.test(v)) {
          el.querySelector('#clientIdInput').focus();
          return;
        }
        store.set('clientId', v);
        location.reload();
      });
    }
  }

  // ---------- the two sources ----------
  const drive = {
    state: driveState,
    thumbKey: (id) => (shots.get(id) || {}).thumbId || '',
    thumbBlob: async (id) => (await call('GET', `/drive/v3/files/${enc(shot(id).thumbId)}?alt=media`, { raw: true })).blob(),
    fullBlob: async (id) => (await call('GET', `/drive/v3/files/${enc(shot(id).driveId)}?alt=media`, { raw: true })).blob(),
    copy: driveCopy,
    pin,
    note,
    del,
    settings: saveSettings,
  };

  // This PC: the Screenmit running here, with the key from its link.
  let localError = '';
  async function lcall(method, path, opt = {}) {
    const L = localConn();
    if (!L) throw Object.assign(new Error(''), { noLocal: true });
    const headers = { Authorization: `Bearer ${L.key}`, 'X-Screenmit': '1' };
    if (opt.json !== undefined) headers['Content-Type'] = 'application/json';
    let r;
    try {
      r = await fetch(L.base + path, { method, headers, body: opt.json !== undefined ? JSON.stringify(opt.json) : undefined });
    } catch (e) {
      // Off, or the browser asked "allow access to this device?" and it was refused.
      throw Object.assign(new Error('이 PC의 Screenmit 에 연결하지 못했어요. Screenmit 이 켜져 있는지, 브라우저가 이 기기 접근을 물었다면 허용했는지 확인해 주세요'), { noLocal: true });
    }
    if (r.status === 401) {
      store.set('local', null);
      throw Object.assign(new Error('이 PC의 Screenmit 과 연결이 끊겼어요. PC 앱 설정 → 정보 → 웹에서 열기를 다시 눌러 주세요'), { noLocal: true });
    }
    if (!r.ok) {
      let m = `${r.status}`;
      try { m = (await r.json()).error || m; } catch (e) { /* ignore */ }
      throw new Error(m);
    }
    return opt.raw ? r : r.json();
  }
  const thisPC = {
    async state() {
      const d = await lcall('GET', '/api/state');
      d.viaPC = true;
      d.email = email;
      return d;
    },
    thumbKey: (id) => `pc:${id}`,
    thumbBlob: async (id) => (await lcall('GET', `/thumb/${enc(id)}`, { raw: true })).blob(),
    fullBlob: async (id) => (await lcall('GET', `/img/${enc(id)}`, { raw: true })).blob(),
    copy: (id) => lcall('POST', `/api/shots/${enc(id)}/copy`), // the same PC: its own clipboard
    pin: (id, v) => lcall('PATCH', `/api/shots/${enc(id)}`, { json: { pinned: v } }),
    note: (id, text) => lcall('PATCH', `/api/shots/${enc(id)}`, { json: { note: text } }),
    del: (id) => lcall('DELETE', `/api/shots/${enc(id)}`),
    settings: (patch) => lcall('POST', '/api/settings', { json: patch }),
  };

  let mode = 'drive';
  const cur = () => (mode === 'pc' ? thisPC : drive);

  // Google first; without it (or when it fails), this PC; else the sign-in screen.
  async function state() {
    let googleErr;
    if (clientId() && valid()) {
      try {
        const d = await drive.state();
        mode = 'drive';
        return d;
      } catch (e) {
        googleErr = e;
      }
    } else {
      googleErr = authError(loginError || (token ? '로그인 시간이 지났어요. 다시 눌러 주세요' : ''));
      loginError = '';
    }
    localError = '';
    if (localConn()) {
      try {
        const d = await thisPC.state();
        mode = 'pc';
        d.notice = noticeFor(googleErr);
        return d;
      } catch (e) {
        localError = e.message;
      }
    }
    mode = 'none';
    throw googleErr;
  }

  function noticeFor(err) {
    const why = err && err.message ? `${err.message} · ` : '';
    if (!clientId()) return { text: '구글 로그인 없이 이 PC의 스크린샷만 보여요' };
    if (err && err.unauthorized) return { text: `${why}이 PC의 스크린샷만 보여요. 구글로 로그인하면 다른 PC 것도 보여요`, action: '구글로 로그인' };
    return { text: '구글 드라이브에 연결하지 못해서 이 PC의 스크린샷만 보여요', action: '다시 시도' };
  }

  window.SCREENMIT_BACKEND = {
    mode: 'drive',
    state,
    gateHTML: (err) => gateHTML(err).replace('</div>', `${localHint()}</div>`),
    wireGate,
    offlineText: '구글 드라이브에 연결하지 못했어요.',
    thumbAttr: (s) => {
      const u = thumbURLs.get(cur().thumbKey(s.id));
      return u ? `src="${u}" data-thumb="${s.id}"` : `data-thumb="${s.id}"`;
    },
    thumbNow: (id) => thumbURLs.get(cur().thumbKey(id)) || '',
    attachThumbs,
    full,
    copy: (id) => cur().copy(id),
    download,
    pin: (id, v) => cur().pin(id, v),
    note: (id, text) => cur().note(id, text),
    del: (id) => cur().del(id),
    settings: (patch) => cur().settings(patch),
    live,
    refresh() { root = ''; settingsId = ''; },
    loggedIn: () => clientId() !== '' && valid(),
    login,
    logout,
    noticeAction() { if (clientId() && !valid()) login(); else location.reload(); },
  };

  function localHint() {
    if (localError) return `<small class="err">${escH(localError)}</small>`;
    return '<small>이 PC에서 Screenmit 을 쓰고 있다면, PC 앱 설정 → 정보 → <b>웹에서 열기</b>로 한 번 열면 로그인 없이 이 PC 스크린샷이 보여요.</small>';
  }
})();
