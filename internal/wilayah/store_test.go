package wilayah

import (
	"os"
	"path/filepath"
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
	t.Cleanup(func() { s.Close() })
	return s
}

func allSubs(t *testing.T, s *Store) []Subdistrict {
	t.Helper()
	var out []Subdistrict
	for d := 1; d < len(s.Districts); d++ {
		subs, err := s.SubdistrictsOf(d)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, subs...)
	}
	return out
}

func TestCountsAndLinks(t *testing.T) {
	s := load(t)
	if got := []int{len(s.Provinces) - 1, len(s.Cities) - 1, len(s.Districts) - 1, s.SubdistrictCount()}; got[0] != 38 || got[1] != 514 || got[2] != 7285 || got[3] != 83762 {
		t.Fatalf("counts = %v", got)
	}
	zip5 := regexp.MustCompile(`^\d{5}$`)
	for _, sd := range allSubs(t, s) {
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
	for _, sd := range allSubs(t, s) {
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
	found, err := s.Search("12550", 1000, 0)
	if err != nil {
		t.Fatal(err)
	}
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

func TestOpenPersistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	st1, _ := os.Stat(path)

	// A matching file is reused untouched.
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	st2, _ := os.Stat(path)
	if !os.SameFile(st1, st2) || !st1.ModTime().Equal(st2.ModTime()) {
		t.Fatal("matching persisted db was rewritten")
	}

	// A stale or corrupt file is replaced.
	if err := os.WriteFile(path, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.SubdistrictCount() != 83762 {
		t.Fatalf("count = %d", s.SubdistrictCount())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestSearchEdgeCases(t *testing.T) {
	s := load(t)
	// LIKE wildcards are literals, not patterns.
	for _, q := range []string{"%", "_", `\`, "ragu%an", "r_gunan"} {
		got, err := s.Search(q, 10, 0)
		if err != nil || len(got) != 0 {
			t.Fatalf("Search(%q) = %d rows, err %v; want none", q, len(got), err)
		}
	}
	// Short (scan) and long (FTS) queries are case-insensitive.
	if got, _ := s.Search("rAgUnAn", 10, 0); len(got) != 1 {
		t.Fatalf("ragunan = %d rows", len(got))
	}
	if got, _ := s.Search("ng", 5, 0); len(got) != 5 {
		t.Fatalf("ng = %d rows", len(got))
	}
	// An offset past the end clamps to the last page.
	all, _ := s.Search("jakarta selatan", 1000, 0)
	last, _ := s.Search("jakarta selatan", 10, 1_000_000)
	if len(last) != 10 || last[9].ID != all[len(all)-1].ID {
		t.Fatalf("clamped page wrong: %d rows", len(last))
	}
}
