// Makes the README pictures (docs/*.png) and the picture for the personal site.
// Real-looking screenshots (an edge-detection slide computed with scikit-image,
// a travel-expense sheet whose sums add up, this repo's own code and a real
// compile error, a Penmit PDF screen) go into the dev build with a fake Drive,
// and the gallery, viewer, settings and the phone web page are captured.
//
//   go build -o /tmp/screenmit-dev .
//   node tools/docs_shots.mjs /tmp/screenmit-dev OUT_DIR [penmit-pdf-screenshot.jpg]
//
// Copy OUT_DIR/docs/*.png into docs/. OUT_DIR/site-screenmit.jpg is for the personal site.
import { spawn, execFileSync } from 'node:child_process';
import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';

process.env.TZ = 'Asia/Seoul';
const { chromium, devices } = await import('playwright').catch(() => import('/opt/npm-tools/node_modules/playwright/index.mjs'));

const bin = process.argv[2];
const out = path.resolve(process.argv[3] || '/tmp/sm-docs');
const penmit = process.argv[4];
const repo = path.dirname(path.dirname(new URL(import.meta.url).pathname));
const scenes = path.join(out, 'scenes');
const docs = path.join(out, 'docs');
for (const d of [scenes, docs]) fs.mkdirSync(d, { recursive: true });

// ---------- pictures computed for the slide ----------
execFileSync('python3', ['-c', `
import sys, numpy as np
from skimage import data, filters, exposure, io
img = data.camera().astype(float) / 255
def mag(fx, fy):
    m = np.hypot(fx(img), fy(img)); return (exposure.rescale_intensity(m, out_range=(0, 1)) ** 0.6 * 255).astype('uint8')
io.imsave(sys.argv[1] + '/cam.png', (img * 255).astype('uint8'))
io.imsave(sys.argv[1] + '/sobel.png', mag(filters.sobel_h, filters.sobel_v))
io.imsave(sys.argv[1] + '/prewitt.png', mag(filters.prewitt_h, filters.prewitt_v))
io.imsave(sys.argv[1] + '/sobelx.png', (exposure.rescale_intensity(filters.sobel_v(img), out_range=(0, 1)) * 255).astype('uint8'))
`, scenes]);
const uri = (f, type = 'image/png') => `data:${type};base64,${fs.readFileSync(f).toString('base64')}`;
const cam = uri(path.join(scenes, 'cam.png'));
const sobel = uri(path.join(scenes, 'sobel.png'));
const prewitt = uri(path.join(scenes, 'prewitt.png'));
const sobelx = uri(path.join(scenes, 'sobelx.png'));

const FONT = `"Noto Sans CJK KR", "Noto Sans", "DejaVu Sans", sans-serif`;
const MONO = `"DejaVu Sans Mono", "Liberation Mono", monospace`;
const base = `*{box-sizing:border-box}html,body{margin:0;height:100%;overflow:hidden}body{font-family:${FONT};font-size:13px;color:#1f1f1f}`;
// a Windows 11 title bar, as Alt + Print Screen gets it
const titlebar = (title, { bg = '#f3f3f3', fg = '#1f1f1f', left = '' } = {}) => `
<div style="height:32px;display:flex;align-items:center;background:${bg};color:${fg};font-size:12px;position:relative">
  <div style="padding-left:12px;display:flex;gap:10px;align-items:center">${left}</div>
  <div style="position:absolute;left:0;right:0;text-align:center;pointer-events:none">${title}</div>
  <div style="margin-left:auto;display:flex;height:100%">
    ${['<div style="width:10px;border-top:1px solid currentColor"></div>', '<div style="width:10px;height:10px;border:1px solid currentColor"></div>', '<div style="font-size:15px;line-height:1">✕</div>']
      .map((g) => `<div style="width:46px;display:grid;place-items:center;opacity:.85">${g}</div>`).join('')}
  </div>
</div>`;

// ---------- 1. PowerPoint: a study slide on Sobel vs Prewitt ----------
const pptTabs = ['파일', '홈', '삽입', '그리기', '디자인', '전환', '애니메이션', '슬라이드 쇼', '녹음/녹화', '검토', '보기', '도움말'];
const slideThumb = (n, sel, inner) => `<div style="display:flex;gap:6px;margin:0 10px 12px 6px"><div style="width:14px;text-align:right;color:#444;font-size:11px">${n}</div>
  <div style="flex:1;aspect-ratio:16/9;background:#fff;border:${sel ? '2.5px solid #c43e1c' : '1px solid #c8c8c8'};padding:6px 8px;overflow:hidden">${inner}</div></div>`;
