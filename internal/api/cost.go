package api

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/hilmoo/shippingking/internal/pricing"
)

type costItem struct {
	Name        string `json:"name"`
	Code        string `json:"code"`
	Service     string `json:"service"`
	Description string `json:"description"`
	Cost        int64  `json:"cost"`
	ETD         string `json:"etd"`
}

func (s *Server) costSubdistrict(w http.ResponseWriter, r *http.Request) {
	s.cost(w, r, func(id int) (pricing.Location, bool) {
		_, d, c, p, found := s.store.Hierarchy(id)
		if !found {
			return pricing.Location{}, false
		}
		return pricing.Location{DistrictID: d.ID, CityID: c.ID, ProvinceID: p.ID,
			ProvinceKode: p.Kode, CityLat: c.Lat, CityLng: c.Lng}, true
	})
}

func (s *Server) costDistrict(w http.ResponseWriter, r *http.Request) {
	s.cost(w, r, func(id int) (pricing.Location, bool) {
		d, c, p, found := s.store.DistrictChain(id)
		if !found {
			return pricing.Location{}, false
		}
		return pricing.Location{DistrictID: d.ID, CityID: c.ID, ProvinceID: p.ID,
			ProvinceKode: p.Kode, CityLat: c.Lat, CityLng: c.Lng}, true
	})
}

func (s *Server) cost(w http.ResponseWriter, r *http.Request, resolve func(int) (pricing.Location, bool)) {
	if err := r.ParseMultipartForm(1 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		fail(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	form := r.PostForm
	if r.MultipartForm != nil {
		form = r.Form
	}
	get := func(k string) string { return strings.TrimSpace(form.Get(k)) }

	var ints [3]int
	for i, name := range []string{"origin", "destination", "weight"} {
		v := get(name)
		if v == "" {
			fail(w, http.StatusBadRequest, "Missing params: "+name)
			return
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			fail(w, http.StatusBadRequest, "Invalid "+name)
			return
		}
		ints[i] = n
	}
	if get("courier") == "" {
		fail(w, http.StatusBadRequest, "Missing params: courier")
		return
	}
	var couriers []string
	seen := map[string]bool{}
	for _, c := range strings.Split(get("courier"), ":") {
		c = strings.ToLower(strings.TrimSpace(c))
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		couriers = append(couriers, c)
	}
	if len(couriers) == 0 {
		fail(w, http.StatusBadRequest, "Invalid courier")
		return
	}

	o, okO := resolve(ints[0])
	if !okO {
		fail(w, http.StatusBadRequest, "Invalid origin")
		return
	}
	d, okD := resolve(ints[1])
	if !okD {
		fail(w, http.StatusBadRequest, "Invalid destination")
		return
	}

	results, err := pricing.Quote(o, d, ints[2], couriers)
	if err != nil {
		var uc *pricing.UnknownCourierError
		if errors.As(err, &uc) {
			fail(w, http.StatusUnprocessableEntity, uc.Error())
			return
		}
		fail(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}
	if len(results) == 0 {
		fail(w, http.StatusBadRequest, "Calculate Domestic Shipping cost not found")
		return
	}

	switch strings.ToLower(get("price")) {
	case "lowest":
		sort.SliceStable(results, func(i, j int) bool { return results[i].Cost < results[j].Cost })
	case "highest":
		sort.SliceStable(results, func(i, j int) bool { return results[i].Cost > results[j].Cost })
	}

	items := make([]costItem, len(results))
	for i, x := range results {
		items[i] = costItem{x.Name, x.Code, x.Service, x.Description, x.Cost, x.ETD}
	}
	ok(w, "Success Calculate Domestic Shipping cost", items)
}
