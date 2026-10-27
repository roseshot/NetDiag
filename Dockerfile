FROM golang:1.27.3-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /netdiag ./cmd/netdiag

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /netdiag /netdiag
EXPOSE 8080
ENV PORT=8080
USER nonroot:nonroot
ENTRYPOINT ["/netdiag"]
