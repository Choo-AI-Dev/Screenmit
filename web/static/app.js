'use strict';

// ---------- icons (stroke icons, 24 grid) ----------
const P = (d, extra = '') => `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${d}${extra}</svg>`;
const ICONS = {
  search: P('<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>'),
  folder: P('<path d="M3 7.5A2.5 2.5 0 0 1 5.5 5H9l2 2.2h7.5A2.5 2.5 0 0 1 21 9.7v7.8a2.5 2.5 0 0 1-2.5 2.5h-13A2.5 2.5 0 0 1 3 17.5z"/>'),
  gear: P('<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/>'),
  copy: P('<rect x="9" y="9" width="11" height="11" rx="2.2"/><path d="M5 15H4.5A1.5 1.5 0 0 1 3 13.5v-9A1.5 1.5 0 0 1 4.5 3h9A1.5 1.5 0 0 1 15 4.5V5"/>'),
  star: P('<path d="M12 3.2l2.7 5.5 6 .9-4.35 4.25 1.03 6-5.38-2.83-5.38 2.83 1.03-6L3.3 9.6l6-.9z"/>'),
  starFill: '<svg viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M12 2.6l2.9 5.9 6.5.95-4.7 4.6 1.1 6.45L12 17.45 6.2 20.5l1.1-6.45-4.7-4.6 6.5-.95z"/></svg>',
  note: P('<path d="M12 20h8"/><path d="M16.4 3.6a2 2 0 0 1 2.9 2.9L7.5 18.3 3.5 19.5l1.2-4z"/>'),
  reveal: P('<path d="M3 7.5A2.5 2.5 0 0 1 5.5 5H9l2 2.2h7.5A2.5 2.5 0 0 1 21 9.7v1.3"/><path d="M3 7.5v10A2.5 2.5 0 0 0 5.5 20H11"/><circle cx="17" cy="16" r="3"/><path d="m21.5 20.5-2.3-2.3"/>'),
  download: P('<path d="M12 4v11"/><path d="m7 10.5 5 5 5-5"/><path d="M5 20h14"/>'),
  trash: P('<path d="M4 7h16"/><path d="M9.5 7V4.8c0-.44.36-.8.8-.8h3.4c.44 0 .8.36.8.8V7"/><path d="M6.5 7l.8 12.2c.07 1 .9 1.8 1.9 1.8h5.6c1 0 1.83-.8 1.9-1.8L17.5 7"/>'),
  x: P('<path d="M18 6 6 18"/><path d="m6 6 12 12"/>'),
  left: P('<path d="m15 5-7 7 7 7"/>'),
  right: P('<path d="m9 5 7 7-7 7"/>'),
  cloud: P('<path d="M7 18.5a4.5 4.5 0 0 1-.6-8.96A6 6 0 0 1 17.9 9.1 4.75 4.75 0 0 1 17.25 18.5z"/>'),
  monitor: P('<rect x="3" y="4" width="18" height="12.5" rx="2"/><path d="M9 20.5h6"/><path d="M12 16.5v4"/>'),
};
const icon = (n) => `<span class="i">${ICONS[n] || ''}</span>`;
function paintIcons(root = document) {
  root.querySelectorAll('[data-icon]').forEach((el) => { if (!el.firstChild) el.innerHTML = ICONS[el.dataset.icon] || ''; });
}

// ---------- helpers ----------
const $ = (s, r = document) => r.querySelector(s);
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const WD = ['일', '월', '화', '수', '목', '금', '토'];
const WEB_URL = 'https://choo-ai-dev.github.io/Screenmit/';

