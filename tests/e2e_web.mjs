// The web page and the PC app against one fake Google Drive: what one side
// changes, the other side sees.   node tests/e2e_web.mjs /tmp/screenmit-dev OUT_DIR
import { spawn, execFileSync } from 'node:child_process';
import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';
import assert from 'node:assert/strict';

// `npm i playwright` (falls back to a preinstalled copy)
const { chromium, devices } = await import('playwright').catch(() => import('/opt/npm-tools/node_modules/playwright/index.mjs'));

const bin = process.argv[2];
const out = process.argv[3] || '/tmp/sm-web';
const samples = '/tmp/samples';
const repo = path.dirname(path.dirname(new URL(import.meta.url).pathname));
fs.mkdirSync(out, { recursive: true });
const cfg = fs.mkdtempSync('/tmp/smcfg-');
const folder = fs.mkdtempSync('/tmp/smshots-');

// ---- the web page, served like GitHub Pages (built once the fake Drive is up) ----
const siteDir = path.join(out, 'site');
const siteNoId = path.join(out, 'site-noid');
function serve() {
  return new Promise((resolve) => {
    const types = { '.html': 'text/html; charset=utf-8', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.png': 'image/png', '.webmanifest': 'application/manifest+json' };
    const srv = http.createServer((req, res) => {
      let p = decodeURIComponent(new URL(req.url, 'http://x').pathname);
      let dir = siteDir;
      if (p.startsWith('/noid/')) { dir = siteNoId; p = p.slice(5); } else p = p.replace(/^\/Screenmit/, '');
      if (p.endsWith('/')) p += 'index.html';
      const f = path.join(dir, p);
      if (!f.startsWith(dir) || !fs.existsSync(f)) { res.writeHead(404); res.end('nf'); return; }
      res.writeHead(200, { 'Content-Type': types[path.extname(f)] || 'application/octet-stream' });
      res.end(fs.readFileSync(f));
    });
    srv.listen(0, '127.0.0.1', () => resolve(srv));
  });
}
const s1 = await serve();
const webOrigin = `http://127.0.0.1:${s1.address().port}`;
const site = `${webOrigin}/Screenmit/`;
const siteNoIdURL = `${webOrigin}/noid/`;

// ---- the PC app (dev build) with a fake Drive; it lets the web page in ----
const proc = spawn(bin, ['--config', cfg, '--folder', folder, '--port', '28200', '--dev-fake-drive', '--dev-pc', 'DESKTOP-8SL9GQP'],
  { stdio: ['ignore', 'pipe', 'pipe'], env: { ...process.env, SCREENMIT_WEB_URL: site } });
let logs = '';
proc.stderr.on('data', (d) => { logs += d; });
const { gallery, fake } = await new Promise((resolve, reject) => {
  let buf = '';
  proc.stdout.on('data', (d) => {
    buf += d;
    const g = buf.match(/GALLERY (\S+)/); const f = buf.match(/FAKEDRIVE (\S+)/);
    if (g && f) resolve({ gallery: g[1], fake: f[1] });
  });
  setTimeout(() => reject(new Error('no start: ' + logs)), 10000);
});
const pcOrigin = new URL(gallery).origin;

const browser = await chromium.launch();
const pcCtx = await browser.newContext({ viewport: { width: 1280, height: 800 }, locale: 'ko-KR', timezoneId: 'Asia/Seoul' });
const pc = await pcCtx.newPage();
await pc.goto(gallery);
await pc.waitForSelector('.empty h2');
const H = { 'X-Screenmit': '1' };
const pcState = async () => (await pc.request.get(`${pcOrigin}/api/state`)).json();
const dayAgo = (n, hh, mm) => { const d = new Date(); d.setDate(d.getDate() - n); d.setHours(hh, mm, 7, 0); return d.toISOString(); };
async function capture(file, q) {
  const r = await pc.request.post(`${pcOrigin}/api/dev/capture?${new URLSearchParams(q)}`, { headers: H, data: fs.readFileSync(path.join(samples, file)) });
  assert.equal(r.status(), 200);
}
await capture('ppt.png', { app: 'Microsoft PowerPoint', exe: 'POWERPNT.EXE', title: 'CV04 정리.pptx - PowerPoint', time: dayAgo(0, 14, 3) });
await capture('excel.png', { app: 'Microsoft Excel', exe: 'EXCEL.EXE', title: '제주 여행 경비.xlsx - Excel', time: dayAgo(0, 11, 42) });
await capture('photo.png', { app: '그림판', exe: 'mspaint.exe', title: 'IMG_2041.jpg - 그림판', time: dayAgo(1, 9, 15) });
await capture('code.png', { app: 'Visual Studio Code', exe: 'Code.exe', title: 'sync.go - screenmit', time: dayAgo(10, 22, 40) });

// connect the PC app to Drive through its own settings, as a user would
const fakeClient = await (await pc.request.get(`${pcOrigin}/api/dev/fake-client`)).text();
const clientFile = path.join(out, 'client.json');
fs.writeFileSync(clientFile, fakeClient);
await pc.click('#settingsBtn');
await pc.setInputFiles('#jsonFile', clientFile);
await pc.waitForSelector('#pcNameInput');
await pc.click('[data-pcname="회사 PC"]');
await pc.click('#connectBtn');
await pc.waitForFunction(() => /맞춤/.test(document.querySelector('#driveGroup').textContent), null, { timeout: 15000 });
await pc.click('#settings [data-close]');
const home = await pc.request.post(`${pcOrigin}/api/dev/remote?${new URLSearchParams({ pc: '집 PC', app: 'KakaoTalk', title: '민수', time: dayAgo(0, 8, 5) })}`,
  { headers: H, data: fs.readFileSync(path.join(samples, 'kakao.png')) });
assert.equal(home.status(), 200);
let drv = await (await pc.request.get(`${pcOrigin}/api/dev/drive`)).json();
assert.equal(drv.shots.length, 5);

execFileSync('python3', [path.join(repo, 'tools/build_site.py'), siteDir, '--client-id', 'web-test.apps.googleusercontent.com', '--api-base', fake, '--auth-url', `${fake}/o/oauth2/auth`]);
execFileSync('python3', [path.join(repo, 'tools/build_site.py'), siteNoId, '--api-base', fake, '--auth-url', `${fake}/o/oauth2/auth`]);

const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'ko-KR', timezoneId: 'Asia/Seoul', acceptDownloads: true });
await ctx.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: webOrigin });
const web = await ctx.newPage();
const errors = [];
web.on('pageerror', (e) => errors.push(e.message));
web.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()); });

