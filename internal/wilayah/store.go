// Package wilayah serves the Indonesian region dataset from an embedded,
// prebuilt SQLite database (see cmd/gendata). Provinces, cities and districts
// (~8k rows) are kept in memory keyed by integer IDs (1-based per level); the
// ~84k subdistricts and their substring search stay in SQLite behind an FTS5
// trigram index so they cost no Go heap.
package wilayah

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

//go:embed data/wilayah.db.gz
var embeddedDB []byte

//go:embed data/wilayah.version
var embeddedVersion string

// DatasetVersion derives the version stamp for a CSV dataset; gendata stores it
// in the DB and wilayah.version, and Open uses it to validate persisted files.
func DatasetVersion(csvGz []byte) string {
	sum := sha256.Sum256(csvGz)
	return schemaVersion + "-" + hex.EncodeToString(sum[:8])
}

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
}

// Store holds provinces/cities/districts in memory (slices indexed by ID,
// index 0 unused) and subdistricts in SQLite. Call Close when done.
type Store struct {
	Provinces []Province
	Cities    []City
	Districts []District

	CitiesByProvince map[int][]int
	DistrictsByCity  map[int][]int

	nSub    int
	db      *sql.DB
	dbPath  string
	ownFile bool
	byID    *sql.Stmt
	byDist  *sql.Stmt
}

// SubdistrictCount returns the number of subdistricts.
func (s *Store) SubdistrictCount() int { return s.nSub }

// Close releases the database; a temp-file database is also deleted.
func (s *Store) Close() error {
	err := s.db.Close()
	if s.ownFile {
		_ = os.Remove(s.dbPath)
	}
	return err
}

// Load opens the embedded dataset using a throwaway temp-file database.
func Load() (*Store, error) { return Open("") }

// Open opens the dataset at path. If path holds a database matching the
// embedded dataset version it is reused as-is (no startup indexing);
// otherwise the embedded database is unpacked there atomically. An empty path
// unpacks into a temp file that Close deletes.
func Open(path string) (*Store, error) {
	want := strings.TrimSpace(embeddedVersion)
	own := path == ""
	if own {
		f, err := os.CreateTemp("", "wilayah-*.db")
		if err != nil {
			return nil, err
		}
		path = f.Name()
		f.Close()
	} else if v, err := fileVersion(path); err == nil && v == want {
		return openFile(path, false)
	}
	if err := unpack(path); err != nil {
		if own {
			os.Remove(path)
		}
		return nil, err
	}
	return openFile(path, own)
}

// fileVersion reads the dataset version stamped in a database file.
func fileVersion(path string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return "", err
	}
	defer db.Close()
	var v string
	err = db.QueryRow(`SELECT value FROM meta WHERE key='version'`).Scan(&v)
	return v, err
}

