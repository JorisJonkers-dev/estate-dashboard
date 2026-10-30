# syntax=docker/dockerfile:1.28
# One image: the Go binary serves the API and embeds the web build (web/embed.go).

FROM --platform=$BUILDPLATFORM node:24.22.1-alpine AS web
RUN npm install -g pnpm@12.6.0
WORKDIR /src
COPY .npmrc package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY openapi/package.json openapi/package.json
COPY web/package.json web/package.json
# The shared theme is on GitHub Packages; the token is a build secret, never a layer.
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    --mount=type=secret,id=github_token,required=true \
    printf '//npm.pkg.github.com/:_authToken=%s\n' "$(cat /run/secrets/github_token)" > ~/.npmrc && \
    pnpm install --frozen-lockfile --filter @estate-dashboard/web... && \
    rm ~/.npmrc
COPY openapi openapi
COPY web web
RUN pnpm --filter @estate-dashboard/web build

# Cross-compiles on the build platform, so a multi-arch build needs no emulation for the Go stage.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=web /src/web/dist web/dist
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/estate-dashboard ./cmd/estate-dashboard

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/estate-dashboard /estate-dashboard
EXPOSE 8080
# distroless's nonroot, by number: the estate's images lock records the uid and gid an image
# states, and a name states neither.
USER 65532:65532
ENTRYPOINT ["/estate-dashboard"]
