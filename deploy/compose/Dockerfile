FROM golang:1.25.4 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/bin-eval-api ./cmd/bin-eval-api && \
    CGO_ENABLED=0 go build -trimpath -o /out/bin-eval-worker ./cmd/bin-eval-worker && \
    CGO_ENABLED=0 go build -trimpath -o /out/bin-eval-llm-fixture ./cmd/bin-eval-llm-fixture

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ /
COPY --from=build /src/migrations /migrations
ENTRYPOINT ["/bin-eval-api"]
