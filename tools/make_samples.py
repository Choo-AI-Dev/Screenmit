"""Makes plausible screenshot PNGs (window captures) for tests and the README.

python3 tools/make_samples.py OUT_DIR
"""
import math
import os
import random
import sys

from PIL import Image, ImageDraw, ImageFilter, ImageFont

FONT = "/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc"
BOLD = "/usr/share/fonts/opentype/noto/NotoSansCJK-Bold.ttc"
MONO = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"


def f(size, bold=False):
    return ImageFont.truetype(BOLD if bold else FONT, size, index=1)  # index 1 = KR


def mono(size):
    try:
        return ImageFont.truetype(MONO, size)
    except OSError:
        return f(size)


def window(w, h, title, bg="#ffffff", bar="#f3f3f3", dark=False):
    im = Image.new("RGB", (w, h), bg)
    d = ImageDraw.Draw(im)
    d.rectangle([0, 0, w, 32], fill=bar)
    d.text((14, 7), title, font=f(13), fill="#e8e8e8" if dark else "#333333")
    for i, c in enumerate(["#e5e5e5", "#e5e5e5", "#e5e5e5"]):
        x = w - 46 * (3 - i)
        d.line([x + 18, 16, x + 28, 16], fill="#888", width=1) if i == 0 else None
    d.rectangle([w - 120, 10, w - 110, 20], outline="#888") if not dark else None
    d.line([w - 30, 11, w - 20, 21], fill="#888", width=1)
    d.line([w - 30, 21, w - 20, 11], fill="#888", width=1)
    return im, d


def ppt(path):
    im, d = window(1600, 900, "CV04 정리.pptx - PowerPoint", bar="#b7472a")
    d.text((14, 7), "CV04 정리.pptx - PowerPoint", font=f(13), fill="#fff")
    d.rectangle([0, 32, 1600, 140], fill="#f5f5f5")
    for i, t in enumerate(["파일", "홈", "삽입", "디자인", "전환", "애니메이션", "슬라이드 쇼", "검토", "보기"]):
        d.text((20 + i * 78, 40), t, font=f(14), fill="#333")
    d.rectangle([0, 140, 230, 900], fill="#e9e9e9")
    for i in range(5):
        y = 160 + i * 140
        d.rectangle([30, y, 210, y + 102], fill="#fff", outline="#c55a11" if i == 2 else "#ccc", width=3 if i == 2 else 1)
        d.text((12, y), str(i + 1), font=f(12), fill="#555")
        d.rectangle([44, y + 12, 160, y + 20], fill="#d0d7e5")
    d.rectangle([300, 170, 1540, 868], fill="#ffffff", outline="#d0d0d0")
    d.text((360, 210), "Sobel vs Prewitt: 가운데 가중치", font=f(38, True), fill="#1f2a44")
    d.line([360, 270, 1480, 270], fill="#c55a11", width=4)
    vals = [62, 48, 31, 19, 12]
    for i, v in enumerate(vals):
        x = 420 + i * 160
        d.rectangle([x, 760 - v * 7, x + 90, 760], fill="#3a6ef2" if i < 4 else "#c55a11")
        d.text((x + 20, 770), f"σ={i + 1}", font=f(18), fill="#333")
        d.text((x + 24, 730 - v * 7), f"{v}", font=f(18, True), fill="#1f2a44")
    d.text((1230, 320), "• Sobel: 가운데 ×2", font=f(22), fill="#333")
    d.text((1230, 360), "• Prewitt: 모두 같게", font=f(22), fill="#333")
    d.text((1230, 400), "• 노이즈에 덜 민감", font=f(22), fill="#333")
    im.save(path)


