// Package pricing implements the deterministic dummy shipping-cost engine.
package pricing

type Class int

const (
	Regular Class = iota
	Economy
	NextDay
	SameDay
	Trucking
)

type classInfo struct {
	multPermille int64
	minKg        int64
}

var classes = map[Class]classInfo{
	Regular:  {1000, 1},
	Economy:  {850, 1},
	NextDay:  {1600, 1},
	SameDay:  {2200, 1},
	Trucking: {550, 10},
}

type Service struct {
	Code, Description string
	Class             Class
}

type Courier struct {
	Code, Name   string
	MultPermille int64
	Services     []Service
}

// Catalog is in the order of PLAN.md §6.1.
var Catalog = []Courier{
	{"jne", "Jalur Nugraha Ekakurir (JNE)", 1000, []Service{
		{"REG", "Layanan Reguler", Regular}, {"OKE", "Ongkos Kirim Ekonomis", Economy},
		{"YES", "Yakin Esok Sampai", NextDay}, {"JTR", "JNE Trucking", Trucking}}},
	{"sicepat", "SiCepat Express", 920, []Service{
		{"REG", "Reguler", Regular}, {"BEST", "Besok Sampai Tujuan", NextDay}, {"GOKIL", "Cargo", Trucking}}},
	{"ide", "ID Express", 880, []Service{
		{"STD", "Standard", Regular}, {"IDLITE", "Ekonomi", Economy}}},
	{"sap", "SAP Express", 900, []Service{
		{"REG", "Regular", Regular}, {"ODS", "One Day Service", NextDay}}},
	{"jnt", "J&T Express", 970, []Service{
		{"EZ", "Reguler", Regular}}},
	{"ninja", "Ninja Xpress", 950, []Service{
		{"STANDARD", "Standard Service", Regular}}},
	{"tiki", "Citra Van Titipan Kilat (TIKI)", 1000, []Service{
		{"REG", "Regular Service", Regular}, {"ECO", "Economy Service", Economy},
		{"ONS", "Over Night Service", NextDay}, {"TRC", "Trucking Service", Trucking}}},
	{"lion", "Lion Parcel", 900, []Service{
		{"REGPACK", "Regular", Regular}, {"JAGOPACK", "Ekonomi", Economy},
		{"ONEPACK", "One Day", NextDay}, {"BIGPACK", "Big Package", Trucking}}},
	{"anteraja", "AnterAja", 930, []Service{
		{"REG", "Regular", Regular}, {"ND", "Next Day", NextDay}, {"SD", "Same Day", SameDay}}},
	{"pos", "POS Indonesia (POS)", 950, []Service{
		{"REG", "Pos Reguler", Regular}, {"NEXTDAY", "Pos Nextday", NextDay}, {"JUMBO", "Pos Jumbo Ekonomi", Trucking}}},
	{"ncs", "Nusantara Card Semesta (NCS)", 980, []Service{
		{"NRS", "Regular", Regular}, {"ONS", "Over Night Service", NextDay}}},
	{"rex", "Royal Express Asia (REX)", 960, []Service{
		{"REG", "Regular", Regular}, {"EXP", "Express", NextDay}}},
	{"rpx", "RPX Holding (RPX)", 1050, []Service{
		{"RGP", "Regular Package", Regular}, {"NDP", "Next Day Package", NextDay}}},
	{"sentral", "Sentral Cargo", 850, []Service{
		{"REG", "Regular", Regular}, {"CARGO", "Cargo", Trucking}}},
	{"star", "Star Cargo", 870, []Service{
		{"REG", "Regular", Regular}, {"CARGO", "Cargo", Trucking}}},
	{"wahana", "Wahana Prestasi Logistik", 820, []Service{
		{"NORMAL", "Normal Service", Regular}}},
	{"dse", "21 Express (DSE)", 920, []Service{
		{"ECO", "Economy Service", Economy}, {"ONS", "Over Night Service", NextDay}}},
}

var courierByCode = func() map[string]*Courier {
	m := make(map[string]*Courier, len(Catalog))
	for i := range Catalog {
		m[Catalog[i].Code] = &Catalog[i]
	}
	return m
}()

// LookupCourier returns the catalog entry for a (lowercase) code.
func LookupCourier(code string) (*Courier, bool) {
	c, ok := courierByCode[code]
	return c, ok
}

type group int

const (
	gSumatera group = iota
	gJawa
	gBaliNusra
	gKalimantan
	gSulawesi
	gMaluku
	gPapua
)

type groupInfo struct {
	remote   int64
	cluster  string
	extraDay int
}

var groups = map[group]groupInfo{
	gSumatera:   {5000, "WEST", 0},
	gJawa:       {0, "WEST", 0},
	gBaliNusra:  {5000, "WEST", 0},
	gKalimantan: {8000, "KALIMANTAN", 0},
	gSulawesi:   {10000, "SULAWESI", 0},
	gMaluku:     {20000, "MALUKU", 1},
	gPapua:      {35000, "PAPUA", 2},
}

// groupOf maps a province kode (e.g. "31") to its region group.
func groupOf(provKode string) group {
	n := 0
	for _, ch := range provKode {
		if ch < '0' || ch > '9' {
			break
		}
		n = n*10 + int(ch-'0')
	}
	switch {
	case n >= 11 && n <= 21:
		return gSumatera
	case n >= 31 && n <= 36:
		return gJawa
	case n >= 51 && n <= 53:
		return gBaliNusra
	case n >= 61 && n <= 65:
		return gKalimantan
	case n >= 71 && n <= 76:
		return gSulawesi
	case n >= 81 && n <= 82:
		return gMaluku
	case n >= 91 && n <= 96:
		return gPapua
	}
	return gJawa
}
