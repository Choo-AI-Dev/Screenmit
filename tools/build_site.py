"""Builds the GitHub Pages site (the gallery reading Google Drive directly).

python3 tools/build_site.py OUT_DIR [--client-id ID] [--site-url URL] [--version V] [--api-base URL --auth-url URL]

It reuses web/static (the same screen as the PC app) and adds site/drive.js
as its data source. --api-base/--auth-url point it at a fake Google in tests.
"""
import argparse
import json
import os
import re
import shutil
from urllib.parse import urlparse

ap = argparse.ArgumentParser()
ap.add_argument("out")
ap.add_argument("--client-id", default="")
ap.add_argument("--version", default="")
ap.add_argument("--api-base", default="")
ap.add_argument("--auth-url", default="")
ap.add_argument("--site-url", default="", help="canonical address, used as the Google redirect URI")
a = ap.parse_args()

root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
out = a.out
if os.path.exists(out):
    shutil.rmtree(out)
os.makedirs(os.path.join(out, "assets"))

for name in os.listdir(os.path.join(root, "web", "static")):
    if name != "index.html":
        shutil.copy(os.path.join(root, "web", "static", name), os.path.join(out, "assets", name))
for name in os.listdir(os.path.join(root, "site")):
    src = os.path.join(root, "site", name)
    dst = os.path.join(out, "assets" if name.endswith(".js") else "", name)
    shutil.copy(src, dst)

version = a.version or open(os.path.join(root, "VERSION")).read().strip()
cfg = {"clientId": a.client_id.strip(), "version": version}
if a.site_url:
    cfg["siteUrl"] = a.site_url
if a.api_base:
    cfg["apiBase"] = a.api_base
if a.auth_url:
    cfg["authUrl"] = a.auth_url
if a.api_base:  # tests: sign-out goes to the same fake Google
    cfg["revokeUrl"] = a.api_base.rstrip("/") + "/revoke"
with open(os.path.join(out, "assets", "config.js"), "w") as f:
    f.write("window.SCREENMIT_CONFIG = %s;\n" % json.dumps(cfg))

# 127.0.0.1: the Screenmit running on this PC (shown when Google is not there)
connect = ["'self'", "blob:", "https://www.googleapis.com", "https://oauth2.googleapis.com", "http://127.0.0.1:*"]
if a.api_base:
    u = urlparse(a.api_base)
    connect.append(f"{u.scheme}://{u.netloc}")
csp = ("default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' blob: data:; "
       f"connect-src {' '.join(connect)}; base-uri 'none'; form-action 'none'; object-src 'none'")

html = open(os.path.join(root, "web", "static", "index.html"), encoding="utf-8").read()
html = html.replace('href="/assets/', 'href="assets/').replace('src="/assets/', 'src="assets/')
head = (f'<meta http-equiv="Content-Security-Policy" content="{csp}">\n'
        '<meta name="theme-color" content="#3a6ef2">\n'
        '<meta name="apple-mobile-web-app-capable" content="yes">\n'
        '<meta name="apple-mobile-web-app-title" content="Screenmit">\n'
        '<link rel="manifest" href="manifest.webmanifest">\n'
        '<link rel="apple-touch-icon" href="apple-touch-icon.png">\n'
        '<script src="assets/config.js" defer></script>\n'
        '<script src="assets/drive.js" defer></script>\n')
html, n = re.subn(r'(<script src="assets/app\.js" defer></script>)', head + r'\1', html)
assert n == 1, "app.js script tag not found"
with open(os.path.join(out, "index.html"), "w", encoding="utf-8") as f:
    f.write(html)
open(os.path.join(out, ".nojekyll"), "w").close()
print("site ->", out, "client id:", "set" if cfg["clientId"] else "none (asked on the page)")