def excel(path):
    im, d = window(1500, 860, "제주 여행 경비.xlsx - Excel", bar="#217346")
    d.text((14, 7), "제주 여행 경비.xlsx - Excel", font=f(13), fill="#fff")
    d.rectangle([0, 32, 1500, 120], fill="#f3f3f3")
    cols = ["날짜", "항목", "결제", "금액", "1인당", "확인", "메모"]
    x0, y0, cw, rh = 50, 150, 190, 30
    for j, c in enumerate(cols):
        d.rectangle([x0 + j * cw, y0, x0 + (j + 1) * cw, y0 + rh], fill="#e7e6e6", outline="#c8c8c8")
        d.text((x0 + j * cw + 8, y0 + 4), c, font=f(15, True), fill="#222")
    rnd = random.Random(7)
    items = ["항공권", "렌터카", "숙소", "주유", "흑돼지", "카페", "시장", "입장료", "편의점", "택시"]
    for i in range(20):
        y = y0 + rh * (i + 1)
        amount = rnd.randint(8, 240) * 1000
        row = [f"11/{7 + i // 7}", items[i % len(items)], ["민수", "지영", "나"][i % 3], f"{amount:,}", f"{amount // 3:,}", "", ""]
        row[5] = "OK" if i % 6 else "확인"
        for j, c in enumerate(row):
            fill = "#fde9e7" if (j == 5 and c == "확인") else "#ffffff"
            d.rectangle([x0 + j * cw, y, x0 + (j + 1) * cw, y + rh], fill=fill, outline="#dadada")
            d.text((x0 + j * cw + 8, y + 5), c, font=f(15), fill="#c00000" if c == "확인" else "#222")
    d.rectangle([x0 + 4 * cw, y0 + rh * 6, x0 + 5 * cw, y0 + rh * 7], outline="#217346", width=3)
    im.save(path)


def kakao(path):
    im, d = window(520, 860, "민수", bg="#b2c7d9", bar="#a9bdce")
    msgs = [
        (False, "지난주에 보낸 캠핑장 예약 화면 아직 있어?"),
        (True, "잠깐만"),
        (True, "캡처해 둔 거 찾아볼게"),
        (False, "고마워 🙏"),
        (True, "이거 맞지? 토요일 B-12"),
        (False, "응 맞아! 이걸로 체크인하면 돼"),
    ]
    y = 60
    for me, t in msgs:
        tw = d.textlength(t, font=f(16)) + 28
        if me:
            d.rounded_rectangle([500 - tw, y, 500, y + 42], 12, fill="#fee500")
            d.text((500 - tw + 14, y + 9), t, font=f(16), fill="#222")
        else:
            d.ellipse([16, y, 56, y + 40], fill="#e1e8ef")
            d.rounded_rectangle([66, y, 66 + tw, y + 42], 12, fill="#fff")
            d.text((80, y + 9), t, font=f(16), fill="#222")
        y += 62
    d.rectangle([0, 760, 520, 860], fill="#fff")
    d.text((18, 790), "메시지 입력", font=f(15), fill="#aaa")
    im.save(path)


def photo(path):
    w, h = 1280, 960
    rnd = random.Random(3)
    sky = Image.new("RGB", (w, h))
    px = sky.load()
    for y in range(h):
        for x in range(w):
            t = y / h
            ridge = 520 + 60 * math.sin(x / 170.0) + 25 * math.sin(x / 53.0)
            if y < ridge:
                c = (int(120 + 90 * t), int(170 + 50 * t), int(225 - 20 * t))
            else:
                k = (y - ridge) / (h - ridge + 1)
                c = (int(60 + 40 * k), int(110 - 20 * k), int(70 - 10 * k))
            n = rnd.randint(-4, 4)
            px[x, y] = (c[0] + n, c[1] + n, c[2] + n)
    im = sky.filter(ImageFilter.GaussianBlur(1.2))
    d = ImageDraw.Draw(im)
    d.rectangle([0, 0, w, 34], fill="#f3f3f3")
    d.text((12, 6), "IMG_2041.jpg - 그림판", font=f(14), fill="#333")
    im.save(path)


def code(path):
    im, d = window(1500, 900, "sync.go - screenmit - Visual Studio Code", bg="#1e1e1e", bar="#323233", dark=True)
    d.rectangle([0, 32, 50, 900], fill="#333333")
    d.rectangle([50, 32, 300, 900], fill="#252526")
    for i, n in enumerate(["internal", "  drive", "    api.go", "    oauth.go", "    sync.go", "  shots", "    store.go", "web", "main.go"]):
        d.text((62, 50 + i * 24), n, font=mono(14), fill="#ffffff" if "sync" in n else "#cccccc")
    lines = [
        ("// SyncOnce does one round.", "#6a9955"),
        ("func (s *Syncer) SyncOnce(ctx context.Context) (more bool, err error) {", "#dcdcaa"),
        ("    email := s.API.Email()", "#9cdcfe"),
        ("    if err := s.ensureFolders(ctx); err != nil {", "#c586c0"),
        ("        return false, err", "#c586c0"),
        ("    }", "#d4d4d4"),
        ("    files, err := s.API.List(ctx, query)", "#9cdcfe"),
        ("    for _, f := range files {", "#c586c0"),
        ("        r := parseRemote(f)", "#9cdcfe"),
        ("        if !r.Pinned && shots.Expired(r.Time, now, days) {", "#c586c0"),
        ("            s.API.Delete(ctx, f.ID)", "#dcdcaa"),
        ("        }", "#d4d4d4"),
        ("    }", "#d4d4d4"),
    ]
    for i, (t, c) in enumerate(lines):
        d.text((320, 60 + i * 26), f"{i + 120:>4}", font=mono(15), fill="#858585")
        d.text((380, 60 + i * 26), t, font=mono(15), fill=c)
    d.rectangle([300, 860, 1500, 900], fill="#007acc")
    im.save(path)


