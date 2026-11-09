// Package wilayah loads the embedded Indonesian region dataset and exposes
// in-memory indexes keyed by integer IDs (1-based per level).
package wilayah

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

//go:embed data/wilayah.csv.gz
var embedded []byte

type Province struct {
	ID       int
	Kode     string
	Name     string
	Lat, Lng float64
}

type City struct {
	ID, ProvinceID int
	Kode, Name     string
	Zip            string
	Lat, Lng       float64
}

type District struct {
	ID, CityID int
	Kode, Name string
	Zip        string
}

type Subdistrict struct {
	ID, DistrictID int
	Kode, Name     string
	Zip            string
	Label          string
	labelLower     string
}

// Store holds all regions. Slices are indexed by ID (index 0 is unused).
type Store struct {
	Provinces    []Province
	Cities       []City
	Districts    []District
	Subdistricts []Subdistrict

	CitiesByProvince       map[int][]int
	DistrictsByCity        map[int][]int
	SubdistrictsByDistrict map[int][]int
}

// Load builds a Store from the embedded dataset.
func Load() (*Store, error) {
	return LoadFrom(bytes.NewReader(embedded))
}

// LoadFrom builds a Store from a gzip'd CSV stream (header kode,nama,lat,lng,zip).
func LoadFrom(r io.Reader) (*Store, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	cr := csv.NewReader(gz)
	if _, err := cr.Read(); err != nil { // header
		return nil, err
	}
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}

	s := &Store{
		Provinces:              []Province{{}},
		Cities:                 []City{{}},
		Districts:              []District{{}},
		Subdistricts:           []Subdistrict{{}},
		CitiesByProvince:       map[int][]int{},
		DistrictsByCity:        map[int][]int{},
		SubdistrictsByDistrict: map[int][]int{},
	}
	// Rows are sorted by kode, so parents always precede children.
	provByKode := map[string]int{}
	cityByKode := map[string]int{}
	distByKode := map[string]int{}

	for _, row := range rows {
		if len(row) < 5 {
			return nil, fmt.Errorf("bad row %v", row)
		}
		kode, name, zip := row[0], strings.ToUpper(row[1]), row[4]
		lat, _ := strconv.ParseFloat(row[2], 64)
		lng, _ := strconv.ParseFloat(row[3], 64)
		switch strings.Count(kode, ".") {
		case 0:
			id := len(s.Provinces)
			s.Provinces = append(s.Provinces, Province{ID: id, Kode: kode, Name: name, Lat: lat, Lng: lng})
			provByKode[kode] = id
		case 1:
			pid, ok := provByKode[parent(kode)]
			if !ok {
				return nil, fmt.Errorf("orphan city %s", kode)
			}
			id := len(s.Cities)
			c := City{ID: id, ProvinceID: pid, Kode: kode, Name: name, Lat: lat, Lng: lng}
			if lat == 0 && lng == 0 {
				c.Lat, c.Lng = s.Provinces[pid].Lat, s.Provinces[pid].Lng
			}
			s.Cities = append(s.Cities, c)
			cityByKode[kode] = id
			s.CitiesByProvince[pid] = append(s.CitiesByProvince[pid], id)
		case 2:
			cid, ok := cityByKode[parent(kode)]
			if !ok {
				return nil, fmt.Errorf("orphan district %s", kode)
			}
			id := len(s.Districts)
			s.Districts = append(s.Districts, District{ID: id, CityID: cid, Kode: kode, Name: name})
			distByKode[kode] = id
			s.DistrictsByCity[cid] = append(s.DistrictsByCity[cid], id)
		case 3:
			did, ok := distByKode[parent(kode)]
			if !ok {
				return nil, fmt.Errorf("orphan subdistrict %s", kode)
			}
			id := len(s.Subdistricts)
			s.Subdistricts = append(s.Subdistricts, Subdistrict{ID: id, DistrictID: did, Kode: kode, Name: name, Zip: zip})
			s.SubdistrictsByDistrict[did] = append(s.SubdistrictsByDistrict[did], id)
		default:
			return nil, fmt.Errorf("bad kode %q", kode)
		}
	}

	s.deriveZips()
	s.buildLabels()
	return s, nil
}

func parent(kode string) string {
	i := strings.LastIndex(kode, ".")
	return kode[:i]
}

// deriveZips sets district/city zips to the most frequent subdistrict zip
// (ties: smallest string).
func (s *Store) deriveZips() {
	cityCounts := map[int]map[string]int{}
	for i := 1; i < len(s.Districts); i++ {
		d := &s.Districts[i]
		counts := map[string]int{}
		for _, sid := range s.SubdistrictsByDistrict[d.ID] {
			counts[s.Subdistricts[sid].Zip]++
		}
		d.Zip = mostFrequent(counts)
		cc := cityCounts[d.CityID]
		if cc == nil {
			cc = map[string]int{}
			cityCounts[d.CityID] = cc
		}
		for z, n := range counts {
			cc[z] += n
		}
	}
	for i := 1; i < len(s.Cities); i++ {
		s.Cities[i].Zip = mostFrequent(cityCounts[i])
	}
}

func mostFrequent(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	best, bestN := "", 0
	for _, k := range keys {
		if counts[k] > bestN {
			best, bestN = k, counts[k]
		}
	}
	return best
}

func (s *Store) buildLabels() {
	for i := 1; i < len(s.Subdistricts); i++ {
		sd := &s.Subdistricts[i]
		d := s.Districts[sd.DistrictID]
		c := s.Cities[d.CityID]
		p := s.Provinces[c.ProvinceID]
		sd.Label = sd.Name + ", " + d.Name + ", " + c.Name + ", " + p.Name + ", " + sd.Zip
		sd.labelLower = strings.ToLower(sd.Label)
	}
}

// Hierarchy resolves a subdistrict ID to its ancestors.
func (s *Store) Hierarchy(subID int) (Subdistrict, District, City, Province, bool) {
	if subID < 1 || subID >= len(s.Subdistricts) {
		return Subdistrict{}, District{}, City{}, Province{}, false
	}
	sd := s.Subdistricts[subID]
	d := s.Districts[sd.DistrictID]
	c := s.Cities[d.CityID]
	return sd, d, c, s.Provinces[c.ProvinceID], true
}

// DistrictChain resolves a district ID to (district, city, province).
func (s *Store) DistrictChain(id int) (District, City, Province, bool) {
	if id < 1 || id >= len(s.Districts) {
		return District{}, City{}, Province{}, false
	}
	d := s.Districts[id]
	c := s.Cities[d.CityID]
	return d, c, s.Provinces[c.ProvinceID], true
}

// Search returns subdistricts whose label contains q (case-insensitive),
// in ascending ID order, after applying offset and limit. An offset past the
// end of the matches is clamped so the last page of results is returned.
func (s *Store) Search(q string, limit, offset int) []Subdistrict {
	q = strings.ToLower(strings.TrimSpace(q))
	var matches []Subdistrict
	for i := 1; i < len(s.Subdistricts); i++ {
		if strings.Contains(s.Subdistricts[i].labelLower, q) {
			matches = append(matches, s.Subdistricts[i])
		}
	}
	if offset >= len(matches) {
		offset = max(0, len(matches)-limit)
	}
	return matches[offset:min(len(matches), offset+limit)]
}
