// End-to-end test of the gallery against the dev build (fake clipboard
// feed + fake Google Drive).  node tests/e2e.mjs /tmp/screenmit-dev OUT_DIR
import { spawn } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';

// `npm i playwright` (falls back to a preinstalled copy)
const { chromium } = await import('playwright').catch(() => import('/opt/npm-tools/node_modules/playwright/index.mjs'));

const bin = process.argv[2];
const out = process.argv[3] || '/tmp/sm-e2e';
const samples = '/tmp/samples';
const cfg = fs.mkdtempSync('/tmp/smcfg-');
const folder = fs.mkdtempSync('/tmp/smshots-');
fs.mkdirSync(out, { recursive: true });

const proc = spawn(bin, ['--config', cfg, '--folder', folder, '--port', '28100', '--dev-fake-drive', '--dev-pc', 'DESKTOP-8SL9GQP'], { stdio: ['ignore', 'pipe', 'pipe'] });
let logs = '';
proc.stderr.on('data', (d) => { logs += d; });
const gallery = await new Promise((resolve, reject) => {
  let buf = '';
  proc.stdout.on('data', (d) => { buf += d; const m = buf.match(/GALLERY (\S+)/); if (m) resolve(m[1]); });
  setTimeout(() => reject(new Error('no start: ' + logs)), 10000);
});
const origin = new URL(gallery).origin;

const browser = await chromium.launch();
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'ko-KR', timezoneId: 'Asia/Seoul', acceptDownloads: true });
const page = await ctx.newPage();
const errors = [];
page.on('pageerror', (e) => errors.push(e.message));
page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()); });
await page.goto(gallery);
await page.waitForSelector('.empty h2');
assert.match(await page.textContent('.empty h2'), /아직 모인 스크린샷이 없어요/);
await page.screenshot({ path: `${out}/0-empty.png` });

const H = { 'X-Screenmit': '1' };
async function capture(file, q) {
  const r = await page.request.post(`${origin}/api/dev/capture?${new URLSearchParams(q)}`, { headers: H, data: fs.readFileSync(path.join(samples, file)) });
  assert.equal(r.status(), 200, await r.text());
}
const dayAgo = (n, hh, mm) => { const d = new Date(); d.setDate(d.getDate() - n); d.setHours(hh, mm, 12, 0); return d.toISOString(); };

await capture('ppt.png', { app: 'Microsoft PowerPoint', exe: 'POWERPNT.EXE', title: 'CV04 정리.pptx - PowerPoint', time: dayAgo(0, 14, 3) });
await capture('excel.png', { app: 'Microsoft Excel', exe: 'EXCEL.EXE', title: '제주 여행 경비.xlsx - Excel', time: dayAgo(0, 11, 42) });
await capture('photo.png', { app: '그림판', exe: 'mspaint.exe', title: 'IMG_2041.jpg - 그림판', time: dayAgo(0, 9, 15) });
await capture('kakao.png', { app: 'KakaoTalk', exe: 'KakaoTalk.exe', title: '민수', time: dayAgo(1, 17, 28) });
await capture('dialog.png', { app: 'Windows 탐색기', exe: 'explorer.exe', title: '네트워크 드라이브 연결', time: dayAgo(1, 10, 2) });
await capture('code.png', { app: 'Visual Studio Code', exe: 'Code.exe', title: 'sync.go - screenmit - Visual Studio Code', time: dayAgo(5, 22, 40) });
await capture('web.png', { app: 'Microsoft Edge', exe: 'msedge.exe', title: 'CV04_edge_detection.pdf - Penmit PDF - Microsoft Edge', time: dayAgo(12, 15, 30) });
// duplicate pixels are not saved twice
await capture('ppt.png', { app: 'Microsoft PowerPoint', time: dayAgo(0, 14, 5) });

