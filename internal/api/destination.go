package api

import (
	"net/http"
	"strconv"
	"strings"
)

type nameItem struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type zipItem struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	ZipCode string `json:"zip_code"`
}

type destItem struct {
	ID              int    `json:"id"`
	Label           string `json:"label"`
	ProvinceName    string `json:"province_name"`
	CityName        string `json:"city_name"`
	DistrictName    string `json:"district_name"`
	SubdistrictName string `json:"subdistrict_name"`
	ZipCode         string `json:"zip_code"`
}

func pathID(r *http.Request, name string) (int, bool) {
	n, err := strconv.Atoi(r.PathValue(name))
	return n, err == nil && n > 0
}

func (s *Server) provinces(w http.ResponseWriter, r *http.Request) {
	items := make([]nameItem, 0, len(s.store.Provinces))
	for _, p := range s.store.Provinces[1:] {
		items = append(items, nameItem{p.ID, p.Name})
	}
	ok(w, "Success Get Province", items)
}

func (s *Server) cities(w http.ResponseWriter, r *http.Request) {
	id, valid := pathID(r, "province_id")
	if !valid {
		fail(w, http.StatusBadRequest, "Invalid province_id")
		return
	}
	ids := s.store.CitiesByProvince[id]
	if len(ids) == 0 {
		fail(w, http.StatusNotFound, "City Data not found")
		return
	}
	items := make([]zipItem, 0, len(ids))
	for _, cid := range ids {
		c := s.store.Cities[cid]
		items = append(items, zipItem{c.ID, c.Name, c.Zip})
	}
	ok(w, "Success Get City By Province ID", items)
}

func (s *Server) districts(w http.ResponseWriter, r *http.Request) {
	id, valid := pathID(r, "city_id")
	if !valid {
		fail(w, http.StatusBadRequest, "Invalid city_id")
		return
	}
	ids := s.store.DistrictsByCity[id]
	if len(ids) == 0 {
		fail(w, http.StatusNotFound, "District Data not found")
		return
	}
	items := make([]zipItem, 0, len(ids))
	for _, did := range ids {
		d := s.store.Districts[did]
		items = append(items, zipItem{d.ID, d.Name, d.Zip})
	}
	ok(w, "Success Get District By City ID", items)
}

func (s *Server) subdistricts(w http.ResponseWriter, r *http.Request) {
	id, valid := pathID(r, "district_id")
	if !valid {
		fail(w, http.StatusBadRequest, "Invalid district_id")
		return
	}
	ids := s.store.SubdistrictsByDistrict[id]
	if len(ids) == 0 {
		fail(w, http.StatusNotFound, "Sub District Data not found")
		return
	}
	items := make([]zipItem, 0, len(ids))
	for _, sid := range ids {
		sd := s.store.Subdistricts[sid]
		items = append(items, zipItem{sd.ID, sd.Name, sd.Zip})
	}
	ok(w, "Success Get Sub District By District ID", items)
}

func (s *Server) domesticDestination(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := strings.TrimSpace(q.Get("search"))
	if search == "" {
		fail(w, http.StatusUnprocessableEntity, "search is required")
		return
	}
	limit, offset := 10, 0
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			fail(w, http.StatusBadRequest, "Invalid limit")
			return
		}
		limit = max(1, min(n, 1000))
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			fail(w, http.StatusBadRequest, "Invalid offset")
			return
		}
		offset = n
	}
	found := s.store.Search(search, limit, offset)
	if len(found) == 0 {
		fail(w, http.StatusNotFound, "Domestic Destinations Data not found")
		return
	}
	items := make([]destItem, 0, len(found))
	for _, f := range found {
		sd, d, c, p, _ := s.store.Hierarchy(f.ID)
		items = append(items, destItem{sd.ID, sd.Label, p.Name, c.Name, d.Name, sd.Name, sd.Zip})
	}
	ok(w, "Success Get Domestic Destinations", items)
}
