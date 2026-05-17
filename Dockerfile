# check=skip=InvalidDefaultArgInFrom
ARG TOOLCHAIN_IMAGE
FROM ${TOOLCHAIN_IMAGE} AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY buf.gen.yaml ./
COPY proto ./proto
COPY cmd ./cmd
COPY internal ./internal

RUN rm -rf gen/grpc \
	&& if [ -n "$(find proto -name '*.proto' 2>/dev/null)" ]; then buf generate; fi

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
	go build -trimpath -ldflags="-s -w" -o /out/saga-sandbox ./cmd/server

FROM alpine:3.22@sha256:310c62b5e7ca5b08167e4384c68db0fd2905dd9c7493756d356e893909057601

RUN apk add --no-cache ca-certificates tzdata \
	&& addgroup -S app \
	&& adduser -S -G app app

WORKDIR /app

COPY --from=builder /out/saga-sandbox /app/saga-sandbox
COPY --chown=app:app config.example.yml /app/config.yml

USER app

EXPOSE 50051

ENTRYPOINT ["/app/saga-sandbox"]