await page.waitForFunction(() => document.querySelectorAll('.card').length === 7, null, { timeout: 5000 });
assert.equal(await page.textContent('#nAll'), '7');
const heads = await page.$$eval('.day header h2', (h) => h.map((x) => x.textContent));
assert.deepEqual(heads.slice(0, 2), ['오늘', '어제']);
assert.equal(heads.length, 4);
const exp = await page.$$eval('.day .expire', (e) => e.map((x) => x.textContent));
assert.equal(exp[0], '14일 남음');
assert.equal(exp[3], '내일까지 보관', `expire texts ${exp}`);
const apps = await page.$$eval('.card .app', (e) => e.map((x) => x.textContent));
assert.equal(apps[0], 'PowerPoint'); // "Microsoft " dropped
// files really on disk in day folders
const today = new Date(); const ymd = (d) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
assert.ok(fs.readdirSync(path.join(folder, ymd(today))).some((f) => /^Screenmit_.*_14-03-12\.png$/.test(f)));
await page.mouse.move(0, 0);
await page.waitForTimeout(1000); // arrival highlight fades
await page.screenshot({ path: `${out}/1-gallery.png` });

// search
await page.fill('#q', '경비');
await page.waitForFunction(() => document.querySelectorAll('.card').length === 1);
await page.fill('#q', '카카오');
await page.waitForSelector('.empty h2');
await page.fill('#q', 'kakao');
await page.waitForFunction(() => document.querySelectorAll('.card').length === 1);
await page.press('#q', 'Escape');
await page.waitForFunction(() => document.querySelectorAll('.card').length === 7);

// viewer
await page.click('.card >> nth=0');
await page.waitForSelector('#viewer:not([hidden])');
await page.waitForFunction(() => document.querySelector('#vImg').src.includes('/img/'));
assert.match(await page.textContent('#vSrc'), /PowerPoint · CV04 정리/);
assert.equal(await page.textContent('#vPos'), '1 / 7');
await page.keyboard.press('ArrowRight');
assert.equal(await page.textContent('#vPos'), '2 / 7');
await page.keyboard.press('ArrowLeft');
// memo
await page.keyboard.press('m');
await page.fill('#vNote', 'Sobel 비교 — 시험 범위');
await page.keyboard.press('Enter');
await page.waitForTimeout(400);
await page.screenshot({ path: `${out}/2-viewer.png` });
// copy
await page.keyboard.press('Control+c');
await page.waitForFunction(() => /복사했어요/.test(document.querySelector('#toastMsg').textContent));
let plat = await (await page.request.get(`${origin}/api/dev/platform`)).json();
assert.equal(plat.copied, 1);
// download
const [dl] = await Promise.all([page.waitForEvent('download'), page.click('#viewer [data-act=save]')]);
assert.match(dl.suggestedFilename(), /^Screenmit_.*\.png$/);
// pin from the viewer
await page.keyboard.press('p');
await page.waitForFunction(() => document.querySelector('#viewer [data-act=pin]').classList.contains('on'));
await page.keyboard.press('Escape');
await page.waitForSelector('#viewer', { state: 'hidden' });
await page.waitForFunction(() => document.querySelector('#nKept').textContent === '1');
await page.waitForTimeout(300);
assert.ok(fs.readdirSync(path.join(folder, '보관함')).length === 1, 'pinned file moved to 보관함');
// note is searchable
await page.fill('#q', '시험 범위');
await page.waitForFunction(() => document.querySelectorAll('.card').length === 1);
await page.press('#q', 'Escape');

// delete + undo, then delete for real
const card = page.locator('.card').nth(2);
const delId = await card.getAttribute('data-id');
await card.focus();
await page.keyboard.press('Delete');
await page.waitForFunction(() => document.querySelectorAll('.card').length === 6);
await page.click('#toastAct');
await page.waitForFunction(() => document.querySelectorAll('.card').length === 7);
await page.locator(`.card[data-id="${delId}"]`).focus();
await page.keyboard.press('Delete');
await page.waitForTimeout(5600);
let st = await (await page.request.get(`${origin}/api/state`)).json();
assert.equal(st.shots.length, 6, 'deleted after undo window');

// kept tab
await page.click('.tab[data-tab=kept]');
await page.waitForFunction(() => document.querySelectorAll('.card').length === 1);
await page.click('.tab[data-tab=all]');

// pause
await page.click('#pauseBtn');
await page.waitForSelector('#pauseBtn.paused');
await page.waitForSelector('#banner:not([hidden])');
await page.click('#resumeBtn');
await page.waitForSelector('#pauseBtn:not(.paused)');

