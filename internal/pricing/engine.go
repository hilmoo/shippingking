package pricing

import (
	"fmt"
	"math"
)

// Location identifies one end of a shipment.
type Location struct {
	DistrictID, CityID, ProvinceID int
	ProvinceKode                   string
	CityLat, CityLng               float64
}

type Result struct {
	Name        string
	Code        string
	Service     string
	Description string
	Cost        int64
	ETD         string
}

// UnknownCourierError is returned for a courier code not in the catalog.
type UnknownCourierError struct{ Code string }

func (e *UnknownCourierError) Error() string { return "Invalid courier: " + e.Code }

func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}

func clamp(v, lo, hi int64) int64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

type route struct {
	sameDistrict, sameCity bool
	distKm                 int64
	og, dg                 group
	baseRate               int64
}

func newRoute(o, d Location) route {
	rt := route{
		sameDistrict: o.DistrictID == d.DistrictID,
		sameCity:     o.CityID == d.CityID,
		og:           groupOf(o.ProvinceKode),
		dg:           groupOf(d.ProvinceKode),
	}
	switch {
	case rt.sameDistrict:
		rt.baseRate = 8000
	case rt.sameCity:
		rt.baseRate = 9000
	default:
		rt.distKm = int64(math.Round(haversineKm(o.CityLat, o.CityLng, d.CityLat, d.CityLng)))
		var s int64
		if rt.og != rt.dg {
			s = max64(groups[rt.og].remote, groups[rt.dg].remote)
		} else {
			s = groups[rt.og].remote / 2
		}
		dk := rt.distKm
		rt.baseRate = 9000 + 15*min64(dk, 500) + 10*clamp(dk-500, 0, 1000) + 20*max64(dk-1500, 0) + s
	}
	return rt
}

func (rt route) available(c Class) bool {
	switch c {
	case NextDay:
		if rt.sameCity {
			return true
		}
		remote := func(g group) bool { return g == gMaluku || g == gPapua }
		return rt.distKm <= 1500 && !remote(rt.og) && !remote(rt.dg)
	case SameDay:
		return rt.sameCity
	case Trucking:
		return groups[rt.og].cluster == groups[rt.dg].cluster
	}
	return true
}

func (rt route) etd(c Class) string {
	switch c {
	case NextDay:
		return "1 day"
	case SameDay:
		return "0 day"
	}
	var lo, hi int64
	switch {
	case rt.sameDistrict:
		lo, hi = 1, 1
	case rt.sameCity:
		lo, hi = 1, 2
	case rt.distKm <= 200:
		lo, hi = 1, 2
	case rt.distKm <= 500:
		lo, hi = 2, 3
	case rt.distKm <= 1000:
		lo, hi = 2, 4
	case rt.distKm <= 2000:
		lo, hi = 3, 5
	default:
		lo, hi = 4, 7
	}
	if !rt.sameCity {
		extra := int64(max(groups[rt.og].extraDay, groups[rt.dg].extraDay))
		lo += extra
		hi += extra
	}
	switch c {
	case Economy:
		lo, hi = lo+1, hi+1
	case Trucking:
		lo, hi = lo+2, hi+2
	}
	if lo == hi {
		return fmt.Sprintf("%d day", lo)
	}
	return fmt.Sprintf("%d-%d day", lo, hi)
}

// chargeKg applies the 300 g tolerance rule.
func chargeKg(weightGram int) int64 {
	w := int64(weightGram) - 300
	kg := int64(1)
	if w > 0 {
		kg = max64(1, (w+999)/1000)
	}
	return kg
}

// Quote returns all available services of the requested couriers, in request
// order and then catalog order. Courier codes must already be normalized.
func Quote(o, d Location, weightGram int, couriers []string) ([]Result, error) {
	rt := newRoute(o, d)
	baseKg := chargeKg(weightGram)
	var out []Result
	for _, code := range couriers {
		c, ok := LookupCourier(code)
		if !ok {
			return nil, &UnknownCourierError{Code: code}
		}
		for _, svc := range c.Services {
			if !rt.available(svc.Class) {
				continue
			}
			ci := classes[svc.Class]
			kg := max64(baseKg, ci.minKg)
			n := rt.baseRate * c.MultPermille * ci.multPermille
			q := int64(1_000_000 * 500)
			rate := ((n + q - 1) / q) * 500
			out = append(out, Result{
				Name: c.Name, Code: c.Code, Service: svc.Code, Description: svc.Description,
				Cost: rate * kg, ETD: rt.etd(svc.Class),
			})
		}
	}
	return out, nil
}