await web.goto(site);
await web.waitForSelector('#loginBtn');
assert.match(await web.textContent('.gate'), /웹에서 열기/); // how to see this PC without Google
await web.screenshot({ path: `${out}/w0-login.png` });

// ---- no Google: the PC app's "웹에서 열기" link shows this PC's screenshots ----
await pcWaits((st) => st.shots.length === 5, 'the home PC shot');
const link = (await pcState()).webLink;
assert.ok(link.startsWith(`${site}#local=${new URL(gallery).port}.`), link);
await web.goto(link);
try {
  await web.waitForFunction(() => document.querySelectorAll('.card').length === 5, null, { timeout: 10000 });
} catch (e) {
  await web.screenshot({ path: `${out}/fail.png` });
  console.log('URL', web.url(), '\nERRORS', errors, '\nBODY', (await web.textContent('body')).replace(/\s+/g, ' ').slice(0, 500));
  throw e;
}
assert.ok(!(await web.evaluate(() => location.hash)), 'key removed from the address bar');
assert.match(await web.textContent('#banner'), /이 PC의 스크린샷만 보여요/);
assert.equal(await web.textContent('#noticeBtn'), '구글로 로그인');
assert.match(await web.textContent('#summary'), /이 PC/);
await web.waitForFunction(() => [...document.querySelectorAll('.card img')].every((i) => i.src.startsWith('blob:')), null, { timeout: 10000 });
await web.waitForTimeout(800);
await web.screenshot({ path: `${out}/w0b-this-pc.png` });
const localPhoto = await web.getAttribute('.card:has-text("그림판")', 'data-id');
await web.click(`.card[data-id="${localPhoto}"]`);
await web.waitForFunction(() => document.querySelector('#vImg').src.startsWith('blob:') && document.querySelector('#vLoading').hidden, null, { timeout: 10000 });
assert.equal(await web.evaluate(() => document.querySelector('#vImg').naturalWidth), 1280);
const copiedBefore = (await (await pc.request.get(`${pcOrigin}/api/dev/platform`)).json()).copied;
await web.keyboard.press('Control+c');
await web.waitForFunction(() => /복사했어요/.test(document.querySelector('#toastMsg').textContent));
assert.equal((await (await pc.request.get(`${pcOrigin}/api/dev/platform`)).json()).copied, copiedBefore + 1, 'copied on the PC');
await web.keyboard.press('p');
await web.waitForFunction(() => document.querySelector('#viewer [data-act=pin]').classList.contains('on'));
assert.ok((await pcState()).shots.find((x) => x.id === localPhoto).pinned, 'pinned on the PC');
await web.keyboard.press('p');
await web.waitForFunction(() => !document.querySelector('#viewer [data-act=pin]').classList.contains('on'));
await web.keyboard.press('Escape');
// from here, sign in with Google: the banner's button
await web.click('#noticeBtn');
try {
  await web.waitForFunction(() => document.querySelectorAll('.card').length === 5, null, { timeout: 10000 }); // 4 from this PC + 1 from home; the 10-day-old one is still within 14
} catch (e) {
  await web.screenshot({ path: `${out}/fail.png` });
  console.log('URL', web.url(), '\nERRORS', errors, '\nBODY', (await web.textContent('body')).replace(/\s+/g, ' ').slice(0, 600));
  throw e;
}
assert.ok(!(await web.evaluate(() => location.hash)), 'token removed from the address bar');
assert.ok(await web.evaluate(() => document.body.classList.contains('web')));
assert.ok(!(await web.isVisible('#pauseBtn')) && !(await web.isVisible('#folderBtn')));
// thumbnails come in as blobs
await web.waitForFunction(() => [...document.querySelectorAll('.card img')].every((i) => i.src.startsWith('blob:')), null, { timeout: 10000 });
const chips = await web.$$eval('#pcs .chip', (c) => c.map((x) => x.textContent));
assert.equal(chips[0], '모든 PC');
assert.deepEqual(chips.slice(1).sort(), ['집 PC', '회사 PC']);
assert.equal(await web.textContent('.card:has-text("KakaoTalk") .pc'), '집 PC');
const heads = await web.$$eval('.day header h2', (h) => h.map((x) => x.textContent));
assert.deepEqual(heads.slice(0, 2), ['오늘', '어제']);
await web.waitForTimeout(1000);
await web.screenshot({ path: `${out}/w1-gallery.png` });

