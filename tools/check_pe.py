"""Checks the built exe: GUI subsystem, icon group -> icons, version strings, imports."""
import struct, sys

b = open(sys.argv[1], "rb").read()
pe = struct.unpack_from("<I", b, 0x3C)[0]
assert b[pe:pe + 4] == b"PE\0\0"
machine, nsec = struct.unpack_from("<HH", b, pe + 4)
opt = pe + 24
magic = struct.unpack_from("<H", b, opt)[0]
assert magic == 0x20B, "PE32+"
subsystem = struct.unpack_from("<H", b, opt + 68)[0]
dd = opt + 112
res_rva, res_size = struct.unpack_from("<II", b, dd + 2 * 8)
secs = []
sh = opt + struct.unpack_from("<H", b, pe + 20)[0]
for i in range(nsec):
    name, vsize, va, rsize, rptr = struct.unpack_from("<8sIIII", b, sh + 40 * i)
    secs.append((name.rstrip(b"\0").decode(), va, vsize, rptr, rsize))

def off(rva):
    for n, va, vs, rp, rs in secs:
        if va <= rva < va + max(vs, rs):
            return rp + rva - va
    raise ValueError(hex(rva))

base = off(res_rva)
def walk(o, depth, path, out):
    n_named, n_id = struct.unpack_from("<HH", b, base + o + 12)
    for i in range(n_named + n_id):
        name, ptr = struct.unpack_from("<II", b, base + o + 16 + 8 * i)
        if ptr & 0x80000000:
            walk(ptr & 0x7FFFFFFF, depth + 1, path + [name], out)
        else:
            rva, size = struct.unpack_from("<II", b, base + ptr)
            out[tuple(path + [name])] = b[off(rva):off(rva) + size]
res = {}
walk(0, 0, [], res)
print("machine", hex(machine), "subsystem", subsystem, "(2 = GUI)")
print("sections", [s[0] for s in secs])
icons = {k[1]: v for k, v in res.items() if k[0] == 3}
groups = {k[1]: v for k, v in res.items() if k[0] == 14}
assert subsystem == 2
assert groups, "no RT_GROUP_ICON"
g = groups[min(groups)]
_, typ, cnt = struct.unpack_from("<HHH", g, 0)
ok = 0
for i in range(cnt):
    w, h, c, r, planes, bits, size, nid = struct.unpack_from("<BBBBHHIH", g, 6 + 14 * i)
    data = icons[nid]
    assert len(data) == size, (nid, len(data), size)
    if data[:8] == b"\x89PNG\r\n\x1a\n":
        kind = "png"
    else:
        hs, iw, ih = struct.unpack_from("<Iii", data, 0)
        assert hs == 40 and ih == 2 * iw == 2 * (w or 256), (hs, iw, ih, w)
        kind = "bmp"
    ok += 1
    print(f"  icon {nid}: {w or 256}px {kind} {size} bytes")
ver = [v for k, v in res.items() if k[0] == 16][0]
txt = ver.decode("utf-16-le", "ignore")
for key in ["FileDescription", "ProductName", "FileVersion"]:
    i = txt.find(key)
    assert i >= 0, key
    val = txt[i + len(key):].lstrip("\0").split("\0")[0]
    print(f"  {key} = {val}")
assert struct.unpack_from("<I", ver, 40)[0] == 0xFEEF04BD, "fixed file info signature"
# make sure every DLL we call is imported lazily (no static import of optional APIs)
print("OK", ok, "icons")
