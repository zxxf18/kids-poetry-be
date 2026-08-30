FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/kids-poetry-api ./cmd/server \
    && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/kids-poetry-importer ./cmd/importer \
    && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/kids-poetry-minio-sync ./cmd/minio-sync

FROM alpine:3.22
RUN addgroup -S poetry && adduser -S -G poetry poetry
WORKDIR /app
COPY --from=builder /out/kids-poetry-api /app/server
COPY --from=builder /out/kids-poetry-importer /app/importer
COPY --from=builder /out/kids-poetry-minio-sync /app/minio-sync
COPY etc/backend.docker.yaml /app/etc/backend.yaml
USER poetry
EXPOSE 8890
ENTRYPOINT ["/app/server"]
CMD ["-f", "/app/etc/backend.yaml"]
