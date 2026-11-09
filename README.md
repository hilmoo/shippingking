# ShippingKing

[![CI/CD](https://github.com/hilmoo/shippingking/actions/workflows/ci.yml/badge.svg)](https://github.com/hilmoo/shippingking/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A local mock of the RajaOngkir (Komerce v1) Shipping Cost API. Region names and postal codes are real
([cahyadsn/wilayah](https://github.com/cahyadsn/wilayah), [wilayah_kodepos](https://github.com/cahyadsn/wilayah_kodepos));
shipping prices are **dummy but deterministic** and must not be shown to end users as real tariffs.

Switch a client by changing only the base URL: `https://rajaongkir.komerce.id` → `http://localhost:8080`.

## Run

```
go run ./cmd/server
```

| env                  | default | meaning                                                          |
|----------------------|---------|------------------------------------------------------------------|
| `PORT`               | `8080`  | listen port                                                      |
| `RAJAONGKIR_API_KEY` | (empty) | if set, the `key` header must equal it; else any non-empty key works |

### Docker

```
docker run --rm -p 8080:8080 -e RAJAONGKIR_API_KEY=dev ghcr.io/hilmoo/shippingking:latest
```

The image is distroless (`static-debian12:nonroot`). Its `HEALTHCHECK` runs `/server -healthcheck`,
which probes the public `GET /healthz` endpoint (no key needed). Build locally with `docker build -t shippingking .`.

Tests: `go test ./...`. Regenerate the dataset (optional, output is committed):
`go run ./cmd/gendata` (add `-src <dir>` to use local copies of the pinned SQL files).

## Examples

```
curl -H 'key: dev' http://localhost:8080/api/v1/destination/province
curl -H 'key: dev' http://localhost:8080/api/v1/destination/city/6
curl -H 'key: dev' http://localhost:8080/api/v1/destination/district/152
curl -H 'key: dev' http://localhost:8080/api/v1/destination/sub-district/2100
curl -H 'key: dev' 'http://localhost:8080/api/v1/destination/domestic-destination?search=ragunan&limit=5'
curl -H 'key: dev' 'http://localhost:8080/api/v1/destination/domestic-destination?search=12550'

# search-based: subdistrict IDs
curl -H 'key: dev' -d 'origin=<id>&destination=<id>&weight=1000&courier=jne:sicepat&price=lowest' \
     http://localhost:8080/api/v1/calculate/domestic-cost

# step-by-step: district IDs
curl -H 'key: dev' -d 'origin=<id>&destination=<id>&weight=1000&courier=jne:jnt' \
     http://localhost:8080/api/v1/calculate/district/domestic-cost
```

IDs are 1-based per level and stable (the source data is pinned). See `PLAN.md` for the full spec.

Unknown paths outside `/api/` return an HTML 404 page; unknown `/api/` paths return the JSON envelope.

## Credits and license

- Region data: [cahyadsn/wilayah](https://github.com/cahyadsn/wilayah) and
  [cahyadsn/wilayah_kodepos](https://github.com/cahyadsn/wilayah_kodepos) (MIT).
- Not affiliated with RajaOngkir or Komerce.
- Source: https://github.com/hilmoo/shippingking, made by hilmi, [hilmo.dev](https://hilmo.dev).

Licensed under the [MIT License](LICENSE).
