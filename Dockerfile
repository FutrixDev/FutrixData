FROM golang:1.24.3 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/futrixdata-http ./cmd/http

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /app
COPY --from=build /out/futrixdata-http /app/futrixdata-http
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/futrixdata-http"]
