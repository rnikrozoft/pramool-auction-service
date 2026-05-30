# Pramool Auction Service

Microservice for seller auction creation/list/detail and local image upload serving.

Database migrations live in **`pramool-core/migrations/auction/`** — run from pramool-core: `go run . migrate --db auction` (or `--db all`).

## Endpoints
- `POST /seller/auctions`
- `GET /seller/auctions`
- `GET /auctions/:id`
- `GET /uploads/*`
