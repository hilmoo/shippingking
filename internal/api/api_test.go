package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/hilmoo/shippingking/internal/wilayah"
)

var testStore *wilayah.Store

func handler(t *testing.T, apiKey string) http.Handler {
	t.Helper()
	if testStore == nil {
		s, err := wilayah.Load()
		if err != nil {
			t.Fatal(err)
		}
		testStore = s
	}
	return NewHandler(testStore, apiKey)
}

type resp struct {
	Meta meta            `json:"meta"`
	Data json.RawMessage `json:"data"`
}

func do(t *testing.T, h http.Handler, method, target, body string, hdr map[string]string) (*httptest.ResponseRecorder, resp) {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range hdr {
		req.Header.Set(k, v) // net/http canonicalizes incoming names, so key/Key are equivalent
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var r resp
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("%s %s: bad json %q", method, target, rec.Body.String())
	}
	if r.Meta.Code != rec.Code {
		t.Fatalf("meta.code %d != http %d", r.Meta.Code, rec.Code)
	}
	return rec, r
}

var key = map[string]string{"Key": "dev"}

func TestAuth(t *testing.T) {
	h := handler(t, "")
	rec, r := do(t, h, "GET", "/api/v1/destination/province", "", nil)
	if rec.Code != 401 || r.Meta.Message != "Invalid Api key, key not found" || string(r.Data) != "null" {
		t.Fatalf("no key: %d %+v", rec.Code, r)
	}
	if rec, _ = do(t, h, "GET", "/api/v1/destination/province", "", map[string]string{"Key": "x"}); rec.Code != 200 {
		t.Fatalf("Key header: %d", rec.Code)
	}
	if rec, _ = do(t, h, "GET", "/api/v1/destination/province", "", map[string]string{"key": "x"}); rec.Code != 200 {
		t.Fatalf("key header: %d", rec.Code)
	}
	hs := handler(t, "secret")
	if rec, _ = do(t, hs, "GET", "/api/v1/destination/province", "", map[string]string{"Key": "wrong"}); rec.Code != 401 {
		t.Fatalf("wrong key: %d", rec.Code)
	}
	if rec, _ = do(t, hs, "GET", "/api/v1/destination/province", "", map[string]string{"Key": "secret"}); rec.Code != 200 {
		t.Fatalf("right key: %d", rec.Code)
	}
}

func TestDestinationChain(t *testing.T) {
	h := handler(t, "")
	type item struct {
		ID      int    `json:"id"`
		Name    string `json:"name"`
		ZipCode string `json:"zip_code"`
	}
	var provs []item
	rec, r := do(t, h, "GET", "/api/v1/destination/province", "", key)
	if rec.Code != 200 || r.Meta.Status != "success" || r.Meta.Message != "Success Get Province" {
		t.Fatalf("%d %+v", rec.Code, r.Meta)
	}
	_ = json.Unmarshal(r.Data, &provs)
	if len(provs) != 38 || provs[0].ID != 1 {
		t.Fatalf("provs = %d", len(provs))
	}

	var cities, dists, subs []item
	_, r = do(t, h, "GET", "/api/v1/destination/city/"+strconv.Itoa(provs[0].ID), "", key)
	_ = json.Unmarshal(r.Data, &cities)
	_, r = do(t, h, "GET", "/api/v1/destination/district/"+strconv.Itoa(cities[0].ID), "", key)
	_ = json.Unmarshal(r.Data, &dists)
	rec, r = do(t, h, "GET", "/api/v1/destination/sub-district/"+strconv.Itoa(dists[0].ID), "", key)
	_ = json.Unmarshal(r.Data, &subs)
	if rec.Code != 200 || len(subs) == 0 || len(subs[0].ZipCode) != 5 {
		t.Fatalf("subs: %d %v", rec.Code, subs)
	}

	for path, want := range map[string]struct {
		code int
		msg  string
	}{
		"/api/v1/destination/city/abc":             {400, "Invalid province_id"},
		"/api/v1/destination/city/0":               {400, "Invalid province_id"},
		"/api/v1/destination/city/999":             {404, "City Data not found"},
		"/api/v1/destination/district/99999":       {404, "District Data not found"},
		"/api/v1/destination/sub-district/-1":      {400, "Invalid district_id"},
		"/api/v1/destination/sub-district/9999999": {404, "Sub District Data not found"},
	} {
		rec, r := do(t, h, "GET", path, "", key)
		if rec.Code != want.code || r.Meta.Message != want.msg {
			t.Errorf("%s: %d %q", path, rec.Code, r.Meta.Message)
		}
	}
}