const thumbTitle = (t) => `<div style="font-size:7px;font-weight:700;color:#1f2a44">${t}</div><div style="height:1px;background:#c43e1c;margin:3px 0 4px"></div>`;
const thumbLines = (n) => Array.from({ length: n }, (_, i) => `<div style="height:3px;background:#cfd5de;margin:3px 0;width:${90 - (i % 3) * 18}%"></div>`).join('');
const pptHTML = `<!doctype html><html><head><meta charset="utf-8"><style>${base}
.rib{height:96px;background:#f8f8f8;border-bottom:1px solid #dcdcdc;display:flex;padding:6px 10px;gap:0}
.grp{display:flex;flex-direction:column;justify-content:space-between;padding:0 12px;border-right:1px solid #e0e0e0}
.grp .lbl{font-size:11px;color:#666;text-align:center}
.btn{display:flex;flex-direction:column;align-items:center;gap:3px;font-size:11px;color:#333}
.ico{width:26px;height:26px;border-radius:4px}
.sm{display:flex;gap:6px;align-items:center;font-size:12px;color:#333}
.box{border:1px solid #c8c8c8;background:#fff;padding:1px 6px;font-size:12px}
</style></head><body>
${titlebar('CV04 정리.pptx - PowerPoint', { bg: '#c43e1c', fg: '#fff', left: '<span style="font-weight:700">자동 저장</span><span style="display:inline-block;width:26px;height:13px;border-radius:7px;border:1px solid #fff"></span>' })}
<div style="height:30px;display:flex;align-items:end;gap:0;padding-left:8px;background:#fff">${pptTabs.map((t, i) => `<div style="padding:0 11px 6px;font-size:12.5px;${i === 1 ? 'color:#c43e1c;border-bottom:3px solid #c43e1c;font-weight:600' : 'color:#333'}">${t}</div>`).join('')}</div>
<div class="rib">
  <div class="grp"><div style="display:flex;gap:10px"><div class="btn"><div class="ico" style="background:#d9dee8"></div>붙여넣기</div><div style="display:flex;flex-direction:column;gap:6px;font-size:11px;color:#333;padding-top:2px"><span>✂ 잘라내기</span><span>⧉ 복사</span></div></div><div class="lbl">클립보드</div></div>
  <div class="grp"><div style="display:flex;gap:10px"><div class="btn"><div class="ico" style="background:#f3d5cc"></div>새 슬라이드</div><div style="display:flex;flex-direction:column;gap:6px;font-size:11px;color:#333;padding-top:2px"><span>레이아웃 ▾</span><span>다시 설정</span></div></div><div class="lbl">슬라이드</div></div>
  <div class="grp"><div style="display:flex;flex-direction:column;gap:6px"><div class="sm"><span class="box" style="width:120px">맑은 고딕 (제목)</span><span class="box">28</span></div><div class="sm" style="gap:12px;font-size:13px"><b>가</b><i>가</i><u>가</u><s>가</s><span style="border-bottom:3px solid #d00">가</span></div></div><div class="lbl">글꼴</div></div>
  <div class="grp"><div style="display:flex;flex-direction:column;gap:8px;font-size:13px;color:#444"><span>☰ &nbsp;☷ &nbsp;⇤ ⇥</span><span>≡ &nbsp;≣ &nbsp;≡ &nbsp;▤</span></div><div class="lbl">단락</div></div>
  <div class="grp"><div style="display:flex;gap:8px;align-items:center"><div style="display:grid;grid-template-columns:repeat(4,16px);gap:3px">${'□○△▷╲⬠☆⬭'.split('').map((c) => `<span style="font-size:13px;color:#555">${c}</span>`).join('')}</div><div class="btn"><div class="ico" style="background:#e4e8f1"></div>정렬</div><div class="btn"><div class="ico" style="background:#dfe9f6"></div>빠른 스타일</div></div><div class="lbl">그리기</div></div>
  <div class="grp" style="border:0"><div style="display:flex;flex-direction:column;gap:6px;font-size:11px;color:#333"><span>⌕ 찾기</span><span>⇄ 바꾸기 ▾</span><span>⬚ 선택 ▾</span></div><div class="lbl">편집</div></div>
</div>
<div style="display:flex;height:calc(100% - 32px - 30px - 96px - 24px)">
  <div style="width:206px;background:#f0f0f0;padding-top:12px;overflow:hidden;border-right:1px solid #d8d8d8">
    ${slideThumb(1, false, `<div style="font-size:9px;font-weight:700;margin-top:14px;text-align:center">CV04 Edge Detection</div><div style="font-size:6px;text-align:center;color:#666">정리 노트</div>`)}
    ${slideThumb(2, false, thumbTitle('이미지 기울기') + thumbLines(5))}
    ${slideThumb(3, true, thumbTitle('Sobel vs Prewitt') + `<div style="display:flex;gap:3px"><img src="${cam}" style="width:30%"><img src="${sobel}" style="width:30%"><img src="${prewitt}" style="width:30%"></div>`)}
    ${slideThumb(4, false, thumbTitle('분리 가능한 필터') + thumbLines(4))}
    ${slideThumb(5, false, thumbTitle('Canny 단계') + thumbLines(6))}
    ${slideThumb(6, false, thumbTitle('비최대 억제') + thumbLines(3))}
  </div>
  <div style="flex:1;background:#e6e6e6;display:flex;flex-direction:column;align-items:center;padding-top:22px">
    <div style="width:1080px;height:608px;background:#fff;box-shadow:0 1px 4px rgba(0,0,0,.25);padding:34px 54px;position:relative">
      <div style="font-size:34px;font-weight:700;color:#1f2a44;letter-spacing:-.01em">Sobel vs Prewitt — 가운데 행 가중치</div>
      <div style="height:3px;background:#c43e1c;margin:12px 0 22px;width:100%"></div>
      <div style="display:flex;gap:22px">
        ${[[cam, '원본 (camera)'], [sobel, 'Sobel |∇I|'], [prewitt, 'Prewitt |∇I|']].map(([s, c]) => `<figure style="margin:0;width:300px"><img src="${s}" style="width:300px;height:300px;display:block"><figcaption style="text-align:center;font-size:15px;margin-top:6px;color:#333">${c}</figcaption></figure>`).join('')}
      </div>
      <ul style="font-size:17px;line-height:1.55;margin:18px 0 0;padding-left:22px;color:#222">
        <li>Sobel = [1 2 1]ᵀ 평활 × [−1 0 1] 미분 → 가운데 행이 2배, 노이즈에 덜 민감</li>
        <li>Prewitt = [1 1 1]ᵀ, 세 행을 똑같이 → 결과는 비슷, 잔무늬가 조금 더 남음</li>
      </ul>
      <div style="position:absolute;right:54px;bottom:20px;font-size:12px;color:#888">3</div>
    </div>
    <div style="width:1080px;margin-top:14px;background:#fff;border:1px solid #d8d8d8;padding:8px 12px;color:#888;font-size:12px">슬라이드 노트를 추가하려면 클릭하십시오.</div>
  </div>
</div>
<div style="height:24px;background:#f3f3f3;border-top:1px solid #dcdcdc;display:flex;align-items:center;gap:18px;padding:0 12px;font-size:11.5px;color:#444">
  <span>슬라이드 3/8</span><span>한국어</span><span>접근성: 계속 진행 가능</span><span style="margin-left:auto">⌂ 메모 &nbsp; ▭ 표시 설정 &nbsp; ▦ ▣ ▤ ▶ &nbsp; — ────●─── + &nbsp; 82%</span>
</div></body></html>`;

