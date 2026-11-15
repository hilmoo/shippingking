package wilayah

import (
	"compress/gzip"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

// schemaVersion is bumped whenever the SQLite layout or query semantics change;
// it is part of the dataset version, so stale persistent DB files get replaced.
const schemaVersion = "1"

const schema = `
CREATE TABLE meta(key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE province(id INTEGER PRIMARY KEY, kode TEXT NOT NULL, name TEXT NOT NULL, lat REAL NOT NULL, lng REAL NOT NULL);
CREATE TABLE city(id INTEGER PRIMARY KEY, province_id INTEGER NOT NULL, kode TEXT NOT NULL, name TEXT NOT NULL, zip TEXT NOT NULL, lat REAL NOT NULL, lng REAL NOT NULL);
CREATE TABLE district(id INTEGER PRIMARY KEY, city_id INTEGER NOT NULL, kode TEXT NOT NULL, name TEXT NOT NULL, zip TEXT NOT NULL);
CREATE TABLE subdistrict(
	id INTEGER PRIMARY KEY, district_id INTEGER NOT NULL,
	kode TEXT NOT NULL, name TEXT NOT NULL, zip TEXT NOT NULL, label TEXT NOT NULL);
CREATE INDEX subdistrict_district ON subdistrict(district_id);
CREATE VIRTUAL TABLE subdistrict_fts USING fts5(label, content='subdistrict', content_rowid='id', tokenize='trigram', detail=column);
`

// BuildDB converts the gzip'd CSV dataset (header kode,nama,lat,lng,zip) into a
// ready-to-query SQLite file at dbPath (replaced if present). version is stored
// in the meta table so a persisted file can be matched against the binary.
func BuildDB(csvGz io.Reader, dbPath, version string) error {
	gz, err := gzip.NewReader(csvGz)
	if err != nil {
		return err
	}
	defer gz.Close()
	cr := csv.NewReader(gz)
	cr.ReuseRecord = true
	if _, err := cr.Read(); err != nil { // header
		return err
	}

	_ = os.Remove(dbPath)
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=journal_mode(OFF)&_pragma=synchronous(OFF)")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var provinces []Province
	var cities []City
	var districts []District
	provByKode := map[string]int{}
	cityByKode := map[string]int{}
	distByKode := map[string]int{}
	distZips := map[int]map[string]int{}
	provinces, cities, districts = append(provinces, Province{}), append(cities, City{}), append(districts, District{})

	insSub, err := tx.Prepare(`INSERT INTO subdistrict(id,district_id,kode,name,zip,label) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	nSub := 0
	// Rows are sorted by kode, so parents always precede children.
	for {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if len(row) < 5 {
			return fmt.Errorf("bad row %v", row)
		}
		kode, name, zip := strings.Clone(row[0]), strings.ToUpper(row[1]), row[4]
		lat, _ := strconv.ParseFloat(row[2], 64)
		lng, _ := strconv.ParseFloat(row[3], 64)
		switch strings.Count(kode, ".") {
		case 0:
			id := len(provinces)
			provinces = append(provinces, Province{ID: id, Kode: kode, Name: name, Lat: lat, Lng: lng})
			provByKode[kode] = id
		case 1:
			pid, ok := provByKode[parent(kode)]
			if !ok {
				return fmt.Errorf("orphan city %s", kode)
			}
			id := len(cities)
			c := City{ID: id, ProvinceID: pid, Kode: kode, Name: name, Lat: lat, Lng: lng}
			if lat == 0 && lng == 0 {
				c.Lat, c.Lng = provinces[pid].Lat, provinces[pid].Lng
			}
			cities = append(cities, c)
			cityByKode[kode] = id
		case 2:
			cid, ok := cityByKode[parent(kode)]
			if !ok {
				return fmt.Errorf("orphan district %s", kode)
			}
			id := len(districts)
			districts = append(districts, District{ID: id, CityID: cid, Kode: kode, Name: name})
			distByKode[kode] = id
		case 3:
			did, ok := distByKode[parent(kode)]
			if !ok {
				return fmt.Errorf("orphan subdistrict %s", kode)
			}
			d := districts[did]
			c := cities[d.CityID]
			p := provinces[c.ProvinceID]
			nSub++
			label := name + ", " + d.Name + ", " + c.Name + ", " + p.Name + ", " + zip
			if _, err := insSub.Exec(nSub, did, kode, name, zip, label); err != nil {
				return err
			}
			if distZips[did] == nil {
				distZips[did] = map[string]int{}
			}
			distZips[did][zip]++
		default:
			return fmt.Errorf("bad kode %q", kode)
		}
	}

	// District/city zips are the most frequent subdistrict zip.
	cityCounts := map[int]map[string]int{}
	for i := 1; i < len(districts); i++ {
		d := &districts[i]
		d.Zip = mostFrequent(distZips[d.ID])
		cc := cityCounts[d.CityID]
		if cc == nil {
			cc = map[string]int{}
			cityCounts[d.CityID] = cc
		}
		for z, n := range distZips[d.ID] {
			cc[z] += n
		}
	}
	for i := 1; i < len(cities); i++ {
		cities[i].Zip = mostFrequent(cityCounts[i])
	}

	for _, p := range provinces[1:] {
		if _, err := tx.Exec(`INSERT INTO province VALUES(?,?,?,?,?)`, p.ID, p.Kode, p.Name, p.Lat, p.Lng); err != nil {
			return err
		}
	}
	for _, c := range cities[1:] {
		if _, err := tx.Exec(`INSERT INTO city VALUES(?,?,?,?,?,?,?)`, c.ID, c.ProvinceID, c.Kode, c.Name, c.Zip, c.Lat, c.Lng); err != nil {
			return err
		}
	}
	for _, d := range districts[1:] {
		if _, err := tx.Exec(`INSERT INTO district VALUES(?,?,?,?,?)`, d.ID, d.CityID, d.Kode, d.Name, d.Zip); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT INTO subdistrict_fts(rowid,label) SELECT id,label FROM subdistrict`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO meta VALUES('version',?),('subdistricts',?)`, version, strconv.Itoa(nSub)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT INTO subdistrict_fts(subdistrict_fts) VALUES('optimize')`); err != nil {
		return err
	}
	if _, err := db.Exec(`VACUUM`); err != nil {
		return err
	}
	return db.Close()
}

func parent(kode string) string {
	i := strings.LastIndex(kode, ".")
	return kode[:i]
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