function parseDay(d) { const [y, m, dd] = d.split('-').map(Number); return new Date(y, m - 1, dd); }
function dayDiff(a, b) { return Math.round((parseDay(a) - parseDay(b)) / 86400000); }
function dayLabel(day, today) {
  const d = parseDay(day);
  const full = `${d.getMonth() + 1}월 ${d.getDate()}일 ${WD[d.getDay()]}요일`;
  const yr = d.getFullYear() !== parseDay(today).getFullYear() ? `${d.getFullYear()}년 ` : '';
  const diff = dayDiff(today, day);
  if (diff === 0) return { title: '오늘', sub: full };
  if (diff === 1) return { title: '어제', sub: full };
  return { title: yr + full, sub: '' };
}
function clock(iso) {
  const d = new Date(iso);
  const h = d.getHours(); const m = String(d.getMinutes()).padStart(2, '0');
  return `${h < 12 ? '오전' : '오후'} ${h % 12 || 12}:${m}`;
}
function fullWhen(iso, today) {
  const d = new Date(iso);
  const day = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  const l = dayLabel(day, today);
  const date = l.sub ? `${l.title} · ${l.sub.replace(/요일$/, '')}` : l.title.replace(/요일$/, '');
  return `${date} ${clock(iso)}:${String(d.getSeconds()).padStart(2, '0')}`;
}
function bytes(n) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${Math.round(n / 1024)} KB`;
  if (n < 1024 ** 3) return `${(n / 1024 / 1024).toFixed(n < 10 * 1024 * 1024 ? 1 : 0)} MB`;
  return `${(n / 1024 ** 3).toFixed(1)} GB`;
}
function ago(iso) {
  if (!iso || iso.startsWith('0001')) return '';
  const s = (Date.now() - new Date(iso)) / 1000;
  if (s < 50) return '방금';
  if (s < 3600) return `${Math.round(s / 60)}분 전`;
  if (s < 86400) return `${Math.round(s / 3600)}시간 전`;
  return `${Math.round(s / 86400)}일 전`;
}
function daysLeft(day) { return S.data.settings.retentionDays - dayDiff(S.data.today, day); }
function expireText(n) {
  if (n >= 3) return { t: `${n}일 남음`, soon: false };
  if (n === 2) return { t: '내일까지 보관', soon: true };
  return { t: '오늘까지 보관', soon: true };
}
function hotkeyText(s) { return !s || s === 'off' ? '꺼짐' : s.split('+').join(' + '); }

// ---------- state ----------
const S = {
  data: null, tab: 'all', pc: '', q: '',
  known: null, pendingDel: null, viewer: null,
  offline: false, unauthorized: false,
};

async function api(method, path, body, raw) {
  const opt = { method, headers: { 'X-Screenmit': '1' } };
  if (body !== undefined) {
    if (raw) opt.body = body;
    else { opt.headers['Content-Type'] = 'application/json'; opt.body = JSON.stringify(body); }
  }
  const r = await fetch(path, opt);
  const j = await r.json().catch(() => ({}));
  if (r.status === 401) { S.unauthorized = true; render(); }
  if (!r.ok) throw new Error(j.error || `${r.status}`);
  return j;
}

// ---------- data source ----------
// The PC app serves this page with its own API ("local"). The web page on
// GitHub Pages sets window.SCREENMIT_BACKEND (drive.js) to read Google Drive.
const localBackend = {
  mode: 'local',
  async state() {
    let r;
    try { r = await fetch('/api/state', { cache: 'no-store' }); } catch (e) { throw Object.assign(new Error('offline'), { offline: true }); }
    if (r.status === 401) throw Object.assign(new Error(''), { unauthorized: true });
    if (!r.ok) throw new Error(`${r.status}`);
    return r.json();
  },
  gateHTML() {
    return `<div class="empty"><div class="art">${artSvg()}</div><h2>이 창은 다시 열어야 해요</h2>
      <p>작업 표시줄 오른쪽(트레이)의 Screenmit 아이콘을 눌러 주세요.</p></div>`;
  },
  offlineText: 'Screenmit 이 꺼져 있어요. 시작 메뉴에서 다시 실행하면 이어서 모아요.',
  thumbAttr: (s) => `src="/thumb/${s.id}"`,
  thumbNow: (id) => `/thumb/${id}`,
  attachThumbs() {},
  full: (id) => Promise.resolve(`/img/${id}`),
  copy: (id) => api('POST', `/api/shots/${id}/copy`),
  pin: (id, v) => api('PATCH', `/api/shots/${id}`, { pinned: v }),
  note: (id, text) => api('PATCH', `/api/shots/${id}`, { note: text }),
  del: (id, keepalive) => fetch(`/api/shots/${id}`, { method: 'DELETE', headers: { 'X-Screenmit': '1' }, keepalive: !!keepalive }),
  settings: (patch) => api('POST', '/api/settings', patch),
  live(onChange) {
    const es = new EventSource('/api/events');
    es.addEventListener('hello', () => { S.offline = false; scheduleLoad(10); });
    es.addEventListener('change', () => onChange());
    es.onerror = () => {
      fetch('/api/state', { cache: 'no-store' }).then((r) => { if (r.status === 401) { S.unauthorized = true; render(); es.close(); } })
        .catch(() => { S.offline = true; renderBanner(); });
    };
  },
};
const B = window.SCREENMIT_BACKEND || localBackend;
const WEB = B.mode !== 'local';
if (WEB) document.body.classList.add('web');

let loadTimer = 0;
function scheduleLoad(ms = 120) { clearTimeout(loadTimer); loadTimer = setTimeout(load, ms); }
async function load() {
  try {
    const d = await B.state();
    S.unauthorized = false; S.offline = false; S.gateError = '';
    S.data = d;
    render();
  } catch (e) {
    if (e && e.unauthorized) { S.unauthorized = true; S.gateError = e.message || ''; render(); return; }
    S.offline = true; S.offlineMsg = (e && !e.offline && e.message) || ''; renderBanner();
  }
}

function byId(id) { return S.data.shots.find((s) => s.id === id); }

// Google sign-in gone, or Drive unreachable for a while (a company network
// that blocks Google): show only this PC's screenshots, quietly.
function driveDown(d = S.data) {
  const st = d && d.drive && d.drive.on ? d.drive.status : null;
  if (!st || WEB && !d.viaPC) return false;
  if (st.needLogin) return true;
  if (!st.error) return false;
  const last = st.lastSync && !st.lastSync.startsWith('0001') ? Date.parse(st.lastSync) : 0;
  return Date.now() - last > 10 * 60 * 1000;
}
function visibleShots() {
  return driveDown() ? S.data.shots.filter((s) => !s.remote) : S.data.shots;
}
function pcsInData() {
  const set = new Set(visibleShots().map((s) => s.pc || S.data.pc));
  set.add(S.data.pc);
  if (!driveDown()) (S.data.drive.status?.pcs || []).forEach((p) => set.add(p));
  return [...set].filter(Boolean);
}
function filtered() {
  const terms = S.q.trim().toLowerCase().split(/\s+/).filter(Boolean);
  return visibleShots().filter((s) => {
    if (S.pendingDel && S.pendingDel.id === s.id) return false;
    if (S.tab === 'kept' && !s.pinned) return false;
    if (S.pc && (s.pc || S.data.pc) !== S.pc) return false;
    if (!terms.length) return true;
    const hay = `${s.app} ${s.exe} ${s.title} ${s.note} ${s.pc} ${s.name}`.toLowerCase();
    return terms.every((t) => hay.includes(t));
  });
}

// ---------- rendering ----------
function render() {
  document.body.classList.toggle('gated', !!S.unauthorized);
  if (S.unauthorized) {
    if (S.viewer) closeViewer();
    $('#settings').hidden = true;
    $('#list').innerHTML = B.gateHTML(S.gateError);
    if (B.wireGate) B.wireGate($('#list'));
    return;
  }
  if (!S.data) return;
  renderTop();
  renderBanner();
  renderList();
  if (!$('#settings').hidden) renderSettings();
  if (S.viewer) refreshViewer();
}

function renderTop() {
  const d = S.data;
  const pb = $('#pauseBtn');
  pb.classList.toggle('paused', d.paused);
  pb.querySelector('.label').textContent = d.paused ? '일시 정지됨' : '모으는 중';
  pb.title = d.paused ? '눌러서 다시 모으기' : '눌러서 잠깐 멈추기 (비밀번호 등을 캡처할 때)';

  const sb = $('#syncBtn');
  const dr = d.drive;
  sb.hidden = !dr.on;
  if (WEB) {
    const st = dr.status || {};
    sb.classList.toggle('busy', !!st.syncing);
    sb.classList.remove('err');
    sb.title = `새로 고침 · ${st.email || ''}${st.lastSync ? ` · ${ago(st.lastSync)}` : ''}`;
  } else if (dr.on) {
    const st = dr.status || {};
    const down = driveDown(d);
    sb.classList.toggle('busy', !down && (!!st.syncing || dr.connecting));
    sb.classList.remove('err');
    sb.title = down ? '구글 드라이브에 연결되지 않아 이 PC 스크린샷만 보여요 (설정에서 다시 연결)'
      : st.error ? `구글 드라이브: ${st.error}` : st.syncing ? '구글 드라이브와 맞추는 중…'
      : `구글 드라이브 · ${st.email || ''}${st.lastSync ? ` · ${ago(st.lastSync)} 맞춤` : ''}${st.pending ? ` · 올릴 것 ${st.pending}장` : ''}`;
  }

  const all = visibleShots().filter((s) => !(S.pendingDel && S.pendingDel.id === s.id));
  $('#nAll').textContent = all.length;
  $('#nKept').textContent = all.filter((s) => s.pinned).length;
  document.querySelectorAll('.tab').forEach((t) => t.classList.toggle('on', t.dataset.tab === S.tab));

  const pcs = pcsInData();
  const box = $('#pcs');
  if (pcs.length > 1) {
    if (S.pc && !pcs.includes(S.pc)) S.pc = '';
    box.hidden = false;
    box.innerHTML = `<button class="chip ${S.pc ? '' : 'on'}" data-pc="">모든 PC</button>` + pcs.map((p) =>
      `<button class="chip ${S.pc === p ? 'on' : ''}" data-pc="${esc(p)}">${p === d.pc ? icon('monitor') : ''}${esc(p)}</button>`).join('');
  } else {
    box.hidden = true; S.pc = '';
  }
  $('#summary').textContent = WEB && !d.viaPC ? `${d.settings.retentionDays}일 보관 · 드라이브 ${bytes(d.stats.bytes)}`
    : `${d.settings.retentionDays}일 보관 · 이 PC ${bytes(d.stats.local)}`;
}

function renderBanner() {
  const b = $('#banner');
  if (S.offline) {
    b.hidden = false;
    b.textContent = B.offlineText + (S.offlineMsg ? ` (${S.offlineMsg})` : '');
    return;
  }
  const d = S.data;
  if (!d) { b.hidden = true; return; }
  if (d.paused) {
    b.hidden = false;
    b.innerHTML = `일시 정지 중이라 지금은 스크린샷을 모으지 않아요. <button class="btn" id="resumeBtn">다시 모으기</button>`;
  } else if (d.notice) {
    b.hidden = false;
    b.innerHTML = `<span>${esc(d.notice.text)}</span>${d.notice.action ? `<button class="btn" id="noticeBtn">${esc(d.notice.action)}</button>` : ''}`;
  } else {
    b.hidden = true;
  }
}

function artSvg() {
  return `<svg viewBox="0 0 120 96" fill="none" stroke="currentColor" stroke-width="5" stroke-linecap="round" stroke-linejoin="round">
    <path d="M10 30V18a8 8 0 0 1 8-8h12"/><path d="M90 10h12a8 8 0 0 1 8 8v12"/><path d="M110 66v12a8 8 0 0 1-8 8H90"/><path d="M30 86H18a8 8 0 0 1-8-8V66"/>
    <rect x="34" y="28" width="52" height="40" rx="6" stroke-width="4"/><path d="m40 62 12-13 9 9 6-6 13 10" stroke-width="4"/><circle cx="72" cy="40" r="4" stroke-width="4"/></svg>`;
}

function emptyHTML() {
  const d = S.data;
  if (S.q.trim()) return `<div class="empty"><h2>‘${esc(S.q.trim())}’ 와 맞는 스크린샷이 없어요</h2><p>프로그램 이름, 창 제목, 메모로 찾을 수 있어요.</p></div>`;
  if (S.tab === 'kept') return `<div class="empty"><div class="art">${artSvg()}</div><h2>보관함이 비어 있어요</h2><p>오래 두고 싶은 스크린샷에 ★를 누르면 여기로 와요.<br>보관함에 있는 건 자동으로 지워지지 않아요.</p></div>`;
  if (WEB && d.viaPC) {
    return `<div class="empty"><div class="art">${artSvg()}</div><h2>아직 모인 스크린샷이 없어요</h2>
    <p>이 PC에서 캡처하면 여기에 보여요.</p></div>`;
  }
  if (WEB) {
    return `<div class="empty"><div class="art">${artSvg()}</div>
    <h2>드라이브에 아직 스크린샷이 없어요</h2>
    <p>PC에서 Screenmit 을 실행하고 설정 → 다른 PC와 같이 보기에서 이 구글 계정으로 연결하면,<br>찍은 스크린샷이 여기에 보여요.</p>
    <p class="hint"><a href="https://github.com/Choo-AI-Dev/Screenmit/releases/latest" target="_blank" rel="noopener">PC용 Screenmit 받기</a></p></div>`;
  }
  const hk = d.settings.hotkey && d.settings.hotkey !== 'off' ? `<p class="hint">이 화면은 <kbd>${esc(hotkeyText(d.settings.hotkey))}</kbd> 로 언제든 열 수 있어요.</p>` : '';
  const drive = d.drive.on ? '' : `<p class="hint">다른 PC에서도 보려면 <button id="emptyDrive">설정 → 구글 드라이브 연결</button></p>`;
  return `<div class="empty"><div class="art">${artSvg()}</div>
    <h2>아직 모인 스크린샷이 없어요</h2>
    <p>평소처럼 캡처하면 여기에 자동으로 모여요.</p>
    <div class="keys"><kbd>Win</kbd>+<kbd>Shift</kbd>+<kbd>S</kbd><span>또는</span><kbd>Print Screen</kbd></div>
    <p>${d.settings.retentionDays}일 동안 보관하고, 지난 건 알아서 지워요.</p>${hk}${drive}</div>`;
}

function cardHTML(s, showPC) {
  const pc = s.pc || S.data.pc;
  const pcBadge = showPC && pc !== S.data.pc ? `<span class="pc">${esc(pc)}</span>` : '';
  const label = `${clock(s.time)} ${s.app || ''} ${s.title || ''}`;
  return `<div class="card" tabindex="0" role="button" data-id="${s.id}" aria-label="${esc(label)}">
    <div class="thumb"><img loading="lazy" ${B.thumbAttr(s)} alt="" data-drag="${s.id}">
      ${s.pinned ? `<span class="pinned" title="보관함">${icon('starFill')}</span>` : ''}
      <div class="hover">
        <button type="button" data-a="copy" title="복사">${icon('copy')}</button>
        <button type="button" data-a="pin" class="${s.pinned ? 'on' : ''}" title="${s.pinned ? '보관 풀기' : '보관하기 (자동으로 안 지워짐)'}">${icon(s.pinned ? 'starFill' : 'star')}</button>
      </div>
      ${s.remote ? `<span class="remote-mark" title="다른 PC에서 찍은 것 · 원본은 열 때 받아요">${icon('cloud')}</span>` : ''}
    </div>
    <div class="meta"><b>${clock(s.time)}</b><span class="app">${esc(s.app || (s.exe ? s.exe.replace(/\.exe$/i, '') : ''))}</span>${pcBadge}</div>
    ${s.title ? `<div class="title" title="${esc(s.title)}">${esc(s.title)}</div>` : ''}
    ${s.note ? `<div class="note">${icon('note')}<span>${esc(s.note)}</span></div>` : ''}
  </div>`;
}

function renderList() {
  const list = filtered();
  const main = $('#list');
  if (!list.length) { main.innerHTML = emptyHTML(); S.known = new Set(S.data.shots.map((s) => s.id)); return; }
  const showPC = pcsInData().length > 1;
  const groups = [];
  for (const s of list) {
    const g = groups[groups.length - 1];
    if (g && g.day === s.day) g.items.push(s); else groups.push({ day: s.day, items: [s] });
  }
  const html = groups.map((g) => {
    const l = dayLabel(g.day, S.data.today);
    let exp = '';
    if (S.tab !== 'kept') {
      const unpinned = g.items.some((s) => !s.pinned);
      if (unpinned) { const e = expireText(daysLeft(g.day)); exp = `<span class="expire ${e.soon ? 'soon' : ''}" title="★보관한 건 남아요">${e.t}</span>`; }
    }
    return `<section class="day"><header><h2>${esc(l.title)}</h2>${l.sub ? `<span class="date">${l.sub}</span>` : ''}<span class="count">${g.items.length}장</span>${exp}</header>
      <div class="grid">${g.items.map((s) => cardHTML(s, showPC)).join('')}</div></section>`;
  }).join('');
  const y = window.scrollY;
  main.innerHTML = html;
  B.attachThumbs(main);
  window.scrollTo(0, y);
  if (S.known) {
    main.querySelectorAll('.card').forEach((c) => { if (!S.known.has(c.dataset.id)) c.classList.add('new'); });
  }
  S.known = new Set(S.data.shots.map((s) => s.id));
}

// ---------- toast & confirm ----------
let toastTimer = 0;
function toast(msg, action, fn, ms = 3200) {
  const t = $('#toast');
  $('#toastMsg').textContent = msg;
  const b = $('#toastAct');
  if (action) { b.hidden = false; b.textContent = action; b.onclick = () => { hideToast(); fn(); }; } else b.hidden = true;
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(hideToast, ms);
}
function hideToast() { $('#toast').hidden = true; }

function confirmBox(msg, yes = '지우기', danger = true) {
  return new Promise((resolve) => {
    const bg = $('#confirm');
    $('#confirmMsg').textContent = msg;
    const y = $('#confirmYes');
    y.textContent = yes;
    y.className = `btn ${danger ? 'danger' : 'primary'}`;
    bg.hidden = false;
    y.focus();
    const done = (v) => { bg.hidden = true; y.onclick = null; $('#confirmNo').onclick = null; bg.onclick = null; S.confirmDone = null; resolve(v); };
    S.confirmDone = done;
    y.onclick = () => done(true);
    $('#confirmNo').onclick = () => done(false);
    bg.onclick = (e) => { if (e.target === bg) done(false); };
  });
}

// ---------- actions ----------
async function copyShot(id) {
  const s = byId(id);
  let slow = 0;
  if (s && (s.remote || WEB)) slow = setTimeout(() => toast('원본 받는 중…'), 400);
  try {
    await B.copy(id);
    clearTimeout(slow);
    toast('복사했어요 · Ctrl+V 로 붙여넣으면 돼요');
  } catch (e) { clearTimeout(slow); toast(`복사하지 못했어요: ${e.message}`); }
}

async function togglePin(id) {
  const s = byId(id);
  if (!s) return;
  const to = !s.pinned;
  if (!to && daysLeft(s.day) <= 0) {
    if (!await confirmBox(`보관 기간(${S.data.settings.retentionDays}일)이 지난 스크린샷이라\n보관을 풀면 곧 지워져요. 풀까요?`, '보관 풀기')) return;
  }
  s.pinned = to; render();
  try {
    await B.pin(id, to);
    if (to) toast('보관함에 넣었어요 · 자동으로 지워지지 않아요');
    else { const n = daysLeft(s.day); toast(n > 0 ? `보관을 풀었어요 · ${expireText(n).t}` : '보관을 풀었어요'); }
  } catch (e) { s.pinned = !to; render(); toast(`바꾸지 못했어요: ${e.message}`); }
  scheduleLoad(300);
}

async function saveNote(id, note) {
  const s = byId(id);
  if (!s || (s.note || '') === note.trim()) return;
  s.note = note.trim();
  try { await B.note(id, s.note); toast(s.note ? '메모를 저장했어요' : '메모를 지웠어요', null, null, 1600); }
  catch (e) { toast(`메모를 저장하지 못했어요: ${e.message}`); }
  scheduleLoad(300);
}

function commitDelete(keepalive) {
  const p = S.pendingDel;
  if (!p) return;
  clearTimeout(p.timer);
  S.pendingDel = null;
  Promise.resolve(B.del(p.id, keepalive))
    .then(() => scheduleLoad(200)).catch((e) => { toast(`지우지 못했어요: ${e.message}`); scheduleLoad(200); });
}

function deleteShot(id) {
  commitDelete();
  const s = byId(id);
  S.pendingDel = { id, timer: setTimeout(() => { commitDelete(); }, 5000) };
  if (S.viewer) {
    const v = S.viewer;
    v.ids = v.ids.filter((x) => x !== id);
    if (!v.ids.length) closeViewer(); else { v.idx = Math.min(v.idx, v.ids.length - 1); }
  }
  render();
  const other = WEB ? ' (모든 PC에서 지워져요)' : s && s.remote ? ' (다른 PC에서도 지워져요)' : s && s.synced ? ' (드라이브에서도 지워져요)' : '';
  toast(`삭제했어요${other}`, '되돌리기', () => {
    if (!S.pendingDel || S.pendingDel.id !== id) return;
    clearTimeout(S.pendingDel.timer);
    S.pendingDel = null;
    render();
  }, 5000);
}

// ---------- viewer ----------
function openViewer(id) {
  const ids = filtered().map((s) => s.id);
  const idx = ids.indexOf(id);
  if (idx < 0) return;
  S.viewer = { ids, idx, shown: '' };
  $('#viewer').hidden = false;
  document.body.style.overflow = 'hidden';
  refreshViewer();
}
function closeViewer() {
  const box = $('#vNoteBox');
  if (!box.hidden && S.viewer) saveNote(S.viewer.ids[S.viewer.idx], $('#vNote').value);
  S.viewer = null;
  $('#viewer').hidden = true;
  $('#vImg').removeAttribute('src');
  document.body.style.overflow = '';
}
function stepViewer(d) {
  const v = S.viewer;
  if (!v) return;
  const n = v.idx + d;
  if (n < 0 || n >= v.ids.length) return;
  if (!$('#vNoteBox').hidden) saveNote(v.ids[v.idx], $('#vNote').value);
  v.idx = n;
  refreshViewer();
}
function refreshViewer() {
  const v = S.viewer;
  v.ids = v.ids.filter((id) => byId(id) && !(S.pendingDel && S.pendingDel.id === id));
  if (!v.ids.length) { closeViewer(); return; }
  v.idx = Math.min(v.idx, v.ids.length - 1);
  const s = byId(v.ids[v.idx]);
  const changed = v.shown !== s.id;
  v.shown = s.id;
  const img = $('#vImg');
  if (changed) {
    $('#vStage').classList.remove('actual');
    const now = B.thumbNow(s.id);
    if (now) img.src = now; else img.removeAttribute('src');
    const load = $('#vLoading');
    load.textContent = '원본 받는 중…';
    load.hidden = !(s.remote || WEB);
    const mine = s.id;
    B.full(s.id)
      .then((url) => new Promise((res, rej) => { const im = new Image(); im.onload = () => res(url); im.onerror = rej; im.src = url; }))
      .then((url) => { if (S.viewer && S.viewer.shown === mine) { img.src = url; load.hidden = true; } })
      .catch(() => { if (S.viewer && S.viewer.shown === mine) { load.hidden = false; load.textContent = '원본을 가져오지 못했어요 · 인터넷 연결을 확인해 주세요'; } });
    $('#vNote').value = s.note || '';
    $('#vNoteBox').hidden = !s.note;
  }
  $('#vWhen').textContent = fullWhen(s.time, S.data.today);
  const pc = s.pc || S.data.pc;
  $('#vSrc').textContent = [s.app, s.title, pcsInData().length > 1 ? pc : ''].filter(Boolean).join(' · ') || '어디서 찍었는지 기록이 없어요';
  $('#vSrc').title = s.title || '';
  $('#vMeta').textContent = `${s.w} × ${s.h} · ${bytes(s.bytes)}`;
  $('#vPos').textContent = `${v.idx + 1} / ${v.ids.length}`;
  const pin = $('#viewer [data-act=pin]');
  pin.classList.toggle('on', s.pinned);
  pin.querySelector('.i').innerHTML = ICONS[s.pinned ? 'starFill' : 'star'];
  pin.querySelector('.pin-label').textContent = s.pinned ? '보관됨' : '보관';
  pin.title = s.pinned ? '보관 풀기 (P)' : '보관하기 — 자동으로 안 지워짐 (P)';
  const rev = $('#viewer [data-act=reveal]');
  rev.disabled = !!s.remote;
  rev.title = s.remote ? '다른 PC에서 찍은 거라 이 PC 폴더에는 없어요' : '폴더에서 보기';
  const save = $('#viewer [data-act=save]');
  save.href = WEB ? '#' : `/img/${s.id}?download=1`;
  save.setAttribute('download', s.name);
  $('#viewer [data-act=prev]').disabled = v.idx === 0;
  $('#viewer [data-act=next]').disabled = v.idx === v.ids.length - 1;
}

$('#viewer').addEventListener('click', (e) => {
  const b = e.target.closest('[data-act]');
  if (!b || !S.viewer) {
    if (e.target.id === 'vImg') $('#vStage').classList.toggle('actual');
    else if (e.target.id === 'vStage') closeViewer();
    return;
  }
  const id = S.viewer.ids[S.viewer.idx];
  switch (b.dataset.act) {
    case 'close': closeViewer(); break;
    case 'prev': stepViewer(-1); break;
    case 'next': stepViewer(1); break;
    case 'copy': copyShot(id); break;
    case 'pin': togglePin(id); break;
    case 'delete': deleteShot(id); break;
    case 'note': $('#vNoteBox').hidden = false; $('#vNote').focus(); break;
    case 'reveal': if (!b.disabled) api('POST', `/api/shots/${id}/reveal`).catch((er) => toast(er.message)); break;
    case 'save':
      if (WEB) { e.preventDefault(); B.download(byId(id)).catch((er) => toast(`저장하지 못했어요: ${er.message}`)); }
      return; // the PC app's link downloads by itself
  }
});
$('#vNote').addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) { e.preventDefault(); e.target.blur(); }
  if (e.key === 'Escape') { e.stopPropagation(); e.target.value = byId(S.viewer.ids[S.viewer.idx])?.note || ''; e.target.blur(); }
});
$('#vNote').addEventListener('blur', (e) => { if (S.viewer) saveNote(S.viewer.ids[S.viewer.idx], e.target.value); });
$('#vNote').addEventListener('input', (e) => { e.target.style.height = 'auto'; e.target.style.height = `${e.target.scrollHeight}px`; });

// ---------- list events ----------
$('#list').addEventListener('click', (e) => {
  if (e.target.id === 'emptyDrive') { openSettings(true); return; }
  const card = e.target.closest('.card');
  if (!card) return;
  const a = e.target.closest('[data-a]');
  if (a) {
    e.stopPropagation();
    if (a.dataset.a === 'copy') copyShot(card.dataset.id);
    if (a.dataset.a === 'pin') togglePin(card.dataset.id);
    return;
  }
  openViewer(card.dataset.id);
});
$('#list').addEventListener('keydown', (e) => {
  const card = e.target.closest('.card');
  if (!card || e.target !== card || S.viewer) return; // the viewer has its own keys
  if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openViewer(card.dataset.id); }
  if (e.key === 'Delete') deleteShot(card.dataset.id);
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'c') { e.preventDefault(); copyShot(card.dataset.id); }
});
$('#list').addEventListener('dragstart', (e) => {
  const id = e.target.dataset && e.target.dataset.drag;
  if (!id || WEB) return;
  const s = byId(id);
  if (!s) return;
  const url = `${location.origin}/img/${id}`;
  e.dataTransfer.setData('DownloadURL', `image/png:${s.name}:${url}`);
  e.dataTransfer.setData('text/uri-list', url);
});

document.querySelector('.tabs').addEventListener('click', (e) => {
  const t = e.target.closest('.tab');
  if (!t) return;
  S.tab = t.dataset.tab; render(); window.scrollTo(0, 0);
});
$('#pcs').addEventListener('click', (e) => {
  const c = e.target.closest('.chip');
  if (!c) return;
  S.pc = c.dataset.pc; render(); window.scrollTo(0, 0);
});
$('#q').addEventListener('input', (e) => { S.q = e.target.value; renderList(); });
$('#q').addEventListener('keydown', (e) => { if (e.key === 'Escape') { e.target.value = ''; S.q = ''; renderList(); e.target.blur(); } });
$('#pauseBtn').addEventListener('click', async () => {
  const to = !S.data.paused;
  try { await api('POST', '/api/pause', { paused: to }); S.data.paused = to; render(); toast(to ? '잠깐 멈췄어요 · 다시 누르면 이어서 모아요' : '다시 모으고 있어요'); } catch (e) { toast(e.message); }
});
$('#banner').addEventListener('click', (e) => {
  if (e.target.id === 'resumeBtn') $('#pauseBtn').click();
  if (e.target.id === 'noticeBtn' && B.noticeAction) B.noticeAction();
});
$('#folderBtn').addEventListener('click', () => api('POST', '/api/folder/open').catch((e) => toast(e.message)));
$('#settingsBtn').addEventListener('click', () => openSettings());
$('#syncBtn').addEventListener('click', () => {
  if (WEB) { B.refresh(); load().then(() => toast('새로 고쳤어요', null, null, 1400)); return; }
  api('POST', '/api/drive/sync').catch(() => {}); openSettings(true);
});

// ---------- settings ----------
function openSettings(drive) {
  $('#settings').hidden = false;
  renderSettings(true);
  if (drive && !WEB) $('#driveGroup').scrollIntoView({ block: 'start' });
}
function closeSettings() { stopHotkeyListen(); $('#settings').hidden = true; }
$('#settings').addEventListener('click', (e) => {
  if (e.target.id === 'settings' || e.target.closest('[data-close]')) closeSettings();
});

async function setSetting(patch, okMsg) {
  try { await B.settings(patch); if (okMsg) toast(okMsg, null, null, 1800); await load(); return true; }
  catch (e) { toast(e.message, null, null, 4000); await load(); return false; }
}

function renderSettings(full) {
  const d = S.data;
  const st = d.settings;
  $('#ver').textContent = d.version;
  if (d.webLink) $('#webLinkA').href = d.webLink;
  $('#retention').innerHTML = st.retentionChoices.map((n) => `<button type="button" data-days="${n}" class="${n === st.retentionDays ? 'on' : ''}">${n}일</button>`).join('');
  $('#retDesc').textContent = `지난 스크린샷은 자동으로 지워져요. ★보관함에 넣은 건 지우지 않아요.${d.drive.on ? ' 다른 PC에도 같이 적용돼요.' : ''}`;
  $('#skipDocs').checked = st.skipDocs;
  $('#notify').checked = st.notify;
  $('#autostart').checked = st.autostart;
  if (!S.listening) $('#hotkeyBtn').textContent = hotkeyText(st.hotkey);
  $('#hotkeyOff').hidden = st.hotkey === 'off';
  const hm = $('#hotkeyMsg');
  hm.textContent = d.hotkeyError ? d.hotkeyError : '어디서든 누르면 이 화면이 열려요';
  hm.className = d.hotkeyError ? 'err' : '';
  if ($('#folderEdit').hidden) $('#folderPath').textContent = st.folder;
  if (WEB) {
    const inG = !!(B.loggedIn && B.loggedIn());
    $('#webEmail').textContent = inG ? (d.email || (d.drive.status && d.drive.status.email) || '') : '구글 로그인 안 함';
    $('#logoutBtn').textContent = inG ? '로그아웃' : '구글로 로그인';
  } else {
    const g = $('#driveGroup');
    if (full || !g.contains(document.activeElement)) g.innerHTML = driveHTML();
  }
  paintIcons($('#settings'));
}

function driveHTML() {
  const d = S.data;
  const dr = d.drive;
  const st = dr.status || {};
  if (!dr.configured) {
    return `<div class="row col">
      <div class="row-text"><b>구글 드라이브로 회사·집 PC에서 같이 봐요</b>
        <small>내 드라이브의 ‘Screenmit’ 폴더에 올리고, 다른 PC의 Screenmit 에서 같이 보여요. 처음 한 번만 Google Cloud 에서 만든 클라이언트 파일을 넣으면 돼요.</small></div>
      <details><summary>Google Cloud 설정 방법 (5분)</summary><ol class="steps">
        <li>console.cloud.google.com 에서 새 프로젝트 <code>Screenmit</code></li>
        <li>API 및 서비스 → 라이브러리 → <b>Google Drive API</b> 사용</li>
        <li>Google 인증 플랫폼 → 시작하기: 앱 이름 <code>Screenmit</code>, 대상 <b>외부</b></li>
        <li>데이터 액세스 → 범위 추가 → <code>…/auth/drive.file</code></li>
        <li>대상 → <b>앱 게시</b> (테스트 상태면 7일마다 로그인이 풀려요)</li>
        <li>클라이언트 → 클라이언트 만들기 → 유형 <b>데스크톱 앱</b> → JSON 다운로드</li>
      </ol></details>
      <div class="drop" id="drop"><b>client_secret_….json</b> 파일을 여기로 끌어다 놓거나 <button type="button" class="link" id="pickJson" style="color:var(--accent)">파일 선택</button>
        <input type="file" id="jsonFile" accept=".json,application/json" hidden></div>
      <small class="err" id="credErr"></small></div>`;
  }
  const pcName = esc(d.settings.pcName);
  if (!dr.on || (!st.connected && dr.connecting)) {
    return `<div class="row col">
        <div class="row-text"><b>이 PC를 뭐라고 부를까요?</b><small>다른 PC에서 볼 때 이 이름으로 구분돼요</small></div>
        <div class="pcname"><input type="text" id="pcNameInput" maxlength="30" value="${pcName}">
          <button type="button" class="chip" data-pcname="회사 PC">회사 PC</button><button type="button" class="chip" data-pcname="집 PC">집 PC</button></div>
      </div>
      <div class="row"><div class="row-text"><b>${dr.connecting ? '브라우저에서 구글 로그인을 마쳐 주세요…' : '구글 계정으로 연결'}</b>
        <small>${dr.connecting ? '로그인 창이 안 보이면 작업 표시줄의 브라우저를 확인해 주세요.' : '브라우저에 구글 로그인이 열려요. Screenmit 이 만든 파일만 볼 수 있는 권한이에요.'}</small></div>
        <button type="button" class="btn primary" id="connectBtn" ${dr.connecting ? 'disabled' : ''}>${dr.connecting ? '기다리는 중' : '연결'}</button></div>
      ${dr.loginError ? `<div class="row"><small class="err">${esc(dr.loginError)}</small></div>` : ''}`;
  }
  let dot = 'ok'; let line = '';
  if (st.needLogin) { dot = 'err'; line = '로그인이 풀렸어요'; }
  else if (st.error) { dot = 'err'; line = st.error; }
  else if (st.syncing || !st.connected) { dot = 'busy'; line = '맞추는 중…'; }
  else { line = `${ago(st.lastSync)} 맞춤${st.pending ? ` · 올릴 것 ${st.pending}장` : ''}`; }
  const others = (st.pcs || []).filter((p) => p !== d.settings.pcName);
  return `<div class="row">
      <div class="row-text"><span class="status-line"><span class="dot ${dot}"></span><b>${esc(st.email || '구글 드라이브')}</b></span>
        <small>${esc(line)}${others.length ? ` · 같이 보는 PC: ${others.map(esc).join(', ')}` : ''}</small></div>
      ${st.needLogin ? '<button type="button" class="btn primary" id="connectBtn">다시 연결</button>' : '<button type="button" class="btn" id="syncNow">지금 맞추기</button>'}
    </div>
    <div class="row"><div class="row-text"><b>이 PC 이름</b><small>다른 PC에서 이 이름으로 보여요</small></div>
      <div class="pcname"><input type="text" id="pcNameInput" maxlength="30" value="${pcName}"></div></div>
    <div class="row"><div class="row-text"><b>휴대폰·앱 없는 PC에서 보기</b><small>같은 구글 계정으로 로그인하면 웹에서 바로 보여요</small></div>
      <div class="links"><a href="${esc(d.webLink || WEB_URL)}" target="_blank" rel="noopener">웹에서 열기</a></div></div>
    <div class="row"><div class="row-text"><small>내 드라이브 ‘Screenmit’ 폴더에 저장돼요. 연결을 끊어도 이 PC의 파일은 그대로예요.</small></div>
      <button type="button" class="link" id="disconnectBtn">연결 끊기</button></div>`;
}

async function sendCredentials(file) {
  const err = $('#credErr');
  if (err) err.textContent = '';
  try {
    const text = await file.text();
    await api('POST', '/api/drive/credentials', text, true);
    toast('클라이언트를 넣었어요 · 이제 구글 계정으로 연결하면 돼요');
    await load();
    renderSettings(true);
  } catch (e) { if ($('#credErr')) $('#credErr').textContent = e.message; }
}

async function connectDrive() {
  const inp = $('#pcNameInput');
  if (inp && inp.value.trim() !== S.data.settings.pcName) await setSetting({ pcName: inp.value.trim() });
  try { await api('POST', '/api/drive/connect'); await load(); renderSettings(true); } catch (e) { toast(e.message, null, null, 4000); }
}

$('#settings').addEventListener('click', async (e) => {
  const t = e.target;
  const days = t.closest('[data-days]');
  if (days) {
    const n = Number(days.dataset.days);
    const cur = S.data.settings.retentionDays;
    if (n < cur) {
      const gone = S.data.shots.filter((s) => !s.pinned && S.data.settings.retentionDays - dayDiff(S.data.today, s.day) + (n - cur) <= 0).length;
      if (gone && !await confirmBox(`${n}일로 줄이면 ${gone}장이 바로 지워져요.${S.data.drive.on ? '\n(다른 PC와 드라이브에서도 지워져요)' : ''}\n★보관한 건 남아요.`, `${n}일로 바꾸기`)) return;
    }
    setSetting({ retentionDays: n }, `${n}일 동안 보관해요`);
    return;
  }
  if (t.id === 'pickJson') $('#jsonFile').click();
  if (t.closest('[data-pcname]')) { $('#pcNameInput').value = t.closest('[data-pcname]').dataset.pcname; }
  if (t.id === 'connectBtn') connectDrive();
  if (t.id === 'syncNow') { api('POST', '/api/drive/sync').catch(() => {}); toast('드라이브와 맞추는 중이에요', null, null, 1500); }
  if (t.id === 'disconnectBtn') {
    if (await confirmBox('연결을 끊으면 다른 PC의 스크린샷은 이 목록에서 빠져요.\n이 PC의 파일과 드라이브의 파일은 그대로예요.', '연결 끊기')) {
      await api('POST', '/api/drive/disconnect').catch((er) => toast(er.message));
      await load(); renderSettings(true);
    }
  }
  if (t.id === 'logoutBtn' && B.logout) { if (B.loggedIn()) B.logout(); else B.login(); }
  if (t.id === 'folderOpen') api('POST', '/api/folder/open').catch((er) => toast(er.message));
  if (t.id === 'folderChange') {
    $('#folderEdit').hidden = false; $('#folderButtons').hidden = true; $('#folderPath').hidden = true;
    $('#folderInput').value = S.data.settings.folder; $('#folderInput').focus(); $('#folderInput').select();
  }
  if (t.id === 'folderCancel') endFolderEdit();
  if (t.id === 'folderApply') applyFolder();
  if (t.id === 'hotkeyBtn') startHotkeyListen();
  if (t.id === 'hotkeyOff') setSetting({ hotkey: 'off' }, '단축키를 껐어요');
});
$('#settings').addEventListener('change', (e) => {
  const t = e.target;
  if (t.id === 'skipDocs') setSetting({ skipDocs: t.checked });
  if (t.id === 'notify') setSetting({ notify: t.checked });
  if (t.id === 'autostart') setSetting({ autostart: t.checked }, t.checked ? '컴퓨터를 켜면 자동으로 실행돼요' : '자동 실행을 껐어요');
  if (t.id === 'jsonFile' && t.files[0]) sendCredentials(t.files[0]);
  if (t.id === 'pcNameInput' && S.data.drive.on) setSetting({ pcName: t.value.trim() }, 'PC 이름을 바꿨어요');
});
$('#settings').addEventListener('keydown', (e) => {
  if (e.target.id === 'pcNameInput' && e.key === 'Enter') e.target.blur();
  if (e.target.id === 'folderInput' && e.key === 'Enter') applyFolder();
  if (e.target.id === 'folderInput' && e.key === 'Escape') { e.stopPropagation(); endFolderEdit(); }
});
for (const ev of ['dragenter', 'dragover']) {
  $('#settings').addEventListener(ev, (e) => { const d = e.target.closest && e.target.closest('#drop'); if (d) { e.preventDefault(); d.classList.add('over'); } });
}
$('#settings').addEventListener('dragleave', (e) => { const d = e.target.closest && e.target.closest('#drop'); if (d) d.classList.remove('over'); });
$('#settings').addEventListener('drop', (e) => {
  const d = e.target.closest && e.target.closest('#drop');
  if (!d) return;
  e.preventDefault(); d.classList.remove('over');
  const f = e.dataTransfer.files[0];
  if (f) sendCredentials(f);
});

function endFolderEdit() {
  $('#folderEdit').hidden = true; $('#folderButtons').hidden = false; $('#folderPath').hidden = false; $('#folderErr').textContent = '';
}
async function applyFolder() {
  const v = $('#folderInput').value.trim();
  try { await api('POST', '/api/settings', { folder: v }); endFolderEdit(); toast('이제 이 폴더에 저장해요'); await load(); }
  catch (e) { $('#folderErr').textContent = e.message; }
}

function startHotkeyListen() {
  S.listening = true;
  const b = $('#hotkeyBtn');
  b.classList.add('listening');
  b.textContent = '새 단축키를 누르세요…';
}
function stopHotkeyListen() {
  if (!S.listening) return;
  S.listening = false;
  $('#hotkeyBtn').classList.remove('listening');
  $('#hotkeyBtn').textContent = hotkeyText(S.data.settings.hotkey);
}
function keySpec(e) {
  let k = '';
  if (/^Key[A-Z]$/.test(e.code)) k = e.code.slice(3);
  else if (/^Digit\d$/.test(e.code)) k = e.code.slice(5);
  else if (/^F([1-9]|1[0-2])$/.test(e.code)) k = e.code;
  if (!k) return '';
  const mods = [];
  if (e.ctrlKey) mods.push('Ctrl');
  if (e.altKey) mods.push('Alt');
  if (e.shiftKey) mods.push('Shift');
  if (e.metaKey) mods.push('Win');
  return [...mods, k].join('+');
}

// ---------- keyboard ----------
document.addEventListener('keydown', (e) => {
  if (S.listening) {
    e.preventDefault();
    if (e.key === 'Escape') { stopHotkeyListen(); return; }
    const spec = keySpec(e);
    if (!spec) return;
    if (!e.ctrlKey && !e.altKey && !e.metaKey && !spec.startsWith('F') && !spec.includes('+F')) {
      $('#hotkeyBtn').textContent = 'Ctrl 이나 Alt 와 같이 눌러 주세요'; return;
    }
    stopHotkeyListen();
    setSetting({ hotkey: spec }, `단축키: ${hotkeyText(spec)}`);
    return;
  }
  if (!$('#confirm').hidden) {
    if (e.key === 'Escape' && S.confirmDone) S.confirmDone(false);
    return;
  }
  const typing = e.target.matches && e.target.matches('input, textarea');
  if (!$('#settings').hidden) {
    if (e.key === 'Escape' && !typing) closeSettings();
    return;
  }
  if (S.viewer) {
    if (typing) return;
    const id = S.viewer.ids[S.viewer.idx];
    if (e.key === 'Escape') closeViewer();
    else if (e.key === 'ArrowLeft') stepViewer(-1);
    else if (e.key === 'ArrowRight') stepViewer(1);
    else if (e.key === 'Delete') deleteShot(id);
    else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'c') { e.preventDefault(); copyShot(id); }
    else if (e.key.toLowerCase() === 'p' && !e.ctrlKey) togglePin(id);
    else if (e.key.toLowerCase() === 'm' && !e.ctrlKey) { e.preventDefault(); $('#vNoteBox').hidden = false; $('#vNote').focus(); }
    return;
  }
  if ((e.ctrlKey && e.key.toLowerCase() === 'f') || (e.key === '/' && !typing)) { e.preventDefault(); $('#q').focus(); $('#q').select(); }
});

window.addEventListener('beforeunload', () => commitDelete(true));

// ---------- live updates ----------
// Swipe left/right in the viewer (phones and tablets).
(() => {
  let x0 = null; let y0 = 0;
  const stage = $('#vStage');
  stage.addEventListener('touchstart', (e) => { if (e.touches.length === 1 && !stage.classList.contains('actual')) { x0 = e.touches[0].clientX; y0 = e.touches[0].clientY; } }, { passive: true });
  stage.addEventListener('touchend', (e) => {
    if (x0 === null) return;
    const dx = e.changedTouches[0].clientX - x0; const dy = e.changedTouches[0].clientY - y0;
    x0 = null;
    if (Math.abs(dx) > 60 && Math.abs(dy) < 80) stepViewer(dx < 0 ? 1 : -1);
  });
})();
setInterval(() => { if (S.data && !document.hidden) renderTop(); }, 30000); // keep "n분 전" fresh

paintIcons();
load().then(() => B.live(() => scheduleLoad()));