// ---------- 2. Excel: travel expenses (a Win + Shift + S region) ----------
const trip = [
  ['11/7', '항공권 (3명)', '나', 312600], ['11/7', '렌터카 2일', '민수', 128000], ['11/7', '숙소 2박', '지영', 264000],
  ['11/7', '흑돼지', '나', 96000], ['11/8', '성산일출봉 입장료', '민수', 15000], ['11/8', '카페', '지영', 23400],
  ['11/8', '주유', '나', 41200], ['11/9', '동문시장', '민수', 38500], ['11/9', '공항 편의점', '지영', 9800],
];
const total = trip.reduce((a, r) => a + r[3], 0);
const each = total / 3;
const won = (n) => n.toLocaleString('ko-KR');
const paid = Object.fromEntries(['나', '민수', '지영'].map((p) => [p, trip.filter((r) => r[2] === p).reduce((a, r) => a + r[3], 0)]));
const cols = ['A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I'];
const widths = [56, 150, 70, 96, 40, 70, 96, 100, 40];
const rows = [];
rows[1] = { A: ['제주 여행 경비 (11/7 ~ 11/9)', 'font-weight:700;font-size:15px'] };
rows[2] = { A: ['날짜', 'h'], B: ['항목', 'h'], C: ['낸 사람', 'h'], D: ['금액', 'h r'], F: ['정산', 'h'], G: ['낸 돈', 'h r'], H: ['받을/줄 돈', 'h r'] };
trip.forEach((r, i) => { rows[3 + i] = { A: [r[0]], B: [r[1]], C: [r[2]], D: [won(r[3]), 'r'] }; });
['나', '민수', '지영'].forEach((p, i) => {
  const d = paid[p] - each;
  Object.assign(rows[3 + i], { F: [p], G: [won(paid[p]), 'r'], H: [(d > 0 ? '+' : '−') + won(Math.abs(d)), `r;color:${d > 0 ? '#1a7f37' : '#c00000'}`] });
});
rows[12] = { A: ['합계', 'b'], D: [won(total), 'r b sel'] };
rows[13] = { A: ['1인당', 'b'], D: [won(each), 'r b'] };
const cellCSS = (st = '') => st.split(/[ ;]/).filter(Boolean).map((k) => ({ h: 'background:#e7f0e9;font-weight:600', r: 'text-align:right', b: 'font-weight:700', sel: 'outline:2px solid #107c41;outline-offset:-2px' }[k] || (k.includes(':') ? k : ''))).join(';') + (st.includes(':') ? ';' + st.split(';').filter((x) => x.includes(':')).join(';') : '');
const excelHTML = `<!doctype html><html><head><meta charset="utf-8"><style>${base}
table{border-collapse:collapse;table-layout:fixed}td{border:1px solid #e1e1e1;height:22px;padding:0 5px;font-size:13px;white-space:nowrap;overflow:hidden}
.hd{background:#f3f3f3;color:#555;text-align:center;font-size:12px}.hd.on{background:#d3e6dc;color:#0f5132;border-bottom:2px solid #107c41}
</style></head><body style="background:#fff">
<div style="height:30px;display:flex;align-items:center;border-bottom:1px solid #dcdcdc;font-size:13px">
  <div style="width:90px;padding-left:8px;border-right:1px solid #dcdcdc">D12</div><div style="width:64px;text-align:center;color:#666;border-right:1px solid #dcdcdc">✕ ✓ <i>fx</i></div><div style="padding-left:8px">=SUM(D3:D11)</div>
</div>
<table style="width:${34 + widths.reduce((a, b) => a + b, 0)}px"><colgroup><col style="width:34px">${widths.map((w) => `<col style="width:${w}px">`).join('')}</colgroup>
<tr><td class="hd"></td>${cols.map((c) => `<td class="hd${c === 'D' ? ' on' : ''}">${c}</td>`).join('')}</tr>
${Array.from({ length: 15 }, (_, i) => i + 1).map((n) => `<tr><td class="hd${n === 12 ? ' on' : ''}">${n}</td>${cols.map((c) => { const v = rows[n]?.[c]; return `<td style="${v ? cellCSS(v[1]) : ''}">${v ? v[0] : ''}</td>`; }).join('')}</tr>`).join('')}
</table></body></html>`;

