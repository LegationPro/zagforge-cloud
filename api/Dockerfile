FROM golang:1.26.1-alpine AS build
RUN apk add --no-cache git

WORKDIR /app
COPY go.work go.work.sum ./
COPY api/go.mod api/go.sum ./api/
COPY worker/go.mod worker/go.su[m] ./worker/
COPY shared/go/go.mod shared/go/go.sum ./shared/go/
RUN go work sync
COPY . .

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/server ./api/cmd/

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /bin/server /bin/server
EXPOSE 8080
ENTRYPOINT ["/bin/server"]