// unpack writes the embedded database to path via a temp file + rename.
func unpack(path string) error {
	gz, err := gzip.NewReader(bytes.NewReader(embeddedDB))
	if err != nil {
		return err
	}
	defer gz.Close()
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	_, err = io.Copy(tmp, gz)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

func openFile(path string, own bool) (_ *Store, err error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=cache_size(-2048)&_pragma=temp_store(FILE)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	s := &Store{
		Provinces:        []Province{{}},
		Cities:           []City{{}},
		Districts:        []District{{}},
		CitiesByProvince: map[int][]int{},
		DistrictsByCity:  map[int][]int{},
		db:               db, dbPath: path, ownFile: own,
	}
	defer func() {
		if err != nil {
			s.Close()
		}
	}()

	rows, err := db.Query(`SELECT id,kode,name,lat,lng FROM province ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p Province
		if err = rows.Scan(&p.ID, &p.Kode, &p.Name, &p.Lat, &p.Lng); err != nil {
			rows.Close()
			return nil, err
		}
		s.Provinces = append(s.Provinces, p)
	}
	rows.Close()
	if rows, err = db.Query(`SELECT id,province_id,kode,name,zip,lat,lng FROM city ORDER BY id`); err != nil {
		return nil, err
	}
	for rows.Next() {
		var c City
		if err = rows.Scan(&c.ID, &c.ProvinceID, &c.Kode, &c.Name, &c.Zip, &c.Lat, &c.Lng); err != nil {
			rows.Close()
			return nil, err
		}
		s.Cities = append(s.Cities, c)
		s.CitiesByProvince[c.ProvinceID] = append(s.CitiesByProvince[c.ProvinceID], c.ID)
	}
	rows.Close()
	if rows, err = db.Query(`SELECT id,city_id,kode,name,zip FROM district ORDER BY id`); err != nil {
		return nil, err
	}
	for rows.Next() {
		var d District
		if err = rows.Scan(&d.ID, &d.CityID, &d.Kode, &d.Name, &d.Zip); err != nil {
			rows.Close()
			return nil, err
		}
		s.Districts = append(s.Districts, d)
		s.DistrictsByCity[d.CityID] = append(s.DistrictsByCity[d.CityID], d.ID)
	}
	rows.Close()
	if err = db.QueryRow(`SELECT count(*) FROM subdistrict`).Scan(&s.nSub); err != nil {
		return nil, err
	}

	const cols = `id,district_id,kode,name,zip,label`
	if s.byID, err = db.Prepare(`SELECT ` + cols + ` FROM subdistrict WHERE id=?`); err != nil {
		return nil, err
	}
	if s.byDist, err = db.Prepare(`SELECT ` + cols + ` FROM subdistrict WHERE district_id=? ORDER BY id`); err != nil {
		return nil, err
	}
	debug.FreeOSMemory()
	return s, nil
}

type scanner interface{ Scan(...any) error }

func scanSub(r scanner) (Subdistrict, error) {
	var sd Subdistrict
	err := r.Scan(&sd.ID, &sd.DistrictID, &sd.Kode, &sd.Name, &sd.Zip, &sd.Label)
	return sd, err
}

// Subdistrict returns one subdistrict by ID.
func (s *Store) Subdistrict(id int) (Subdistrict, bool) {
	sd, err := scanSub(s.byID.QueryRow(id))
	return sd, err == nil
}

// SubdistrictsOf lists a district's subdistricts in ID order.
func (s *Store) SubdistrictsOf(districtID int) ([]Subdistrict, error) {
	rows, err := s.byDist.Query(districtID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Subdistrict
	for rows.Next() {
		sd, err := scanSub(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sd)
	}
	return out, rows.Err()
}

// Hierarchy resolves a subdistrict ID to its ancestors.
func (s *Store) Hierarchy(subID int) (Subdistrict, District, City, Province, bool) {
	sd, ok := s.Subdistrict(subID)
	if !ok {
		return Subdistrict{}, District{}, City{}, Province{}, false
	}
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

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// Search returns subdistricts whose label contains q (case-insensitive),
// in ascending ID order, after applying offset and limit. An offset past the
// end of the matches is clamped so the last page of results is returned.
// Plain queries of 3+ characters use the FTS5 trigram index; shorter ones, or
// ones containing LIKE wildcards (which FTS5 cannot ESCAPE), scan.
func (s *Store) Search(q string, limit, offset int) ([]Subdistrict, error) {
	q = strings.TrimSpace(q)
	var cond, pat string
	if utf8.RuneCountInString(q) >= 3 && !strings.ContainsAny(q, `%_\`) {
		cond = `s.id IN (SELECT rowid FROM subdistrict_fts WHERE label LIKE ?)`
		pat = "%" + q + "%"
	} else {
		cond = `s.label LIKE ? ESCAPE '\'`
		pat = "%" + likeEscaper.Replace(q) + "%"
	}
	page := func(off int) ([]Subdistrict, error) {
		rows, err := s.db.Query(`SELECT s.id,s.district_id,s.kode,s.name,s.zip,s.label FROM subdistrict s WHERE `+
			cond+` ORDER BY s.id LIMIT ? OFFSET ?`, pat, limit, off)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []Subdistrict
		for rows.Next() {
			sd, err := scanSub(rows)
			if err != nil {
				return nil, err
			}
			out = append(out, sd)
		}
		return out, rows.Err()
	}
	out, err := page(offset)
	if err != nil || len(out) > 0 || offset == 0 {
		return out, err
	}
	// Offset is past the end: clamp to the last page (needs the match count).
	var total int
	if err := s.db.QueryRow(`SELECT count(*) FROM subdistrict s WHERE `+cond, pat).Scan(&total); err != nil {
		return nil, err
	}
	return page(max(0, total-limit))
}