// ---------- 3. Penmit PDF in the browser ----------
const browserHTML = penmit && `<!doctype html><html><head><meta charset="utf-8"><style>${base}</style></head><body style="background:#f7f7f7">
<div style="height:40px;display:flex;align-items:end;padding-left:8px;background:#e8eaed;position:relative">
  <div style="width:250px;height:32px;background:#f7f7f7;border-radius:8px 8px 0 0;display:flex;align-items:center;gap:8px;padding:0 10px;font-size:12px">
    <span style="width:16px;height:16px;border-radius:4px;background:#2b6de8;color:#fff;font-size:10px;display:grid;place-items:center;font-weight:700">P</span><span style="flex:1;overflow:hidden;white-space:nowrap">CV04_edge_detection.pdf - Penmit PDF</span><span style="color:#555">✕</span></div>
  <div style="padding:0 12px 8px;color:#555;font-size:16px">+</div>
  <div style="position:absolute;right:0;top:0;display:flex;height:32px">${['—', '▢', '✕'].map((g) => `<div style="width:46px;display:grid;place-items:center;font-size:12px;color:#333">${g}</div>`).join('')}</div>
</div>
<div style="height:40px;display:flex;align-items:center;gap:14px;padding:0 12px;border-bottom:1px solid #dadce0;color:#555;font-size:15px">
  <span>←</span><span style="color:#bbb">→</span><span>↻</span>
  <div style="flex:1;height:30px;border-radius:15px;background:#f1f3f4;display:flex;align-items:center;gap:8px;padding:0 14px;font-size:13px;color:#202124"><span style="color:#5f6368">⊜</span>choo-ai-dev.github.io<span style="color:#5f6368">/penmit-pdf/</span></div>
  <span>☆</span><span>⋯</span>
</div>
<div style="height:calc(100% - 80px);overflow:hidden;background:#fff"><img src="${uri(penmit, 'image/jpeg')}" style="width:100%;display:block;margin-top:-6px"></div>
</body></html>`;

// ---------- 4. a chat on the other PC ----------
const bubble = (me, html, time, pic = false) => `<div style="display:flex;${me ? 'justify-content:flex-end' : ''};align-items:flex-end;gap:5px;margin:6px 0">
  ${me ? `<span style="font-size:10px;color:#556">${time}</span>` : ''}
  <div style="max-width:250px;${pic ? '' : `background:${me ? '#fee500' : '#fff'};padding:7px 10px;`}border-radius:10px;font-size:13px;line-height:1.45">${html}</div>
  ${me ? '' : `<span style="font-size:10px;color:#556">${time}</span>`}</div>`;
const chatHTML = `<!doctype html><html><head><meta charset="utf-8"><style>${base}</style></head><body style="background:#bacee0">
${titlebar('', { bg: '#bacee0', fg: '#333' })}
<div style="display:flex;align-items:center;gap:10px;padding:0 14px 10px"><div style="width:40px;height:40px;border-radius:15px;background:#9fb3c8;display:grid;place-items:center;color:#fff;font-weight:700">민</div><div><div style="font-weight:700;font-size:14px">민수</div><div style="font-size:11px;color:#556">⌕ ☰</div></div></div>
<div style="padding:4px 14px">
  <div style="text-align:center;margin:6px 0 10px"><span style="background:rgba(0,0,0,.12);color:#fff;font-size:11px;padding:3px 10px;border-radius:10px">2026년 11월 17일 화요일</span></div>
  <div style="display:flex;gap:8px"><div style="width:36px;height:36px;border-radius:14px;background:#9fb3c8;flex:none"></div><div><div style="font-size:12px;color:#333;margin-bottom:2px">민수</div>
  ${bubble(false, '제주 경비 정리한 거 다시 보내줄 수 있어? 단톡방에서 못 찾겠어 ㅠ', '오후 12:16')}</div></div>
  ${bubble(true, '잠깐만', '오후 12:17')}
  ${bubble(true, '<div style="width:228px;height:120px;overflow:hidden;border-radius:10px;background:#fff"><iframe srcdoc="' + excelHTML.replace(/"/g, '&quot;') + '" style="width:760px;height:400px;border:0;transform:scale(.3);transform-origin:0 0"></iframe></div>', '오후 12:18', true)}
  ${bubble(true, '1인당 309,500원!<br>나한테 민수 128,000 / 지영 12,300', '오후 12:18')}
  <div style="display:flex;gap:8px;margin-top:6px"><div style="width:36px;height:36px;border-radius:14px;background:#9fb3c8;flex:none"></div><div>${bubble(false, '오케이 지금 보낼게 👍', '오후 12:19')}</div></div>
</div>
<div style="position:absolute;left:0;right:0;bottom:0;height:96px;background:#fff;padding:10px 14px;color:#999;font-size:13px">메시지 입력<div style="position:absolute;right:12px;bottom:12px;background:#f5f5f5;color:#bbb;padding:6px 14px;border-radius:4px;font-size:12px">전송</div></div>
</body></html>`;

