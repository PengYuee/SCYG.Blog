# syntax=docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e
# Go 构建镜像固定版本和官方多架构摘要。
FROM golang:1.26.8-bookworm@sha256:dc9ad6c05acc7a88e5b71bde60a5fe3bd4b9f0db209011711b464107438a8107 AS build
WORKDIR /repo/backend
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY contracts /repo/contracts
RUN --mount=type=cache,target=/go/pkg/mod go run github.com/bufbuild/buf/cmd/buf@v1.47.2 generate /repo/contracts --template /repo/contracts/buf.gen.go.yaml --output /repo/backend/internal/generated/proto
COPY backend/api ./api
COPY backend/cmd ./cmd
COPY backend/internal ./internal
COPY backend/migrations ./migrations
COPY backend/oapi-codegen.yaml ./
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/healthcheck ./cmd/healthcheck
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate
RUN mkdir -p /out/article-images

# Distroless 无 shell、包管理器、Go 工具链、源码或构建缓存。
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
WORKDIR /app
COPY --from=build --chown=65532:65532 /out/api /api
COPY --from=build --chown=65532:65532 /out/healthcheck /healthcheck
COPY --from=build --chown=65532:65532 /out/migrate /migrate
COPY --from=build --chown=65532:65532 /out/article-images /app/data/article-images
USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=5s --start-period=10s --retries=5 CMD ["/healthcheck"]
ENTRYPOINT ["/api", "-config="]
