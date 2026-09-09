# syntax=docker/dockerfile:1

# Building needs cgo, because the Swiss Ephemeris C sources are compiled into
# the binary rather than linked from a system library. Linking statically
# against musl leaves a binary with no runtime dependency at all, which is what
# lets the image it ships in be empty.
FROM golang:1.27-alpine AS build

RUN apk add --no-cache gcc musl-dev

WORKDIR /src

# The module declares no dependencies beyond the standard library. Copying the
# manifest on its own first keeps that true: if one ever appears, it is
# resolved in a layer that source changes do not invalidate.
COPY go.mod ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=1 go build \
      -trimpath \
      -ldflags "-s -w -linkmode external -extldflags '-static' -X github.com/casbek/api-swiss-ephemeris/internal/version.Version=${VERSION}" \
      -o /out/api ./cmd/api

# A binary that turned out to be dynamically linked would build cleanly, ship
# cleanly, and then die on start with "no such file or directory", which names
# the missing loader rather than anything a reader would recognise. Catching it
# here says what actually went wrong.
RUN linkage=$(ldd /out/api 2>&1 || true); \
    case "$linkage" in \
      *"Not a valid dynamic program"*|*"not a dynamic executable"*) ;; \
      *) echo "the binary is dynamically linked and cannot run on scratch:"; \
         echo "$linkage"; exit 1 ;; \
    esac

# The image holds the binary and the ephemeris files and nothing else: no
# shell, no package manager, no libc, no CA bundle. There is nothing in it to
# exploit and nothing in it to keep patched.
FROM scratch

COPY --from=build /out/api /api
COPY ephe /ephe

# nobody, which does not exist in this image. That is the point: the process
# runs as an account with no home, no shell and nothing to take over.
USER 65534:65534

ENV APP_ENV=production \
    LISTEN_ADDR=0.0.0.0:8080 \
    EPHE_PATH=/ephe \
    LOG_FORMAT=json

EXPOSE 8080

# The health endpoint runs a real calculation, so a passing check means the
# ephemeris files are being read rather than only that the process started.
# There is no shell here to run curl, so the binary probes itself.
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/api", "--healthcheck"]

ENTRYPOINT ["/api"]