// ---------- 5. this repository's code ----------
const src = fs.readFileSync(path.join(repo, 'internal/drive/sync.go'), 'utf8').split('\n');
const first = src.findIndex((l) => l.startsWith('func (s *Syncer) SyncOnce')) + 24;
const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;');
const KW = /^(if|for|range|return|defer|else|switch|case|break|continue|go|select)$/;
const KW2 = /^(func|var|const|type|struct|nil|true|false|interface|map|chan|package|import)$/;
function hl(line) {
  line = line.replace(/\t/g, '    ');
  let outS = '';
  const re = /(\/\/.*$)|("(?:[^"\\]|\\.)*"|`[^`]*`)|([A-Za-z_][A-Za-z0-9_]*)(?=\()|([A-Za-z_][A-Za-z0-9_]*)|(\d+)|(\s+)|(.)/g;
  let m;
  while ((m = re.exec(line))) {
    if (m[1]) outS += `<span style="color:#6a9955">${esc(m[1])}</span>`;
    else if (m[2]) outS += `<span style="color:#ce9178">${esc(m[2])}</span>`;
    else if (m[3]) outS += KW.test(m[3]) ? `<span style="color:#c586c0">${m[3]}</span>` : KW2.test(m[3]) ? `<span style="color:#569cd6">${m[3]}</span>` : `<span style="color:#dcdcaa">${m[3]}</span>`;
    else if (m[4]) outS += KW.test(m[4]) ? `<span style="color:#c586c0">${m[4]}</span>` : KW2.test(m[4]) ? `<span style="color:#569cd6">${m[4]}</span>` : /^[A-Z]/.test(m[4]) && /^(Status|Shot|Meta|Store)$/.test(m[4]) ? `<span style="color:#4ec9b0">${m[4]}</span>` : `<span style="color:#9cdcfe">${m[4]}</span>`;
    else if (m[5]) outS += `<span style="color:#b5cea8">${m[5]}</span>`;
    else outS += esc(m[0]);
  }
  return outS;
}
const codeLines = src.slice(first, first + 34);
const tree = [['SCREENMIT', 0, 'h'], ['.github', 1], ['internal', 1, 'o'], ['config', 2], ['drive', 2, 'o'], ['fakedrive', 3], ['api.go', 3], ['oauth.go', 3], ['sync.go', 3, 's'], ['sync_test.go', 3], ['imgclip', 2], ['server', 2], ['shots', 2], ['site', 1], ['tests', 1], ['tools', 1], ['web', 1], ['app.go', 1], ['main.go', 1], ['README.md', 1]];
const codeHTML = `<!doctype html><html><head><meta charset="utf-8"><style>${base}body{background:#1e1e1e;color:#ccc}</style></head><body>
${titlebar('sync.go - screenmit - Visual Studio Code', { bg: '#1f1f1f', fg: '#ccc', left: '<span style="color:#3794ff;font-weight:700">⧉</span><span>파일 &nbsp;편집 &nbsp;선택 &nbsp;보기 &nbsp;이동 &nbsp;실행 &nbsp;터미널 &nbsp;도움말</span>' })}
<div style="display:flex;height:calc(100% - 32px - 22px)">
  <div style="width:48px;background:#181818;display:flex;flex-direction:column;align-items:center;gap:22px;padding-top:14px;color:#858585;font-size:20px"><span style="color:#d7d7d7">⧉</span><span>⌕</span><span>⑂</span><span>▷</span><span>⊞</span></div>
  <div style="width:240px;background:#181818;border-right:1px solid #2b2b2b;font-size:13px;padding-top:8px">
    <div style="padding:2px 18px 8px;font-size:11px;color:#bbb;letter-spacing:.04em">탐색기</div>
    ${tree.map(([n, d, k]) => `<div style="padding:2px 0 2px ${8 + d * 12}px;${k === 's' ? 'background:#37373d;color:#fff' : k === 'h' ? 'font-weight:700;font-size:11px' : ''};color:${k === 's' ? '#fff' : /\./.test(n) && !n.startsWith('.') ? '#ccc' : '#ccc'}">${k === 'o' ? '⌄ ' : k === 'h' ? '⌄ ' : /\.(go|md)$/.test(n) ? '<span style="color:#519aba;font-size:11px">' + (n.endsWith('.md') ? 'ⓘ' : 'GO') + '</span> ' : '› '}${n}</div>`).join('')}
  </div>
  <div style="flex:1;display:flex;flex-direction:column;min-width:0">
    <div style="height:35px;display:flex;background:#181818">
      <div style="padding:0 14px;display:flex;align-items:center;gap:8px;background:#1e1e1e;border-top:1px solid #0078d4;color:#fff"><span style="color:#519aba;font-size:11px">GO</span>sync.go <span style="color:#aaa">✕</span></div>
      <div style="padding:0 14px;display:flex;align-items:center;gap:8px;color:#999"><span style="color:#519aba;font-size:11px">GO</span>app.go</div>
      <div style="padding:0 14px;display:flex;align-items:center;gap:8px;color:#999"><span style="color:#519aba;font-size:11px">GO</span>sync_test.go</div>
    </div>
    <div style="height:22px;padding-left:18px;color:#a9a9a9;font-size:12px">internal › drive › <span style="color:#519aba">GO</span> sync.go › ⬡ (*Syncer).SyncOnce</div>
    <pre style="margin:0;font-family:${MONO};font-size:14px;line-height:20px;color:#d4d4d4;overflow:hidden">${codeLines.map((l, i) => `<div style="display:flex${i === 12 ? ';background:#2a2d2e' : ''}"><span style="flex:none;width:64px;text-align:right;padding-right:22px;color:${i === 12 ? '#c6c6c6' : '#6e7681'}">${first + 1 + i}</span><span>${hl(l)}</span></div>`).join('')}</pre>
  </div>
</div>
<div style="height:22px;background:#0078d4;color:#fff;display:flex;align-items:center;gap:16px;padding:0 10px;font-size:12px"><span>⑂ main</span><span>⊗ 0 ⚠ 0</span><span style="margin-left:auto">줄 ${first + 13}, 열 9</span><span>탭 크기: 4</span><span>UTF-8</span><span>LF</span><span>{ } Go</span><span>Go 1.24.4</span></div>
</body></html>`;

