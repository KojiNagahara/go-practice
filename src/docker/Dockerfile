FROM golang:1.27.1 AS build

WORKDIR /src
COPY go/go.mod ./
COPY go/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/go-practice ./cmd/web
RUN mkdir -p /out/uploads

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/go-practice /go-practice
COPY --from=build --chown=nonroot:nonroot /out/uploads /data/uploads
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/go-practice"]
