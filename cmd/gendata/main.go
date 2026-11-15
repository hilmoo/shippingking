// Command gendata converts the pinned wilayah SQL dumps into the compact
// dataset internal/wilayah/data/wilayah.csv.gz and then builds the prebuilt,
// FTS5-indexed SQLite database the server embeds (wilayah.db.gz plus its
// wilayah.version stamp), so the server needs no startup indexing.
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hilmoo/shippingking/internal/wilayah"
)

const (
	wilayahSHA = "a985ff702ac1d7741f5298efe9cff57398c03a44"
	kodeposSHA = "ba8497156c5cc9bcbfc527f7b8875d403eda2354"
)

var (
	reName   = regexp.MustCompile(`\('([0-9.]+)','((?:[^']|'')*)'\)`)
	reCoord  = regexp.MustCompile(`^\('([0-9.]+)','((?:[^']|'')*)',\s*(?:'(?:[^']|'')*'|NULL),\s*(-?[0-9.]+|NULL),\s*(-?[0-9.]+|NULL)`)
	reZip    = regexp.MustCompile(`\('([0-9.]+)',\s*'([0-9]*)'\)`)
	reZip5   = regexp.MustCompile(`^\d{5}$`)
	expected = map[int]int{0: 38, 1: 514, 2: 7285, 3: 83762}
)

type source struct{ name, url string }

func main() {
	src := flag.String("src", "", "directory containing the three SQL files (otherwise downloaded)")
	out := flag.String("out", "internal/wilayah/data/wilayah.csv.gz", "output CSV file")
	fromCSV := flag.Bool("from-csv", false, "skip the SQL dumps and rebuild only the database from the existing -out CSV")
	flag.Parse()

	if *fromCSV {
		if err := buildDB(*out); err != nil {
			fatal("build db: %v", err)
		}
		return
	}

	sources := []source{
		{"wilayah.sql", "https://raw.githubusercontent.com/cahyadsn/wilayah/" + wilayahSHA + "/db/wilayah.sql"},
		{"wilayah_level_1_2.sql", "https://raw.githubusercontent.com/cahyadsn/wilayah/" + wilayahSHA + "/db/wilayah_level_1_2.sql"},
		{"wilayah_kodepos.sql", "https://raw.githubusercontent.com/cahyadsn/wilayah_kodepos/" + kodeposSHA + "/db/wilayah_kodepos.sql"},
	}
	readers := map[string]io.ReadCloser{}
	for _, s := range sources {
		rc, err := open(*src, s)
		if err != nil {
			fatal("open %s: %v", s.name, err)
		}
		readers[s.name] = rc
	}

	names := map[string]string{}
	scan(readers["wilayah.sql"], func(line string) {
		for _, m := range reName.FindAllStringSubmatch(line, -1) {
			names[m[1]] = unescape(m[2])
		}
	})

	coords := map[string][2]string{}
	scan(readers["wilayah_level_1_2.sql"], func(line string) {
		if m := reCoord.FindStringSubmatch(line); m != nil && m[3] != "NULL" && m[4] != "NULL" {
			coords[m[1]] = [2]string{m[3], m[4]}
		}
	})

	zips := map[string]string{}
	scan(readers["wilayah_kodepos.sql"], func(line string) {
		line = strings.TrimPrefix(line, "\xef\xbb\xbf")
		for _, m := range reZip.FindAllStringSubmatch(line, -1) {
			zips[m[1]] = m[2]
		}
	})

	counts := map[int]int{}
	for k := range names {
		counts[strings.Count(k, ".")]++
	}
	for lvl := 0; lvl <= 3; lvl++ {
		if counts[lvl] == 0 {
			fatal("level %d has 0 rows", lvl)
		}
	}
	for k := range names {
		if i := strings.LastIndex(k, "."); i >= 0 {
			if _, ok := names[k[:i]]; !ok {
				fatal("parent of %s missing", k)
			}
		}
	}
	for k := range names {
		if strings.Count(k, ".") != 3 {
			continue
		}
		z, ok := zips[k]
		if !ok || !reZip5.MatchString(z) {
			fatal("subdistrict %s has bad zip %q", k, z)
		}
	}
	for k := range zips {
		if _, ok := names[k]; !ok {
			fatal("kodepos kode %s not in wilayah", k)
		}
	}
	for lvl := 0; lvl <= 1; lvl++ {
		for k := range names {
			if strings.Count(k, ".") == lvl {
				if _, ok := coords[k]; !ok {
					fmt.Fprintf(os.Stderr, "warning: %s has no coordinates\n", k)
				}
			}
		}
	}

	keys := make([]string, 0, len(names))
	for k := range names {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fatal("%v", err)
	}
	f, err := os.Create(*out)
	if err != nil {
		fatal("%v", err)
	}
	gz, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
	w := csv.NewWriter(gz)
	_ = w.Write([]string{"kode", "nama", "lat", "lng", "zip"})
	for _, k := range keys {
		c := coords[k]
		zip := ""
		if strings.Count(k, ".") == 3 {
			zip = zips[k]
		}
		_ = w.Write([]string{k, names[k], c[0], c[1], zip})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		fatal("%v", err)
	}
	if err := gz.Close(); err != nil {
		fatal("%v", err)
	}
	if err := f.Close(); err != nil {
		fatal("%v", err)
	}

	if err := buildDB(*out); err != nil {
		fatal("build db: %v", err)
	}

	fmt.Printf("provinces=%d cities=%d districts=%d subdistricts=%d zips=%d\n",
		counts[0], counts[1], counts[2], counts[3], len(zips))
	for lvl, want := range expected {
		if counts[lvl] != want {
			fmt.Fprintf(os.Stderr, "warning: level %d count %d, expected %d\n", lvl, counts[lvl], want)
		}
	}
}

// buildDB turns the CSV at csvPath into wilayah.db.gz and wilayah.version
// next to it.
func buildDB(csvPath string) error {
	csvGz, err := os.ReadFile(csvPath)
	if err != nil {
		return err
	}
	dir := filepath.Dir(csvPath)
	version := wilayah.DatasetVersion(csvGz)

	tmp, err := os.MkdirTemp("", "gendata-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	dbPath := filepath.Join(tmp, "wilayah.db")
	if err := wilayah.BuildDB(bytes.NewReader(csvGz), dbPath, version); err != nil {
		return err
	}
	raw, err := os.Open(dbPath)
	if err != nil {
		return err
	}
	defer raw.Close()
	st, _ := raw.Stat()

	dbGz := filepath.Join(dir, "wilayah.db.gz")
	f, err := os.Create(dbGz)
	if err != nil {
		return err
	}
	gz, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
	if _, err := io.Copy(gz, raw); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "wilayah.version"), []byte(version+"\n"), 0o644); err != nil {
		return err
	}
	gzSt, _ := os.Stat(dbGz)
	fmt.Printf("db version=%s raw=%.1fMB gz=%.1fMB\n", version, float64(st.Size())/1e6, float64(gzSt.Size())/1e6)
	return nil
}

func open(dir string, s source) (io.ReadCloser, error) {
	if dir != "" {
		return os.Open(filepath.Join(dir, s.name))
	}
	resp, err := http.Get(s.url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func scan(rc io.ReadCloser, fn func(string)) {
	defer rc.Close()
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		fn(sc.Text())
	}
	if err := sc.Err(); err != nil {
		fatal("scan: %v", err)
	}
}

func unescape(s string) string { return strings.ReplaceAll(s, "''", "'") }

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "gendata: "+format+"\n", a...)
	os.Exit(1)
}
