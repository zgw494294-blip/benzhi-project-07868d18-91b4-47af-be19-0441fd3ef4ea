package httpapi

import (
	"bytes"
	"net/http"
)

// muxDefaultNotFoundBody is the body written by net/http.ServeMux when no
// registered pattern matches the request path.
var muxDefaultNotFoundBody = []byte("404 page not found\n")

// muxDefaultMethodNotAllowedBody is the body written by net/http.ServeMux when
// a path matches but the request method is not registered for that path.
var muxDefaultMethodNotAllowedBody = []byte("Method Not Allowed\n")

// notFoundRewrite wraps a handler so that the default plain-text 405 and 404
// responses produced by net/http.ServeMux are rendered through the unified
// error envelope. The HTTP status code and the Allow header (for 405) are
// preserved.
func notFoundRewrite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := &deferredResponseWriter{header: http.Header{}}
		next.ServeHTTP(buf, r)
		buf.flush(w, r)
	})
}

// deferredResponseWriter buffers a handler's response so the caller can decide
// whether to rewrite the body before anything is committed to the underlying
// writer. It owns its own header map so that the mux's plain-text Content-Type
// can be discarded when rewriting to JSON.
type deferredResponseWriter struct {
	header    http.Header
	status    int
	wroteHead bool
	body      bytes.Buffer
}

func (d *deferredResponseWriter) Header() http.Header {
	if d.header == nil {
		d.header = http.Header{}
	}
	return d.header
}

func (d *deferredResponseWriter) WriteHeader(status int) {
	if d.wroteHead {
		return
	}
	d.wroteHead = true
	d.status = status
}

func (d *deferredResponseWriter) Write(data []byte) (int, error) {
	if !d.wroteHead {
		d.WriteHeader(http.StatusOK)
	}
	return d.body.Write(data)
}

// flush commits the buffered response to w, rewriting the mux's default 405/404
// plain-text responses into the unified JSON error envelope.
func (d *deferredResponseWriter) flush(w http.ResponseWriter, r *http.Request) {
	status := d.status
	if status == 0 {
		status = http.StatusOK
	}
	body := d.body.Bytes()
	ct := d.header.Get("Content-Type")

	plainDefault := ct == "" || ct == "text/plain; charset=utf-8" || ct == "text/plain"
	trim := bytes.TrimSpace(body)
	notAllowedDefault := bytes.Equal(trim, bytes.TrimSpace(muxDefaultMethodNotAllowedBody))
	notFoundDefault := bytes.Equal(trim, bytes.TrimSpace(muxDefaultNotFoundBody))

	if status == http.StatusMethodNotAllowed && plainDefault && notAllowedDefault {
		if allow := d.header.Get("Allow"); allow != "" {
			w.Header().Set("Allow", allow)
		}
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "不支持的请求方法")
		return
	}
	if status == http.StatusNotFound && plainDefault && notFoundDefault {
		writeError(w, r, http.StatusNotFound, "not_found", "路径不存在")
		return
	}

	copyHeader(w.Header(), d.header)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		// Avoid duplicated values; Replace the key on dst.
		dst[key] = append([]string(nil), values...)
	}
}