// search works the same
await web.fill('#q', 'CV04');
await web.waitForFunction(() => document.querySelectorAll('.card').length === 1);
await web.press('#q', 'Escape');

// open the PowerPoint shot: full picture from Drive
const pptId = await web.getAttribute('.card:has-text("PowerPoint")', 'data-id');
await web.click(`.card[data-id="${pptId}"]`);
await web.waitForFunction(() => document.querySelector('#vImg').src.startsWith('blob:') && document.querySelector('#vLoading').hidden, null, { timeout: 10000 });
const natural = await web.evaluate(() => document.querySelector('#vImg').naturalWidth);
assert.equal(natural, 1600, 'full size picture, not the thumbnail');
assert.ok(!(await web.isVisible('#viewer [data-act=reveal]')));
// copy to the clipboard
await web.keyboard.press('Control+c');
await web.waitForFunction(() => /복사했어요|복사하지/.test(document.querySelector('#toastMsg').textContent), null, { timeout: 10000 });
assert.match(await web.textContent('#toastMsg'), /복사했어요/);
const clipTypes = await web.evaluate(async () => (await navigator.clipboard.read())[0].types);
assert.ok(clipTypes.includes('image/png'), clipTypes.join());
// save
const [dl] = await Promise.all([web.waitForEvent('download'), web.click('#viewer [data-act=save]')]);
assert.match(dl.suggestedFilename(), /^Screenmit_.*\.png$/);
// keep + memo from the web
await web.keyboard.press('p');
await web.waitForFunction(() => document.querySelector('#viewer [data-act=pin]').classList.contains('on'));
await web.keyboard.press('m');
await web.fill('#vNote', '웹에서 쓴 메모');
await web.keyboard.press('Enter');
await web.waitForFunction(() => /메모를 저장했어요/.test(document.querySelector('#toastMsg').textContent));
await web.screenshot({ path: `${out}/w2-viewer.png` });
await web.keyboard.press('Escape');

