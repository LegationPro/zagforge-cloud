FROM golang:1.26.1-alpine AS build
RUN apk add --no-cache git curl tar gcompat

# Install zigzag from prebuilt release
ARG TARGETARCH
ARG ZIGZAG_VERSION=v0.15.1
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) ZIGZAG_ARCH="x86_64" ;; \
      arm64) ZIGZAG_ARCH="aarch64" ;; \
      *) echo "unsupported arch: ${TARGETARCH}" && exit 1 ;; \
    esac; \
    curl -fsSL "https://github.com/LegationPro/zigzag/releases/download/${ZIGZAG_VERSION}/zigzag-${ZIGZAG_ARCH}-linux.tar.gz" \
      | tar xz -C /usr/local/bin

WORKDIR /app
COPY go.work go.work.sum ./
COPY api/go.mod api/go.sum ./api/
COPY worker/go.mod worker/go.su[m] ./worker/
COPY shared/go/go.mod shared/go/go.sum ./shared/go/
RUN go work sync
COPY . .

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/worker ./worker/cmd/

FROM alpine:3.21
RUN apk add --no-cache git gcompat ca-certificates
COPY --from=build /usr/local/bin/zigzag /usr/local/bin/zigzag
COPY --from=build /bin/worker /bin/worker
USER nobody
ENTRYPOINT ["/bin/worker"]