// settings
await page.click('#settingsBtn');
await page.waitForSelector('#settings:not([hidden])');
await page.screenshot({ path: `${out}/3-settings.png` });
// hotkey: taken one fails, free one works
await page.click('#hotkeyBtn');
await page.keyboard.press('Control+Alt+KeyX');
await page.waitForFunction(() => /쓰고 있어요/.test(document.querySelector('#toastMsg').textContent));
await page.click('#hotkeyBtn');
await page.keyboard.press('Control+Shift+Alt+KeyQ');
await page.waitForFunction(() => document.querySelector('#hotkeyBtn').textContent === 'Ctrl + Alt + Shift + Q');
plat = await (await page.request.get(`${origin}/api/dev/platform`)).json();
assert.equal(plat.hotkey, 'Ctrl+Alt+Shift+Q');
// autostart switch
await page.click('label.row:has(#autostart)');
plat = await (await page.request.get(`${origin}/api/dev/platform`)).json();
assert.equal(plat.autostart, false);
// retention 7: the 12-day-old shot goes (after a confirm)
await page.click('#retention button[data-days="7"]');
await page.waitForSelector('#confirm:not([hidden])');
assert.match(await page.textContent('#confirmMsg'), /1장이 바로 지워져요/);
await page.click('#confirmYes');
await page.waitForFunction(() => document.querySelector('#retention .on')?.textContent === '7일');
st = await (await page.request.get(`${origin}/api/state`)).json();
assert.equal(st.shots.length, 5);

// Google Drive: drop the client JSON, name the PC, connect
const fakeClient = await (await page.request.get(`${origin}/api/dev/fake-client`)).text();
const clientFile = path.join(out, 'client_secret_test.json');
fs.writeFileSync(clientFile, fakeClient);
await page.setInputFiles('#jsonFile', clientFile);
await page.waitForSelector('#pcNameInput');
await page.click('[data-pcname="회사 PC"]');
await page.screenshot({ path: `${out}/4-drive-connect.png` });
await page.click('#connectBtn');
await page.waitForFunction(() => /choo@example\.com/.test(document.querySelector('#driveGroup').textContent), null, { timeout: 15000 });
await page.waitForFunction(() => /맞춤/.test(document.querySelector('#driveGroup').textContent), null, { timeout: 15000 });
let drv = await (await page.request.get(`${origin}/api/dev/drive`)).json();
assert.equal(drv.shots.length, 5, 'all local shots uploaded');
assert.equal(drv.shots[0].appProperties.pc, '회사 PC');
assert.equal(JSON.parse(drv.settings[0] ? '{}' : '{}') && drv.settings.length, 1);
await page.screenshot({ path: `${out}/5-drive-connected.png` });
await page.click('#settings [data-close]');