// ---------- 6. PowerShell: a real compile error ----------
const psHTML = `<!doctype html><html><head><meta charset="utf-8"><style>${base}body{background:#0c0c0c;color:#cccccc}</style></head><body>
<pre style="margin:0;padding:10px 12px;font-family:${MONO};font-size:14px;line-height:19px;white-space:pre-wrap">PS C:\\dev\\screenmit&gt; go test ./internal/drive/
# github.com/Choo-AI-Dev/screenmit/internal/drive [github.com/Choo-AI-Dev/screenmit/internal/drive.test]
<span style="color:#e74856">internal\\drive\\sync.go:183:2: declared and not used: kept</span>
FAIL	github.com/Choo-AI-Dev/screenmit/internal/drive [build failed]
FAIL
PS C:\\dev\\screenmit&gt; <span style="background:#cccccc"> </span></pre></body></html>`;

// ---------- 7. mail on the other PC ----------
const mails = [
  ['GitHub', '[Choo-AI-Dev/Screenmit] Release 0.2.1', 'Screenmit 0.2.1 — 구글이 안 될 때도 그대로 써요.', '오전 8:31', true],
  ['숲속캠핑장', '예약이 확정되었습니다 (11/21~22, B-12)', '체크인 15:00 · 예약번호 2611-0482', '오전 7:58', true],
  ['민수', '제주 사진 모음', '공유 앨범 링크 보내! 사진 186장', '어제', false],
  ['GitHub', '[Choo-AI-Dev/penmit-pdf] Run succeeded: Pages', 'Deploy to GitHub Pages · main', '11월 14일', false],
  ['지영', 'Re: 렌터카 영수증', '사진으로 첨부했어요', '11월 10일', false],
];
const mailHTML = `<!doctype html><html><head><meta charset="utf-8"><style>${base}</style></head><body style="background:#f6f8fc">
${titlebar('받은편지함 - 메일', { bg: '#f6f8fc' })}
<div style="display:flex;height:calc(100% - 32px)">
  <div style="width:210px;padding:10px 8px;font-size:13.5px">
    <div style="background:#c2e7ff;border-radius:16px;padding:12px 18px;width:120px;font-weight:600;margin-bottom:14px">✎ 편지 쓰기</div>
    ${[['받은편지함', 3, true], ['별표편지함'], ['보낸편지함'], ['임시보관함', 1], ['스팸함']].map(([t, n, on]) => `<div style="padding:6px 18px;border-radius:0 16px 16px 0;${on ? 'background:#d3e3fd;font-weight:700' : ''};display:flex">${t}<span style="margin-left:auto">${n || ''}</span></div>`).join('')}
  </div>
  <div style="flex:1;background:#fff;border-radius:14px;margin:6px 10px 10px 0;overflow:hidden">
    <div style="height:46px;border-bottom:1px solid #eee;display:flex;align-items:center;padding:0 16px;color:#666;gap:18px">☐ ⟳ ⋮<span style="margin-left:auto;font-size:12px">1–5 / 5</span></div>
    ${mails.map(([who, subj, prev, when, unread]) => `<div style="display:flex;gap:14px;align-items:center;padding:0 16px;height:40px;border-bottom:1px solid #f1f1f1;font-size:13.5px;${unread ? 'font-weight:700' : 'background:#f8fafd;color:#333'}">
      <span style="color:#bbb">☐ ☆</span><span style="width:120px">${who}</span><span style="flex:1;white-space:nowrap;overflow:hidden">${subj}<span style="font-weight:400;color:#666"> - ${prev}</span></span><span style="font-size:12px">${when}</span></div>`).join('')}
  </div>
</div></body></html>`;

