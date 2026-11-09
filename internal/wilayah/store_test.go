package wilayah

import (
	"regexp"
	"strings"
	"testing"
)

func load(t *testing.T) *Store {
	t.Helper()
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCountsAndLinks(t *testing.T) {
	s := load(t)
	if got := []int{len(s.Provinces) - 1, len(s.Cities) - 1, len(s.Districts) - 1, len(s.Subdistricts) - 1}; got[0] != 38 || got[1] != 514 || got[2] != 7285 || got[3] != 83762 {
		t.Fatalf("counts = %v", got)
	}
	zip5 := regexp.MustCompile(`^\d{5}$`)
	for _, sd := range s.Subdistricts[1:] {
		if !zip5.MatchString(sd.Zip) {
			t.Fatalf("bad zip %q for %s", sd.Zip, sd.Kode)
		}
		if sd.DistrictID < 1 || s.Districts[sd.DistrictID].Kode != sd.Kode[:strings.LastIndex(sd.Kode, ".")] {
			t.Fatalf("bad parent for %s", sd.Kode)
		}
	}
	for _, c := range s.Cities[1:] {
		if c.Lat == 0 && c.Lng == 0 {
			t.Fatalf("city %s has no coordinates", c.Kode)
		}
	}
}

func TestKnownZips(t *testing.T) {
	s := load(t)
	var ragunan Subdistrict
	for _, sd := range s.Subdistricts[1:] {
		if sd.Kode == "31.74.04.1004" {
			ragunan = sd
		}
	}
	if ragunan.Zip != "12550" {
		t.Fatalf("Ragunan zip = %q", ragunan.Zip)
	}
	for _, d := range s.Districts[1:] {
		if d.Kode == "31.74.04" && d.Zip != "12510" {
			t.Fatalf("Pasar Minggu zip = %q", d.Zip)
		}
	}
	found := s.Search("12550", 1000, 0)
	hit := false
	for _, f := range found {
		if f.ID == ragunan.ID {
			hit = true
		}
	}
	if !hit {
		t.Fatal("search 12550 did not find Ragunan")
	}
	if !strings.HasPrefix(ragunan.Label, "RAGUNAN, PASAR MINGGU, KOTA ADMINISTRASI JAKARTA SELATAN, ") {
		t.Fatalf("label = %q", ragunan.Label)
	}
}