// a screenshot from home arrives
const r = await page.request.post(`${origin}/api/dev/remote?${new URLSearchParams({ pc: '집 PC', app: 'Chrome', title: 'Gmail - 받은편지함', time: dayAgo(0, 8, 5) })}`, { headers: H, data: fs.readFileSync(path.join(samples, 'web.png')) });
assert.equal(r.status(), 200, await r.text());
const remoteId = (await r.json()).id;
await page.waitForFunction((id) => !!document.querySelector(`.card[data-id="${id}"]`), remoteId, { timeout: 15000 });
assert.ok(await page.isVisible('#pcs'));
const chips = await page.$$eval('#pcs .chip', (c) => c.map((x) => x.textContent));
assert.deepEqual(chips, ['모든 PC', '회사 PC', '집 PC']);
assert.equal(await page.textContent(`.card[data-id="${remoteId}"] .pc`), '집 PC');
await page.mouse.move(0, 0);
await page.evaluate(() => { document.querySelector('#toast').hidden = true; });
await page.waitForTimeout(1000);
await page.screenshot({ path: `${out}/6-two-pcs.png` });
await page.click('#pcs .chip:has-text("집 PC")');
await page.waitForFunction(() => document.querySelectorAll('.card').length === 1);
await page.click('#pcs .chip:has-text("모든 PC")');
// open it: the full picture comes from Drive
await page.click(`.card[data-id="${remoteId}"]`);
await page.waitForFunction(() => document.querySelector('#vImg').src.includes('/img/') && document.querySelector('#vLoading').hidden, null, { timeout: 10000 });
assert.ok(await page.isDisabled('#viewer [data-act=reveal]'));
await page.keyboard.press('Escape');
// home pins it
await page.request.post(`${origin}/api/dev/remote-edit`, { headers: H, data: { id: remoteId, pin: true } });
await page.waitForFunction((id) => !!document.querySelector(`.card[data-id="${id}"] .pinned`), remoteId, { timeout: 15000 });
// home deletes it
await page.request.post(`${origin}/api/dev/remote-edit`, { headers: H, data: { id: remoteId, delete: true } });
await page.waitForFunction((id) => !document.querySelector(`.card[data-id="${id}"]`), remoteId, { timeout: 15000 });
// a delete here reaches Drive
const delId2 = await page.locator('.card').nth(1).getAttribute('data-id');
await page.locator(`.card[data-id="${delId2}"]`).focus();
await page.keyboard.press('Delete');
await page.waitForTimeout(5600);
await page.request.post(`${origin}/api/drive/sync`, { headers: H });
await page.waitForTimeout(2500);
drv = await (await page.request.get(`${origin}/api/dev/drive`)).json();
assert.ok(!drv.shots.some((f) => f.appProperties.id === delId2), 'deleted on drive');

// Google login gone: only this PC's screenshots, and no warning bar
const homeId = (await page.request.post(`${origin}/api/dev/remote?${new URLSearchParams({ pc: '집 PC', app: 'Chrome', title: '메일', time: dayAgo(0, 7, 10) })}`,
  { headers: H, data: fs.readFileSync(path.join(samples, 'mail.png')) }).then((x) => x.json())).id;
await page.waitForFunction((id) => !!document.querySelector(`.card[data-id="${id}"]`), homeId, { timeout: 15000 });
await page.request.post(`${origin}/api/dev/drive-revoke`, { headers: H });
await page.waitForFunction((id) => !document.querySelector(`.card[data-id="${id}"]`), homeId, { timeout: 15000 });
assert.ok(await page.isHidden('#banner'), 'no warning bar');
assert.ok(await page.isHidden('#pcs'), 'no PC filter with only this PC');
assert.ok(!(await page.$eval('#syncBtn', (b) => b.classList.contains('err'))), 'no red dot');
assert.match(await page.getAttribute('#syncBtn', 'title'), /이 PC 스크린샷만/);
st = await (await page.request.get(`${origin}/api/state`)).json();
assert.ok(st.shots.some((x) => x.id === homeId), 'still known, just hidden');
await page.click('#settingsBtn');
await page.waitForFunction(() => /로그인이 풀렸어요/.test(document.querySelector('#driveGroup').textContent));
assert.match(await page.getAttribute('#webLinkA', 'href'), new RegExp(`^https://choo-ai-dev\\.github\\.io/Screenmit/#local=${new URL(origin).port}\\.[0-9a-f]{48}$`));
await page.screenshot({ path: `${out}/6b-drive-down-settings.png` });
await page.click('#settings [data-close]');