// ...the PC app picks both up, and moves its file to 보관함
async function pcWaits(fn, what) {
  for (let i = 0; i < 40; i++) {
    await pc.request.post(`${pcOrigin}/api/drive/sync`, { headers: H });
    await new Promise((r) => setTimeout(r, 700));
    const st = await pcState();
    if (fn(st)) return st;
  }
  throw new Error(`PC app never saw: ${what}`);
}
await pcWaits((st) => { const s = st.shots.find((x) => x.id === pptId); return s && s.pinned && s.note === '웹에서 쓴 메모'; }, 'pin + memo');
await new Promise((r) => setTimeout(r, 300));
assert.equal(fs.readdirSync(path.join(folder, '보관함')).length, 1, 'PC file moved to 보관함');

// delete on the web -> gone on the PC (file too)
const excelId = await web.getAttribute('.card:has-text("Excel")', 'data-id');
const before = (await pcState()).shots.find((x) => x.id === excelId);
assert.ok(before);
await web.locator(`.card[data-id="${excelId}"]`).focus();
await web.keyboard.press('Delete');
await web.waitForFunction(() => /모든 PC에서 지워져요/.test(document.querySelector('#toastMsg').textContent));
await web.waitForTimeout(5800);
drv = await (await pc.request.get(`${pcOrigin}/api/dev/drive`)).json();
assert.ok(!drv.shots.some((f) => f.appProperties.id === excelId), 'deleted on Drive');
await pcWaits((st) => !st.shots.some((x) => x.id === excelId), 'delete');
const day0 = fs.readdirSync(folder).filter((d) => /^\d{4}-/.test(d));
const left = day0.flatMap((d) => fs.readdirSync(path.join(folder, d)));
assert.ok(!left.some((f) => f.includes('_11-42-')), `excel file still on disk: ${left}`);

// a change on the PC shows on the web after a refresh
const photoId = (await pcState()).shots.find((x) => x.app === '그림판').id;
await pc.request.fetch(`${pcOrigin}/api/shots/${photoId}`, { method: 'PATCH', headers: { ...H, 'Content-Type': 'application/json' }, data: { note: 'PC에서 쓴 메모' } });
await pcWaits(() => true, 'push');
await pc.request.post(`${pcOrigin}/api/drive/sync`, { headers: H });
await new Promise((r) => setTimeout(r, 2500));
await web.click('#syncBtn');
await web.waitForFunction((id) => /PC에서 쓴 메모/.test(document.querySelector(`.card[data-id="${id}"]`)?.textContent || ''), photoId, { timeout: 10000 });

// retention 7 from the web: the 10-day-old shot leaves both sides
await web.click('#settingsBtn');
await web.waitForSelector('#settings:not([hidden])');
assert.equal(await web.textContent('#webEmail'), 'choo@example.com');
assert.ok(!(await web.isVisible('#driveGroup')) && !(await web.isVisible('#autostart')));
await web.screenshot({ path: `${out}/w3-settings.png` });
await web.click('#retention button[data-days="7"]');
await web.waitForSelector('#confirm:not([hidden])');
await web.click('#confirmYes');
await web.waitForFunction(() => document.querySelector('#retention .on')?.textContent === '7일');
await web.click('#settings [data-close]');
await web.waitForFunction(() => !document.querySelector('.card .app') || ![...document.querySelectorAll('.card .app')].some((a) => a.textContent === 'Visual Studio Code'));
await pcWaits((st) => st.settings.retentionDays === 7 && !st.shots.some((x) => x.app === 'Visual Studio Code'), 'retention 7');