def dialog(path):
    im = Image.new("RGB", (620, 260), "#ffffff")
    d = ImageDraw.Draw(im)
    d.rectangle([0, 0, 620, 34], fill="#f0f0f0")
    d.text((12, 8), "네트워크 드라이브 연결", font=f(13), fill="#333")
    d.ellipse([30, 70, 74, 114], fill="#e81123")
    d.text((45, 74), "×", font=f(30, True), fill="#fff")
    d.text((96, 70), "\\\\NAS\\photos 에 연결할 수 없습니다.", font=f(17, True), fill="#222")
    d.text((96, 104), "네트워크 경로를 찾을 수 없습니다. (0x80070035)", font=f(14), fill="#444")
    d.rectangle([0, 190, 620, 260], fill="#f0f0f0")
    d.rectangle([500, 208, 596, 240], fill="#e1e1e1", outline="#0078d7", width=2)
    d.text((534, 212), "확인", font=f(14), fill="#222")
    im.save(path)


def web(path):
    im, d = window(1440, 880, "CV04_edge_detection.pdf - Penmit PDF - Microsoft Edge", bar="#f7f7f7")
    d.rectangle([0, 32, 1440, 80], fill="#ffffff")
    d.rounded_rectangle([200, 42, 1100, 72], 14, fill="#f1f3f4")
    d.text((222, 47), "choo-ai-dev.github.io/penmit-pdf/", font=f(14), fill="#444")
    d.rectangle([0, 80, 1440, 880], fill="#e9ecef")
    d.rectangle([260, 100, 1180, 860], fill="#ffffff", outline="#ddd")
    d.text((320, 140), "2.1  Finite differences and the Sobel operator", font=f(24, True), fill="#111")
    for i in range(9):
        d.rectangle([320, 200 + i * 26, 1120 - (i % 3) * 60, 212 + i * 26], fill="#d9d9d9")
    d.rounded_rectangle([520, 300, 900, 318], 8, fill="#fff3a6")
    d.ellipse([700, 470, 860, 540], outline="#d0312d", width=3)
    d.text((440, 480), "★ 시험", font=f(22, True), fill="#d0312d")
    for r in range(3):
        for c in range(3):
            d.text((740 + c * 34, 600 + r * 34), str([-1, 0, 1][c] * [1, 2, 1][r]), font=f(18), fill="#111")
    im.save(path)


def mail(path):
    im, d = window(1200, 760, "받은편지함 - 메일", bar="#f3f6fc")
    d.rectangle([0, 32, 220, 760], fill="#f6f8fc")
    for i, t in enumerate(["받은편지함", "별표", "보낸편지함", "임시보관함"]):
        d.text((24, 60 + i * 34), t, font=f(15, i == 0), fill="#1f1f1f")
    rows = [("GitHub", "[Choo-AI-Dev/Screenmit] Release 0.2.1", "오전 8:02"), ("숲속캠핑장", "예약이 확정되었습니다 (11/21 B-12)", "어제"), ("민수", "제주 사진 모음", "11월 14일")]
    for i, (who, subj, when) in enumerate(rows):
        y = 60 + i * 52
        d.rectangle([240, y, 1180, y + 46], fill="#ffffff" if i else "#eaf1fb")
        d.text((260, y + 12), who, font=f(15, True), fill="#1f1f1f")
        d.text((420, y + 12), subj, font=f(15), fill="#333")
        d.text((1080, y + 12), when, font=f(13), fill="#666")
    im.save(path)


SAMPLES = {
    "ppt.png": ppt, "excel.png": excel, "kakao.png": kakao, "photo.png": photo,
    "code.png": code, "dialog.png": dialog, "web.png": web, "mail.png": mail,
}

if __name__ == "__main__":
    out = sys.argv[1]
    os.makedirs(out, exist_ok=True)
    for name, fn in SAMPLES.items():
        fn(os.path.join(out, name))
    print("ok", len(SAMPLES))
