# syntax=docker/dockerfile:1
#
# The stele-pull worker image: stele-pull-worker, which the CronJob runs, plus the
# stele-pull CLI for inspecting or garbage-collecting the store from a one-off Job.

FROM golang:1.26.6-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

# Static binaries: the runtime image has no libc.
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/stele-pull-worker ./cmd/stele-pull-worker \
 && go build -trimpath -ldflags="-s -w" -o /out/stele-pull ./cmd/stele-pull

# distroless static: CA certificates for Canvas over HTTPS, a nonroot user
# (65532, the CronJob's runAsUser) and no shell. /tmp exists; the CronJob mounts
# an emptyDir there for in-flight downloads.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/stele-pull-worker /out/stele-pull /usr/local/bin/

USER 65532:65532
ENTRYPOINT ["/usr/local/bin/stele-pull-worker"]
CMD ["help"]