// phone
const phoneCtx = await browser.newContext({ ...devices['iPhone 13'], locale: 'ko-KR', timezoneId: 'Asia/Seoul' });
const phone = await phoneCtx.newPage();
phone.on('pageerror', (e) => errors.push('phone: ' + e.message));
await phone.goto(site);
await phone.click('#loginBtn');
await phone.waitForFunction(() => document.querySelectorAll('.card').length === 3, null, { timeout: 10000 });
await phone.waitForFunction(() => [...document.querySelectorAll('.card img')].every((i) => i.src.startsWith('blob:')), null, { timeout: 10000 });
await phone.screenshot({ path: `${out}/w4-phone.png` });
await phone.tap('.card >> nth=0');
await phone.waitForFunction(() => document.querySelector('#vImg').src.startsWith('blob:') && document.querySelector('#vLoading').hidden, null, { timeout: 10000 });
const pos0 = await phone.textContent('#vPos');
await phone.evaluate(() => { // swipe left
  const st = document.querySelector('#vStage');
  const t = (x) => new Touch({ identifier: 1, target: st, clientX: x, clientY: 300 });
  st.dispatchEvent(new TouchEvent('touchstart', { touches: [t(300)], changedTouches: [t(300)], bubbles: true }));
  st.dispatchEvent(new TouchEvent('touchend', { touches: [], changedTouches: [t(120)], bubbles: true }));
});
await phone.waitForFunction((p) => document.querySelector('#vPos').textContent !== p, pos0);
assert.equal(await phone.textContent('#vPos'), '2 / 3');
await phone.waitForTimeout(600);
await phone.screenshot({ path: `${out}/w5-phone-viewer.png` });

// coming back after the hour-long login ran out: the sign-in screen, one click back in
await web.evaluate(() => { const k = 'screenmit.web.token'; const t = JSON.parse(localStorage.getItem(k)); t.exp = Date.now() - 1; localStorage.setItem(k, JSON.stringify(t)); });
await web.reload();
await web.waitForFunction(() => /로그인 시간이 지났어요/.test(document.querySelector('#banner').textContent), null, { timeout: 10000 });
assert.ok(await web.isVisible('#noticeBtn'));
await web.click('#noticeBtn');
await web.waitForFunction(() => document.querySelectorAll('.card').length === 3 && document.querySelector('#banner').hidden, null, { timeout: 10000 });
// log out: back to this PC only
await web.click('#settingsBtn');
assert.equal(await web.textContent('#logoutBtn'), '로그아웃');
await web.click('#logoutBtn');
await web.waitForFunction(() => /이 PC의 스크린샷만/.test(document.querySelector('#banner').textContent), null, { timeout: 10000 });
await web.click('#settingsBtn');
assert.equal(await web.textContent('#webEmail'), '구글 로그인 안 함');
assert.equal(await web.textContent('#logoutBtn'), '구글로 로그인');
await web.click('#settings [data-close]');

// no client id built in: the page asks for one
const p2 = await ctx.newPage();
await p2.goto(siteNoIdURL);
await p2.waitForFunction(() => /구글 로그인 없이 이 PC의 스크린샷만 보여요/.test(document.querySelector('#banner').textContent), null, { timeout: 10000 });
assert.ok(await p2.isHidden('#noticeBtn'));
await p2.evaluate(() => localStorage.removeItem('screenmit.web.local'));
await p2.reload();
await p2.waitForSelector('#clientIdInput');
await p2.screenshot({ path: `${out}/w6-setup.png` });
await p2.fill('#clientIdInput', 'not an id');
await p2.click('#clientIdSave');
assert.ok(await p2.isVisible('#clientIdInput'));
await p2.fill('#clientIdInput', '1234-abc.apps.googleusercontent.com');
await p2.click('#clientIdSave');
await p2.waitForSelector('#loginBtn');
await p2.click('#loginBtn');
await p2.waitForFunction(() => document.querySelectorAll('.card').length === 3, null, { timeout: 10000 });

// the PC app is off: the sign-in screen says so
await web.goto(link);
await web.waitForSelector('.card');
proc.kill('SIGINT');
await new Promise((r) => setTimeout(r, 800));
await web.evaluate(() => localStorage.removeItem('screenmit.web.token'));
await web.reload();
await web.waitForSelector('.gate');
assert.match(await web.textContent('.gate'), /이 PC의 Screenmit 에 연결하지 못했어요/);
await web.screenshot({ path: `${out}/w7-pc-off.png` });

assert.deepEqual(errors.filter((e) => !/Failed to load resource|ERR_CONNECTION_REFUSED/.test(e)), [], 'console errors');
await browser.close();
s1.close();
console.log('WEB E2E OK');
