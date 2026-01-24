FROM golang:1.22-bookworm AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o tct_scrooper .

FROM debian:bookworm-slim

# Chromium + headless browser deps
RUN apt-get update && apt-get install -y --no-install-recommends \
	chromium \
	libnss3 \
	libatk1.0-0 \
	libatk-bridge2.0-0 \
	libcups2 \
	libdrm2 \
	libxcomposite1 \
	libxdamage1 \
	libxrandr2 \
	libgbm1 \
	libasound2 \
	libpangocairo-1.0-0 \
	libgtk-3-0 \
	fonts-liberation \
	ca-certificates \
	&& rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/tct_scrooper .
COPY --from=builder /app/config ./config

# SQLite DB will be mounted as volume
VOLUME /app/data

ENV DB_PATH=/app/data/scraper.db

CMD ["./tct_scrooper"]