// ---------- 8. an Explorer error ----------
const dlgHTML = `<!doctype html><html><head><meta charset="utf-8"><style>${base}</style></head><body style="background:#fff;border:1px solid #888">
${titlebar('<span style="position:absolute;left:12px">네트워크 드라이브 연결</span>', { bg: '#fff' })}
<div style="display:flex;gap:16px;padding:22px 24px 0">
  <div style="width:36px;height:36px;border-radius:50%;background:#c42b1c;color:#fff;display:grid;place-items:center;font-size:20px;font-weight:700;flex:none">✕</div>
  <div><div style="font-size:15px;color:#003399;margin-bottom:8px">\\\\NAS\\photos 에 연결할 수 없습니다.</div><div style="font-size:13px;color:#333;line-height:1.5">네트워크 경로를 찾을 수 없습니다.<br>오류 코드: 0x80070035</div></div>
</div>
<div style="position:absolute;left:0;right:0;bottom:0;height:52px;background:#f3f3f3;display:flex;justify-content:flex-end;align-items:center;gap:8px;padding:0 14px">
  <div style="border:1px solid #d0d0d0;background:#fff;padding:5px 18px;font-size:13px;border-radius:4px">진단</div><div style="border:1px solid #005fb8;background:#005fb8;color:#fff;padding:5px 24px;font-size:13px;border-radius:4px">확인</div>
</div></body></html>`;

// ---------- render the scenes ----------
const browser = await chromium.launch();
// 집 PC runs at 100 % scaling, the 회사 PC laptop at 150 %
async function render(name, html, width, height, scale = 1) {
  const p = await browser.newPage({ viewport: { width, height }, deviceScaleFactor: scale });
  await p.setContent(html, { waitUntil: 'load' });
  await p.waitForTimeout(150);
  await p.screenshot({ path: path.join(scenes, name) });
  await p.close();
  return path.join(scenes, name);
}
const S = {
  ppt: await render('ppt.png', pptHTML, 1600, 900),
  excel: await render('excel.png', excelHTML, 760, 400),
  pdf: penmit ? await render('pdf.png', browserHTML, 1440, 880) : null,
  chat: await render('chat.png', chatHTML, 380, 640, 1.5),
  code: await render('code.png', codeHTML, 1500, 860),
  ps: await render('ps.png', psHTML, 940, 150),
  mail: await render('mail.png', mailHTML, 1180, 420, 1.5),
  dlg: await render('dlg.png', dlgHTML, 470, 190),
};

// ---------- the app (dev build) with a fake Drive ----------
const cfg = fs.mkdtempSync('/tmp/smcfg-');
const folder = fs.mkdtempSync('/tmp/smshots-');
const siteDir = path.join(out, 'site');
const srv = http.createServer((req, res) => {
  let p = decodeURIComponent(new URL(req.url, 'http://x').pathname).replace(/^\/Screenmit/, '');
  if (p.endsWith('/')) p += 'index.html';
  const f = path.join(siteDir, p);
  if (!f.startsWith(siteDir) || !fs.existsSync(f)) { res.writeHead(404); res.end(); return; }
  const types = { '.html': 'text/html; charset=utf-8', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.png': 'image/png', '.webmanifest': 'application/manifest+json' };
  res.writeHead(200, { 'Content-Type': types[path.extname(f)] || 'application/octet-stream' });
  res.end(fs.readFileSync(f));
});
await new Promise((r) => srv.listen(0, '127.0.0.1', r));
const site = `http://127.0.0.1:${srv.address().port}/Screenmit/`;
const proc = spawn(bin, ['--config', cfg, '--folder', folder, '--port', '28300', '--dev-fake-drive', '--dev-pc', 'DESKTOP-JH'],
  { stdio: ['ignore', 'pipe', 'pipe'], env: { ...process.env, SCREENMIT_WEB_URL: site } });
const { gallery, fake } = await new Promise((resolve, reject) => {
  let buf = '';
  proc.stdout.on('data', (d) => {
    buf += d;
    const g = buf.match(/GALLERY (\S+)/); const f = buf.match(/FAKEDRIVE (\S+)/);
    if (g && f) resolve({ gallery: g[1], fake: f[1] });
  });
  setTimeout(() => reject(new Error('no start')), 10000);
});
const origin = new URL(gallery).origin;
const H = { 'X-Screenmit': '1' };
const at = (n, hh, mm, ss = 7) => { const d = new Date(); d.setDate(d.getDate() - n); d.setHours(hh, mm, ss, 0); return d.toISOString(); };

const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 1.25, locale: 'ko-KR', timezoneId: 'Asia/Seoul' });
const page = await ctx.newPage();
await page.goto(gallery);
await page.waitForSelector('.empty h2');
const post = async (route, q, file) => {
  const r = await page.request.post(`${origin}${route}?${new URLSearchParams(q)}`, { headers: H, data: fs.readFileSync(file) });
  if (r.status() !== 200) throw new Error(`${route} ${r.status()} ${await r.text()}`);
  return r.json().catch(() => ({}));
};

// connect Drive as "집 PC" through the settings, like a person would
const clientFile = path.join(out, 'client.json');
fs.writeFileSync(clientFile, await (await page.request.get(`${origin}/api/dev/fake-client`)).text());
await page.click('#settingsBtn');
await page.setInputFiles('#jsonFile', clientFile);
await page.waitForSelector('#pcNameInput');
await page.click('[data-pcname="집 PC"]');
await page.click('#connectBtn');
await page.waitForFunction(() => /맞춤/.test(document.querySelector('#driveGroup').textContent), null, { timeout: 15000 });
await page.click('#settings [data-close]');

