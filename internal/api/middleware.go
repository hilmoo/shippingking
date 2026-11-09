package api

import (
	"crypto/subtle"
	"log"
	"net/http"
	"time"
)

// requireKey checks the `key` header. If apiKey is empty any non-empty key passes.
func requireKey(apiKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k := r.Header.Get("key") // Header.Get canonicalizes, so "Key:" works too
		valid := k != ""
		if valid && apiKey != "" {
			valid = subtle.ConstantTimeCompare([]byte(k), []byte(apiKey)) == 1
		}
		if !valid {
			fail(w, http.StatusUnauthorized, "Invalid Api key, key not found")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v", rec)
				fail(w, http.StatusInternalServerError, "Internal Server Error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (s *statusRecorder) WriteHeader(c int) {
	s.code = c
	s.ResponseWriter.WriteHeader(c)
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.RequestURI(), rec.code, time.Since(start).Round(time.Microsecond))
	})
}
