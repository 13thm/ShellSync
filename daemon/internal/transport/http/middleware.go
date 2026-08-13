package http

import (
	"context"
	"net/http"
	"runtime/debug"

	"github.com/shellsync/daemon/internal/auth"
)

type ctxKey string

const userCtxKey ctxKey = "user"

// userIDFromContext returns the authenticated user id, if any.
func userIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userCtxKey).(string); ok {
		return v
	}
	return ""
}

// authMiddleware rejects requests without a valid bearer token. The local
// lock token and paired device session tokens are both accepted.
func authMiddleware(v *auth.Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := bearerToken(r)
			uid, ok := v.Verify(r.Context(), tok)
			if !ok {
				failCode(w, codeUnauthorized, http.StatusUnauthorized, "unauthorized")
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), userCtxKey, uid))
			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const pfx = "Bearer "
	if len(h) > len(pfx) && (h[:len(pfx)] == pfx) {
		return h[len(pfx):]
	}
	return ""
}

// recoverMiddleware catches panics and returns a 500 instead of crashing.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				debug.PrintStack()
				failCode(w, codeInternal, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
