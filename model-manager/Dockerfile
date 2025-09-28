FROM docker.io/golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/load  load.go
RUN go build -trimpath -ldflags="-s -w" -o /out/probe probe.go

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /out/load  /load
COPY --from=build /out/probe /probe
USER nonroot:nonroot
ENTRYPOINT ["/load"]