// this PC (집 PC)
await post('/api/dev/capture', { app: 'Microsoft PowerPoint', exe: 'POWERPNT.EXE', title: 'CV04 정리.pptx - PowerPoint', time: at(0, 14, 3, 12) }, S.ppt);
await post('/api/dev/capture', { app: 'Microsoft Excel', exe: 'EXCEL.EXE', title: '제주 여행 경비.xlsx - Excel', time: at(0, 11, 42) }, S.excel);
if (S.pdf) await post('/api/dev/capture', { app: 'Google Chrome', exe: 'chrome.exe', title: 'CV04_edge_detection.pdf - Penmit PDF - Chrome', time: at(0, 9, 15) }, S.pdf);
await post('/api/dev/capture', { app: 'Visual Studio Code', exe: 'Code.exe', title: 'sync.go - screenmit - Visual Studio Code', time: at(1, 23, 10) }, S.code);
await post('/api/dev/capture', { app: 'Windows PowerShell', exe: 'powershell.exe', title: 'Windows PowerShell', time: at(1, 22, 47) }, S.ps);
await post('/api/dev/capture', { app: 'Windows 탐색기', exe: 'explorer.exe', title: '네트워크 드라이브 연결', time: at(12, 19, 30) }, S.dlg);
// the other PC (회사 PC), through Drive
await post('/api/dev/remote', { pc: '회사 PC', app: 'KakaoTalk', title: '민수', time: at(0, 12, 18) }, S.chat);
await post('/api/dev/remote', { pc: '회사 PC', app: 'Chrome', title: '받은편지함 - 메일', time: at(1, 8, 33) }, S.mail);
for (let i = 0; i < 20; i++) { await page.request.post(`${origin}/api/drive/sync`, { headers: H }); const st = await (await page.request.get(`${origin}/api/state`)).json(); if (st.shots.length >= (S.pdf ? 8 : 7)) break; await page.waitForTimeout(500); }

// keep the expense sheet, a memo on the slide
const st = await (await page.request.get(`${origin}/api/state`)).json();
const id = (app) => st.shots.find((x) => x.app === app).id;
const patch = (sid, data) => page.request.fetch(`${origin}/api/shots/${sid}`, { method: 'PATCH', headers: { ...H, 'Content-Type': 'application/json' }, data });
await patch(id('Excel'), { pinned: true });
await patch(id('PowerPoint'), { note: 'Sobel 비교 — 시험 범위' });
await page.reload();
await page.waitForFunction((n) => document.querySelectorAll('.card').length === n, S.pdf ? 8 : 7, { timeout: 10000 });
await page.mouse.move(0, 0);
await page.waitForTimeout(1200);
await page.screenshot({ path: path.join(docs, 'gallery.png') });
// a narrower window for the personal site, so the thumbnails read at card size
await page.setViewportSize({ width: 1060, height: 700 });
await page.waitForTimeout(600);
await page.screenshot({ path: path.join(out, 'site-screenmit.jpg'), type: 'jpeg', quality: 88 });
await page.setViewportSize({ width: 1280, height: 800 });
await page.waitForTimeout(300);

// viewer
await page.click(`.card[data-id="${id('PowerPoint')}"]`);
await page.waitForFunction(() => document.querySelector('#vImg').complete && document.querySelector('#vImg').naturalWidth > 0);
await page.waitForTimeout(400);
await page.screenshot({ path: path.join(docs, 'viewer.png') });
await page.keyboard.press('Escape');

// settings
await page.click('#settingsBtn');
await page.waitForSelector('#settings:not([hidden])');
await page.waitForTimeout(300);
await page.screenshot({ path: path.join(docs, 'settings.png') });
await page.click('#settings [data-close]');

// the web page on a phone, against the same fake Drive
execFileSync('python3', [path.join(repo, 'tools/build_site.py'), siteDir, '--client-id', 'docs.apps.googleusercontent.com', '--api-base', fake, '--auth-url', `${fake}/o/oauth2/auth`]);
const phoneCtx = await browser.newContext({ ...devices['iPhone 13'], deviceScaleFactor: 2, locale: 'ko-KR', timezoneId: 'Asia/Seoul' });
const phone = await phoneCtx.newPage();
await phone.goto(site);
await phone.click('#loginBtn');
await phone.waitForFunction(() => document.querySelectorAll('.card').length >= 3, null, { timeout: 15000 });
await phone.waitForFunction(() => [...document.querySelectorAll('.card img')].slice(0, 4).every((i) => i.src.startsWith('blob:') && i.complete), null, { timeout: 15000 });
await phone.waitForTimeout(800);
await phone.screenshot({ path: path.join(docs, 'web-phone.png') });
await phone.tap(`.card:has-text("KakaoTalk")`);
await phone.waitForFunction(() => document.querySelector('#vImg').src.startsWith('blob:') && document.querySelector('#vLoading').hidden, null, { timeout: 15000 });
await phone.waitForTimeout(600);
await phone.screenshot({ path: path.join(docs, 'web-phone-viewer.png') });

await browser.close();
proc.kill();
srv.close();
console.log('DOCS OK', out);