// screenshot key: off at first; pick one in the settings, then press it
const platNow = async () => (await page.request.get(`${origin}/api/dev/platform`)).json();
const stateNow = async () => (await page.request.get(`${origin}/api/state`)).json();
const pressKey = () => page.request.post(`${origin}/api/dev/screenshot-key`, { headers: H });
await page.click('#settingsBtn');
await page.waitForSelector('#settings:not([hidden])');
assert.equal(await page.textContent('#captureBtn'), '키 정하기');
assert.equal(await page.textContent('#captureMode .on'), '영역 선택');
assert.ok(await page.isHidden('#captureOff'));
// the gallery's own key can't be used twice
await page.click('#captureBtn');
await page.keyboard.press('Control+Shift+Alt+KeyQ');
await page.waitForFunction(() => /갤러리 여는 단축키와 같아요/.test(document.querySelector('#toastMsg').textContent));
// a letter alone needs Ctrl or Alt; F keys don't
await page.click('#captureBtn');
await page.keyboard.press('KeyK');
assert.match(await page.textContent('#captureBtn'), /Ctrl 이나 Alt/);
await page.keyboard.press('F9');
await page.waitForFunction(() => document.querySelector('#captureBtn').textContent === 'F9');
assert.equal((await platNow()).captureKey, 'F9');
// Print Screen only shows up when it is let go
await page.click('#captureBtn');
await page.keyboard.up('PrintScreen');
await page.waitForFunction(() => document.querySelector('#captureBtn').textContent === 'Print Screen');
assert.equal((await platNow()).captureKey, 'PrintScreen');
assert.match(await page.textContent('#captureMsg'), /Print Screen 키로 화면 캡처 열기/);
// "영역 선택" hands over to Windows' snipping overlay
const before = (await stateNow()).shots.length;
await pressKey();
plat = await platNow();
assert.equal(plat.regions, 1);
assert.equal(plat.grabs, 0);
// "지금 창": taken here, saved and copied at once
await page.click('#captureMode button[data-mode="window"]');
await page.waitForFunction(() => document.querySelector('#captureMode .on')?.textContent === '지금 창');
assert.match(await page.textContent('#captureModeDesc'), /앞에 있는 창/);
const copied0 = plat.copied;
await pressKey();
plat = await platNow();
assert.equal(plat.grabs, 1);
assert.equal(plat.copied, copied0 + 1, 'copied like Print Screen');
assert.ok(plat.notes.includes('스크린샷을 찍었어요'), plat.notes.join());
st = await stateNow();
assert.equal(st.shots.length, before + 1);
const taken = st.shots.find((x) => x.title === '가계부 1.xlsx - Excel');
assert.ok(taken && taken.app === 'Excel' && taken.w === 900, JSON.stringify(taken));
assert.equal(st.settings.captureMode, 'window');
// paused: still copied, not saved
await page.request.post(`${origin}/api/pause`, { headers: H, data: { paused: true } });
await pressKey();
plat = await platNow();
assert.equal(plat.copied, copied0 + 2);
assert.equal((await stateNow()).shots.length, before + 1);
assert.equal(plat.notes[plat.notes.length - 1], '스크린샷을 복사했어요');
await page.request.post(`${origin}/api/pause`, { headers: H, data: { paused: false } });
await page.waitForSelector('#pauseBtn:not(.paused)');
await page.screenshot({ path: `${out}/6c-capture-key.png` });
// off again
await page.click('#captureOff');
await page.waitForFunction(() => document.querySelector('#captureBtn').textContent === '키 정하기');
assert.equal((await platNow()).captureKey, 'off');
await page.click('#settings [data-close]');
await page.waitForSelector('.card:has-text("가계부 1.xlsx")', { timeout: 5000 });

// dark mode
await page.emulateMedia({ colorScheme: 'dark' });
await page.screenshot({ path: `${out}/7-dark.png` });
await page.emulateMedia({ colorScheme: 'light' });

// a page without the cookie gets nothing
const other = await browser.newContext();
const op = await other.newPage();
await op.goto(`${origin}/`);
await op.waitForSelector('.empty h2');
assert.match(await op.textContent('.empty h2'), /다시 열어야/);
const leak = await op.request.get(`${origin}/api/state`);
assert.equal(leak.status(), 401);
const csrf = await page.request.post(`${origin}/api/pause`, { data: { paused: true } });
assert.equal(csrf.status(), 403, 'change without our header is refused');
const rebinding = await page.request.get(`${origin}/api/state`, { headers: { Host: 'evil.example:28100' } });
assert.equal(rebinding.status(), 403);

// mobile width still usable
await page.setViewportSize({ width: 420, height: 860 });
await page.screenshot({ path: `${out}/8-narrow.png` });

// the 400s are the two shortcut conflicts tried on purpose above
assert.deepEqual(errors.filter((e) => !/status of 400/.test(e)), [], 'console errors');
assert.ok(errors.length <= 2, errors.join('\n'));
await browser.close();
proc.kill('SIGINT');
console.log('E2E OK');