func TestDomesticDestination(t *testing.T) {
	h := handler(t, "")
	var items []destItem
	rec, r := do(t, h, "GET", "/api/v1/destination/domestic-destination?search=ragunan&limit=5", "", key)
	_ = json.Unmarshal(r.Data, &items)
	if rec.Code != 200 || len(items) == 0 || items[0].SubdistrictName != "RAGUNAN" {
		t.Fatalf("%d %v", rec.Code, items)
	}
	rec, r = do(t, h, "POST", "/api/v1/destination/domestic-destination?search=12550", "", key)
	_ = json.Unmarshal(r.Data, &items)
	if rec.Code != 200 || len(items) == 0 || items[0].SubdistrictName != "RAGUNAN" {
		t.Fatalf("zip search: %d len=%d", rec.Code, len(items))
	}
	rec, r = do(t, h, "GET", "/api/v1/destination/domestic-destination?search=ragunan&limit=999&offset=999", "", key)
	if rec.Code != 404 || r.Meta.Message != "Domestic Destinations Data not found" {
		t.Fatalf("past-end offset: %d %s", rec.Code, r.Meta.Message)
	}
	for path, code := range map[string]int{
		"/api/v1/destination/domestic-destination":                  422,
		"/api/v1/destination/domestic-destination?search=%20":       422,
		"/api/v1/destination/domestic-destination?search=zzzzqq":    404,
		"/api/v1/destination/domestic-destination?search=a&limit=x": 400,
	} {
		if rec, _ := do(t, h, "GET", path, "", key); rec.Code != code {
			t.Errorf("%s: %d want %d", path, rec.Code, code)
		}
	}
	if rec, _ := do(t, h, "PUT", "/api/v1/destination/domestic-destination?search=a", "", key); rec.Code != 405 {
		t.Errorf("PUT: %d", rec.Code)
	}
}

func subID(t *testing.T, h http.Handler, search string) int {
	t.Helper()
	var items []destItem
	_, r := do(t, h, "GET", "/api/v1/destination/domestic-destination?limit=1&search="+url.QueryEscape(search), "", key)
	_ = json.Unmarshal(r.Data, &items)
	if len(items) == 0 {
		t.Fatalf("no destination for %q", search)
	}
	return items[0].ID
}

func TestCost(t *testing.T) {
	h := handler(t, "")
	o := subID(t, h, "ragunan, pasar minggu")
	d := subID(t, h, "kota surabaya")
	form := func(extra string) string {
		return "origin=" + strconv.Itoa(o) + "&destination=" + strconv.Itoa(d) + extra
	}
	path := "/api/v1/calculate/domestic-cost"
	type item struct {
		Code, Service string
		Cost          int64
	}

	var items []item
	rec, r := do(t, h, "POST", path, form("&weight=1000&courier=jne:SiCepat:jne&price=lowest"), key)
	if rec.Code != 200 || r.Meta.Message != "Success Calculate Domestic Shipping cost" {
		t.Fatalf("%d %+v", rec.Code, r.Meta)
	}
	_ = json.Unmarshal(r.Data, &items)
	for i := 1; i < len(items); i++ {
		if items[i].Cost < items[i-1].Cost {
			t.Fatalf("not sorted ascending: %v", items)
		}
	}
	rec2, _ := do(t, h, "POST", path, form("&weight=1000&courier=jne:SiCepat:jne&price=lowest"), key)
	if rec.Body.String() != rec2.Body.String() {
		t.Error("not deterministic")
	}

	_, r = do(t, h, "POST", path, form("&weight=1000&courier=jne:sicepat&price=highest"), key)
	_ = json.Unmarshal(r.Data, &items)
	for i := 1; i < len(items); i++ {
		if items[i].Cost > items[i-1].Cost {
			t.Fatalf("not sorted descending: %v", items)
		}
	}
	_, r = do(t, h, "POST", path, form("&weight=1000&courier=jne:sicepat"), key)
	_ = json.Unmarshal(r.Data, &items)
	if items[0].Code != "jne" || items[0].Service != "REG" {
		t.Fatalf("catalog order broken: %v", items[0])
	}

	// multipart form is accepted too
	if rec, _ := do(t, h, "POST", "/api/v1/calculate/district/domestic-cost", "origin=1&destination=2&weight=1000&courier=jne", key); rec.Code != 200 {
		t.Errorf("district cost: %d", rec.Code)
	}

	bad := map[string]struct {
		code int
		msg  string
	}{
		"origin=&destination=1&weight=1&courier=jne":         {400, "Missing params: origin"},
		"origin=1&destination=1&weight=0&courier=jne":        {400, "Invalid weight"},
		"origin=1&destination=1&weight=5":                    {400, "Missing params: courier"},
		"origin=99999999&destination=1&weight=5&courier=jne": {400, "Invalid origin"},
		"origin=1&destination=99999999&weight=5&courier=jne": {400, "Invalid destination"},
		"origin=1&destination=1&weight=5&courier=nope":       {422, "Invalid courier: nope"},
	}
	for body, want := range bad {
		rec, r := do(t, h, "POST", path, body, key)
		if rec.Code != want.code || r.Meta.Message != want.msg {
			t.Errorf("%q: %d %q", body, rec.Code, r.Meta.Message)
		}
	}
	if rec, _ := do(t, h, "GET", path, "", key); rec.Code != 405 {
		t.Errorf("GET on POST route: %d", rec.Code)
	}
}

func TestNotFoundAndHealth(t *testing.T) {
	h := handler(t, "secret")
	req := httptest.NewRequest("GET", "/nope", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	b := rec.Body.String()
	if rec.Code != 404 || !strings.Contains(rec.Header().Get("Content-Type"), "text/html") ||
		!strings.Contains(b, "https://github.com/hilmoo/shippingking") || !strings.Contains(b, "https://hilmo.dev") {
		t.Fatalf("404 page: %d %s", rec.Code, b)
	}
	req = httptest.NewRequest("GET", "/healthz", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("healthz: %d", rec.Code)
	}
	rec, r := do(t, h, "GET", "/api/v1/nope", "", map[string]string{"Key": "secret"})
	if rec.Code != 404 || r.Meta.Message != "Not Found" {
		t.Fatalf("api 404: %d %+v", rec.Code, r.Meta)
	}
}
