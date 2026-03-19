package middleware

import "net/http"

// SecurityHeaders adds defensive HTTP response headers to every response.
//
// Header policy:
//   - Content-Security-Policy: restricts resource origins; nonce-based inline
//     scripts are not yet supported — tighten script-src when the portal
//     migrates away from inline scripts.
//   - Strict-Transport-Security: 2-year max-age, applied to subdomains.
//     NOTE: only meaningful when the server is reached over HTTPS.  The
//     middleware sets it unconditionally so staging / production environments
//     benefit automatically without a separate build flag.
//   - X-Frame-Options: DENY — prevents clickjacking in older browsers that
//     predate the frame-ancestors CSP directive.
//   - X-Content-Type-Options: nosniff — prevents MIME sniffing attacks.
//   - Referrer-Policy: strict-origin-when-cross-origin — sends full path on
//     same-origin requests, origin only on cross-origin HTTPS, nothing on
//     cross-origin HTTP downgrades.
//   - Permissions-Policy: opts out of all sensor / media APIs that the
//     AgentFabric portal does not use.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()

		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self'; "+
				"style-src 'self' 'unsafe-inline'; "+ // Tailwind / CSS-in-JS still needs unsafe-inline
				"img-src 'self' data: blob:; "+
				"font-src 'self' data:; "+
				"connect-src 'self' wss:; "+ // WebSocket live stream
				"frame-ancestors 'none'; "+
				"object-src 'none'; "+
				"base-uri 'self'",
		)

		// 63 072 000 s = 2 years
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")

		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

		h.Set("Permissions-Policy",
			"accelerometer=(), "+
				"ambient-light-sensor=(), "+
				"camera=(), "+
				"geolocation=(), "+
				"gyroscope=(), "+
				"magnetometer=(), "+
				"microphone=(), "+
				"payment=(), "+
				"usb=()",
		)

		next.ServeHTTP(w, r)
	})
}
