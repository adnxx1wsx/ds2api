FROM node:20 AS webui-builder

WORKDIR /app/webui
COPY webui/package.json webui/package-lock.json ./
RUN npm ci
COPY webui ./
RUN npm run build

FROM golang:1.25 AS go-builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/ds2api ./cmd/ds2api

FROM debian:bookworm-slim
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates wget && rm -rf /var/lib/apt/lists/*
COPY --from=go-builder /out/ds2api /usr/local/bin/ds2api
COPY --from=go-builder /app/sha3_wasm_bg.7b9ca65ddd.wasm /app/sha3_wasm_bg.7b9ca65ddd.wasm
COPY --from=go-builder /app/config.example.json /app/config.example.json
COPY --from=webui-builder /app/static/admin /app/static/admin
EXPOSE 5001
CMD ["/usr/local/bin/ds2api"]
