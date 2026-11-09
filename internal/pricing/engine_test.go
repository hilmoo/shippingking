package pricing

import (
	"reflect"
	"testing"

	"github.com/hilmoo/shippingking/internal/wilayah"
)

func cityLoc(t *testing.T, s *wilayah.Store, kode string, districtIdx int) Location {
	t.Helper()
	for _, c := range s.Cities[1:] {
		if c.Kode == kode {
			ds := s.DistrictsByCity[c.ID]
			p := s.Provinces[c.ProvinceID]
			return Location{DistrictID: ds[districtIdx], CityID: c.ID, ProvinceID: p.ID,
				ProvinceKode: p.Kode, CityLat: c.Lat, CityLng: c.Lng}
		}
	}
	t.Fatalf("city %s not found", kode)
	return Location{}
}

func costs(t *testing.T, o, d Location, w int, couriers ...string) map[string]int64 {
	t.Helper()
	res, err := Quote(o, d, w, couriers)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]int64{}
	for _, r := range res {
		m[r.Code+" "+r.Service] = r.Cost
	}
	return m
}

func TestGolden(t *testing.T) {
	s, err := wilayah.Load()
	if err != nil {
		t.Fatal(err)
	}
	o := cityLoc(t, s, "31.74", 0)
	type row struct {
		kode                   string
		reg, oke, yes, jtr, sc int64 // 0 = n/a
	}
	rows := []row{
		{"32.73", 11000, 9500, 17500, 60000, 10000},
		{"35.78", 18500, 15500, 29500, 100000, 17000},
		{"51.71", 26500, 22500, 42000, 145000, 24500},
		{"12.71", 31000, 26500, 49500, 170000, 28500},
		{"73.71", 35500, 30500, 57000, 0, 33000},
		{"91.71", 107500, 91500, 0, 0, 98500},
	}
	for _, r := range rows {
		d := cityLoc(t, s, r.kode, 0)
		m := costs(t, o, d, 1000, "jne", "sicepat")
		check := func(key string, want int64) {
			got, present := m[key]
			if want == 0 {
				if present {
					t.Errorf("%s %s: expected n/a, got %d", r.kode, key, got)
				}
				return
			}
			if got != want {
				t.Errorf("%s %s = %d, want %d", r.kode, key, got, want)
			}
		}
		check("jne REG", r.reg)
		check("jne OKE", r.oke)
		check("jne YES", r.yes)
		check("jne JTR", r.jtr)
		check("sicepat REG", r.sc)
	}

	// same city, other district
	d := cityLoc(t, s, "31.74", 1)
	m := costs(t, o, d, 1000, "jne", "sicepat")
	if m["jne REG"] != 9000 || m["jne OKE"] != 8000 || m["jne YES"] != 14500 || m["jne JTR"] != 50000 || m["sicepat REG"] != 8500 {
		t.Errorf("same city: %v", m)
	}

	sby := cityLoc(t, s, "35.78", 0)
	if c := costs(t, o, sby, 2500, "jne")["jne REG"]; c != 55500 {
		t.Errorf("2500g = %d", c)
	}
	res, _ := Quote(o, sby, 1000, []string{"jne"})
	if res[0].ETD != "2-4 day" {
		t.Errorf("etd = %s", res[0].ETD)
	}
	jyp := cityLoc(t, s, "91.71", 0)
	res, _ = Quote(o, jyp, 1000, []string{"jne"})
	if res[0].ETD != "6-9 day" {
		t.Errorf("etd = %s", res[0].ETD)
	}
}

func TestWeightRounding(t *testing.T) {
	cases := map[int]int64{1: 1, 1000: 1, 1300: 1, 1301: 2, 10000: 10}
	for g, want := range cases {
		if got := chargeKg(g); got != want {
			t.Errorf("chargeKg(%d) = %d, want %d", g, got, want)
		}
	}
}

func TestAvailabilityAndErrors(t *testing.T) {
	s, _ := wilayah.Load()
	o := cityLoc(t, s, "31.74", 0)
	// SAME_DAY only within the same city.
	if _, ok := costs(t, o, cityLoc(t, s, "31.74", 1), 1000, "anteraja")["anteraja SD"]; !ok {
		t.Error("SD should be available in same city")
	}
	if _, ok := costs(t, o, cityLoc(t, s, "32.73", 0), 1000, "anteraja")["anteraja SD"]; ok {
		t.Error("SD must not be available across cities")
	}
	// NEXT_DAY unavailable to Papua.
	if _, ok := costs(t, o, cityLoc(t, s, "91.71", 0), 1000, "jne")["jne YES"]; ok {
		t.Error("YES must not reach Papua")
	}
	if _, err := Quote(o, o, 1000, []string{"nope"}); err == nil {
		t.Error("expected unknown courier error")
	}
}

func TestDeterministic(t *testing.T) {
	s, _ := wilayah.Load()
	o, d := cityLoc(t, s, "31.74", 0), cityLoc(t, s, "35.78", 0)
	a, _ := Quote(o, d, 1234, []string{"jne", "tiki", "pos"})
	b, _ := Quote(o, d, 1234, []string{"jne", "tiki", "pos"})
	if !reflect.DeepEqual(a, b) {
		t.Error("not deterministic")
	}
}
