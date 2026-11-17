"""Builds the Windows resources from the rendered icon PNGs:

  res/screenmit.ico, res/screenmit-paused.ico   tray icons (embedded with go:embed)
  rsrc_windows_amd64.syso                        exe icon + version info, linked by `go build`

python3 tools/mkres.py PNG_DIR VERSION      (PNG_DIR from tools/render_icons.mjs)
python3 tools/mkres.py --syso-only VERSION  (reuse res/screenmit.ico, new version)

The .syso is a COFF object with one .rsrc section; data entries point at
their data through IMAGE_REL_AMD64_ADDR32NB relocations (what rsrc and
go-winres emit too).
"""
import io
import struct
import sys

SIZES = [16, 20, 24, 32, 40, 48, 64, 256]


def images_from_ico(path):
    b = open(path, "rb").read()
    n = struct.unpack_from("<H", b, 4)[0]
    out = []
    for i in range(n):
        w, h, c, r, planes, bits, size, off = struct.unpack_from("<BBBBHHII", b, 6 + 16 * i)
        out.append((w or 256, b[off:off + size]))
    return out


def bmp_entry(img):
    """32-bit BGRA DIB with doubled height and an all-zero AND mask."""
    w, h = img.size
    img = img.convert("RGBA")
    px = img.tobytes()
    xor = bytearray()
    for y in range(h - 1, -1, -1):
        row = px[y * w * 4:(y + 1) * w * 4]
        for x in range(w):
            r, g, b, a = row[4 * x:4 * x + 4]
            xor += bytes((b, g, r, a))
    mask_stride = ((w + 31) // 32) * 4
    andmask = bytes(mask_stride * h)
    hdr = struct.pack("<IiiHHIIiiII", 40, w, h * 2, 1, 32, 0, len(xor) + len(andmask), 0, 0, 0, 0)
    return hdr + bytes(xor) + andmask


def png_entry(img):
    b = io.BytesIO()
    img.save(b, "PNG", optimize=True)
    return b.getvalue()


def icon_images(pngdir, variant):
    from PIL import Image  # only needed when rendering from PNGs

    out = []
    for s in SIZES:
        img = Image.open(f"{pngdir}/icon{variant}-{s}.png").convert("RGBA")
        data = png_entry(img) if s >= 256 else bmp_entry(img)
        out.append((s, data))
    return out


def ico_file(images):
    n = len(images)
    head = struct.pack("<HHH", 0, 1, n)
    off = 6 + 16 * n
    dirs, blobs = b"", b""
    for s, data in images:
        dirs += struct.pack("<BBBBHHII", s % 256, s % 256, 0, 0, 1, 32, len(data), off + len(blobs))
        blobs += data
    return head + dirs + blobs


def group_icon(images, first_id):
    out = struct.pack("<HHH", 0, 1, len(images))
    for i, (s, data) in enumerate(images):
        out += struct.pack("<BBBBHHIH", s % 256, s % 256, 0, 0, 1, 32, len(data), first_id + i)
    return out


# ---------- VS_VERSIONINFO ----------

def pad4(b):
    return b + b"\0" * (-len(b) % 4)


def block(key, value=b"", text=False, children=(), value_len=None):
    k = (key + "\0").encode("utf-16-le")
    body = struct.pack("<HHH", 0, 0, 1 if text else 0) + k
    body = pad4(body) + value
    if children:
        body = pad4(body)
        for c in children[:-1]:
            body += pad4(c)
        body += children[-1]
    vl = value_len if value_len is not None else len(value)
    return struct.pack("<HHH", len(body), vl, 1 if text else 0) + body[6:]


def string(key, val):
    v = (val + "\0").encode("utf-16-le")
    return block(key, v, text=True, value_len=len(val) + 1)


def version_info(ver):
    parts = [int(x) for x in (ver.split(".") + ["0", "0", "0"])[:4]]
    ms, ls = parts[0] << 16 | parts[1], parts[2] << 16 | parts[3]
    fixed = struct.pack("<13I", 0xFEEF04BD, 0x00010000, ms, ls, ms, ls, 0x3F, 0, 0x40004, 1, 0, 0, 0)
    strings = [
        string("CompanyName", "Choo-AI-Dev"),
        string("FileDescription", "Screenmit"),
        string("FileVersion", ver),
        string("InternalName", "Screenmit"),
        string("LegalCopyright", "Copyright (c) 2026 Choo-AI-Dev"),
        string("OriginalFilename", "Screenmit.exe"),
        string("ProductName", "Screenmit"),
        string("ProductVersion", ver),
    ]
    table = block("041204b0", text=True, children=strings)
    sfi = block("StringFileInfo", text=True, children=[table])
    var = block("Translation", struct.pack("<HH", 0x0412, 0x04B0))
    vfi = block("VarFileInfo", text=True, children=[var])
    return block("VS_VERSION_INFO", fixed, children=[sfi, vfi])


# ---------- COFF .rsrc ----------

def rsrc_section(resources):
    """resources: {type_id: {name_id: (lang, bytes)}} -> (section bytes, reloc offsets)."""
    types = sorted(resources)
    out = bytearray()

    def dir_header(n):
        return struct.pack("<IIHHHH", 0, 0, 0, 0, 0, n)

    # sizes to lay out: root, type dirs, name dirs (lang), data entries, data
    root_size = 16 + 8 * len(types)
    type_dirs, name_dirs = [], []
    off = root_size
    for t in types:
        type_dirs.append(off)
        off += 16 + 8 * len(resources[t])
    for t in types:
        for n in sorted(resources[t]):
            name_dirs.append((t, n, off))
            off += 16 + 8
    entries_at = off
    n_items = len(name_dirs)
    off += 16 * n_items
    data_at = []
    blobs = bytearray()
    for t, n, _ in name_dirs:
        while (off + len(blobs)) % 8:
            blobs += b"\0"
        data_at.append(off + len(blobs))
        blobs += resources[t][n][1]

    out += dir_header(len(types))
    for t, td in zip(types, type_dirs):
        out += struct.pack("<II", t, 0x80000000 | td)
    i = 0
    for t in types:
        out += dir_header(len(resources[t]))
        for n in sorted(resources[t]):
            out += struct.pack("<II", n, 0x80000000 | name_dirs[i][2])
            i += 1
    for k, (t, n, _) in enumerate(name_dirs):
        lang = resources[t][n][0]
        out += dir_header(1)
        out += struct.pack("<II", lang, entries_at + 16 * k)  # leaf: no high bit
    relocs = []
    for k, (t, n, _) in enumerate(name_dirs):
        relocs.append(len(out))
        out += struct.pack("<IIII", data_at[k], len(resources[t][n][1]), 0, 0)
    assert len(out) == entries_at + 16 * n_items
    out += blobs
    return bytes(out), relocs


def coff(section, relocs):
    IMAGE_FILE_MACHINE_AMD64 = 0x8664
    IMAGE_REL_AMD64_ADDR32NB = 3
    hdr_size, sec_size = 20, 40
    raw_at = hdr_size + sec_size
    reloc_at = raw_at + len(section)
    sym_at = reloc_at + 10 * len(relocs)
    out = struct.pack("<HHIIIHH", IMAGE_FILE_MACHINE_AMD64, 1, 0, sym_at, 1, 0, 0)
    out += struct.pack("<8sIIIIIIHHI", b".rsrc", 0, 0, len(section), raw_at, reloc_at, 0, len(relocs), 0, 0x40000040)
    out += section
    for r in relocs:
        out += struct.pack("<IIH", r, 0, IMAGE_REL_AMD64_ADDR32NB)
    out += struct.pack("<8sIhHBB", b".rsrc", 0, 1, 0, 3, 0)  # static section symbol
    out += struct.pack("<I", 4)  # empty string table
    return out


def main():
    if sys.argv[1] == "--syso-only":  # CI: new version number, same icons
        ver = sys.argv[2]
        normal = images_from_ico("res/screenmit.ico")
    else:
        pngdir, ver = sys.argv[1], sys.argv[2]
        normal, paused = icon_images(pngdir, ""), icon_images(pngdir, "-paused")
        open("res/screenmit.ico", "wb").write(ico_file(normal))
        open("res/screenmit-paused.ico", "wb").write(ico_file(paused))
    res = {3: {}, 14: {}, 16: {}}
    for i, (s, data) in enumerate(normal):
        res[3][i + 1] = (0x0409, data)
    res[14][1] = (0x0409, group_icon(normal, 1))
    res[16][1] = (0x0412, version_info(ver))
    sec, relocs = rsrc_section(res)
    open("rsrc_windows_amd64.syso", "wb").write(coff(sec, relocs))
    print("ok", len(sec), "bytes of resources")


if __name__ == "__main__":
    main()
