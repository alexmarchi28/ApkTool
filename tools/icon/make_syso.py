"""Write a COFF .syso (amd64) containing RT_ICON/RT_GROUP_ICON/RT_MANIFEST/RT_VERSION for the Go linker."""
import struct, sys

ico_path, out_path = sys.argv[1], sys.argv[2]

MANIFEST = b"""<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity version="1.0.0.0" processorArchitecture="amd64" name="ApkTool" type="win32"/>
  <description>ApkTool</description>
  <dependency><dependentAssembly>
    <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0"
      processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>
  </dependentAssembly></dependency>
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3"><security><requestedPrivileges>
    <requestedExecutionLevel level="asInvoker" uiAccess="false"/>
  </requestedPrivileges></security></trustInfo>
  <application xmlns="urn:schemas-microsoft-com:asm.v3"><windowsSettings>
    <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware>
    <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2</dpiAwareness>
  </windowsSettings></application>
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1"><application>
    <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>
  </application></compatibility>
</assembly>
"""

# ---- icon parsing
ico = open(ico_path, "rb").read()
_, typ, count = struct.unpack_from("<HHH", ico, 0)
assert typ == 1
icons, group = [], struct.pack("<HHH", 0, 1, count)
for i in range(count):
    w, h, cc, res, planes, bpp, size, off = struct.unpack_from("<BBBBHHII", ico, 6 + 16 * i)
    icons.append(ico[off:off + size])
    group += struct.pack("<BBBBHHIH", w, h, cc, res, planes, bpp, size, i + 1)

# ---- version info (VS_VERSIONINFO)
def vs_block(key, value=b"", children=b"", is_text=False, value_len=None):
    k = (key + "\0").encode("utf-16-le")
    hdr_len = 6 + len(k)
    pad1 = (4 - hdr_len % 4) % 4
    body = k + b"\0" * pad1 + value
    if children:
        body += b"\0" * ((4 - (6 + len(body)) % 4) % 4) + children
    total = 6 + len(body)
    vl = value_len if value_len is not None else len(value)
    return struct.pack("<HHH", total, vl, 1 if is_text else 0) + body

def align4(b): return b + b"\0" * ((4 - len(b) % 4) % 4)

def string_entry(k, v):
    val = (v + "\0").encode("utf-16-le")
    return align4(vs_block(k, val, is_text=True, value_len=len(v) + 1))

strings = {"CompanyName": "ApkTool", "FileDescription": "ApkTool - cambia package name ed etichetta APK",
           "FileVersion": "1.0.0.0", "InternalName": "ApkTool", "OriginalFilename": "ApkTool.exe",
           "ProductName": "ApkTool", "ProductVersion": "1.0.0.0"}
st = b"".join(string_entry(k, v) for k, v in strings.items())
string_table = align4(vs_block("041004B0", children=st, is_text=True))
sfi = align4(vs_block("StringFileInfo", children=string_table, is_text=True))
var = align4(vs_block("Translation", struct.pack("<HH", 0x0410, 0x04B0)))
vfi = align4(vs_block("VarFileInfo", children=var, is_text=True))
ffi = struct.pack("<13I", 0xFEEF04BD, 0x00010000, 0x00010000, 0, 0x00010000, 0, 0x3F, 0, 0x00040004, 1, 0, 0, 0)
version = vs_block("VS_VERSION_INFO", ffi, children=sfi + vfi)

LANG = 0x0410  # Italian
res = {3: [(i + 1, data) for i, data in enumerate(icons)],  # RT_ICON
       14: [(1, group)],                                    # RT_GROUP_ICON
       16: [(1, version)],                                  # RT_VERSION
       24: [(1, MANIFEST)]}                                 # RT_MANIFEST

# ---- layout the resource tree
types = sorted(res)
def dir_size(n): return 16 + 8 * n
off = dir_size(len(types))
type_dir_off = {}
for t in types:
    type_dir_off[t] = off; off += dir_size(len(res[t]))
name_dir_off = {}
for t in types:
    for nid, _ in res[t]:
        name_dir_off[(t, nid)] = off; off += dir_size(1)
data_entry_off = {}
for t in types:
    for nid, _ in res[t]:
        data_entry_off[(t, nid)] = off; off += 16
raw_off = {}
for t in types:
    for nid, data in res[t]:
        off = (off + 7) & ~7
        raw_off[(t, nid)] = off; off += len(data)
total = (off + 7) & ~7

sec = bytearray(total)
relocs = []
def put_dir(at, entries):
    struct.pack_into("<IIHHHH", sec, at, 0, 0, 0, 0, 0, len(entries))
    for i, (ident, target, is_dir) in enumerate(entries):
        struct.pack_into("<II", sec, at + 16 + 8 * i, ident, target | (0x80000000 if is_dir else 0))

put_dir(0, [(t, type_dir_off[t], True) for t in types])
for t in types:
    put_dir(type_dir_off[t], [(nid, name_dir_off[(t, nid)], True) for nid, _ in res[t]])
    for nid, data in res[t]:
        put_dir(name_dir_off[(t, nid)], [(LANG, data_entry_off[(t, nid)], False)])
        de = data_entry_off[(t, nid)]
        struct.pack_into("<IIII", sec, de, raw_off[(t, nid)], len(data), 0, 0)
        relocs.append(de)  # OffsetToData needs an RVA relocation
        sec[raw_off[(t, nid)]:raw_off[(t, nid)] + len(data)] = data

# ---- COFF object
hdr_size, sh_size = 20, 40
p_raw = hdr_size + sh_size
p_rel = p_raw + len(sec)
p_sym = p_rel + 10 * len(relocs)
out = bytearray()
out += struct.pack("<HHIIIHH", 0x8664, 1, 0, p_sym, 1, 0, 0)
out += struct.pack("<8sIIIIIIHHI", b".rsrc", 0, 0, len(sec), p_raw, p_rel, 0, len(relocs), 0, 0x40000040)
out += sec
for r in relocs:
    out += struct.pack("<IIH", r, 0, 3)  # IMAGE_REL_AMD64_ADDR32NB
out += struct.pack("<8sIhHBB", b".rsrc", 0, 1, 0, 3, 0)
out += struct.pack("<I", 4)
open(out_path, "wb").write(out)
print("syso", len(out), "bytes,", len(relocs), "resources")
