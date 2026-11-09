package api

import (
	"net/http"
)

const notFoundHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>404 - ShippingKing</title>
<style>
  body{font-family:system-ui,sans-serif;max-width:36rem;margin:12vh auto;padding:0 1rem;line-height:1.6;color:#1f2328}
  h1{font-size:4rem;margin:0}
  code{background:#f0f2f4;padding:.1rem .35rem;border-radius:4px}
  a{color:#0969da}
  @media (prefers-color-scheme:dark){body{background:#0d1117;color:#e6edf3}code{background:#21262d}a{color:#58a6ff}}
</style>
</head>
<body>
<h1>404</h1>
<p>Nothing here. This is <strong>ShippingKing</strong>, a local mock of the RajaOngkir (Komerce) shipping cost API.
The API lives under <code>/api/v1</code> and needs a <code>key</code> header.</p>
<p>Source code: <a href="https://github.com/hilmoo/shippingking">github.com/hilmoo/shippingking</a></p>
<p>Built by Hilmi &mdash; more at <a href="https://hilmo.dev">hilmo.dev</a></p>
</body>
</html>
`

func notFoundPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	if r.Method != http.MethodHead {
		_, _ = w.Write([]byte(notFoundHTML))
	}
}
