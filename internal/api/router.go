package api

import (
	"net/http"
	"strings"

	"github.com/hilmoo/shippingking/internal/wilayah"
)

type Server struct {
	store  *wilayah.Store
	apiKey string
}

// NewHandler builds the full HTTP handler. apiKey may be empty.
func NewHandler(store *wilayah.Store, apiKey string) http.Handler {
	s := &Server{store: store, apiKey: apiKey}
	mux := http.NewServeMux()

	// route registers h for each method plus a method-less fallback, so wrong
	// methods get a JSON 405 instead of the mux's plain-text one.
	route := func(path string, h http.HandlerFunc, methods ...string) {
		for _, m := range methods {
			mux.HandleFunc(m+" "+path, h)
		}
		allow := strings.Join(methods, ", ")
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Allow", allow)
			fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		})
	}

	const p = "/api/v1"
	route(p+"/destination/province", s.provinces, "GET")
	route(p+"/destination/city/{province_id}", s.cities, "GET")
	route(p+"/destination/district/{city_id}", s.districts, "GET")
	route(p+"/destination/sub-district/{district_id}", s.subdistricts, "GET")
	route(p+"/destination/domestic-destination", s.domesticDestination, "GET", "POST")
	route(p+"/calculate/domestic-cost", s.costSubdistrict, "POST")
	route(p+"/calculate/district/domestic-cost", s.costDistrict, "POST")

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fail(w, http.StatusNotFound, "Not Found")
	})

	// Only /api/ requires a key; /healthz and the HTML 404 page stay public.
	top := http.NewServeMux()
	top.Handle("/api/", requireKey(s.apiKey, mux))
	top.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	top.HandleFunc("/", notFoundPage)

	return logging(recoverer(top))
}
