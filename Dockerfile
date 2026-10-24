# syntax=docker/dockerfile:1

# ---------- client ----------
FROM node:22-alpine AS client
WORKDIR /src/client
COPY client/package.json client/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY client/ ./
RUN npm run build

# ---------- server ----------
FROM golang:1.24-alpine AS server
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/killa ./cmd/killa

# ---------- runtime ----------
FROM alpine:3.20
RUN adduser -D -H -u 10001 killa
WORKDIR /app
COPY --from=server /out/killa /app/killa
COPY --from=client /src/client/dist /app/client/dist
USER killa
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/healthz > /dev/null || exit 1
STOPSIGNAL SIGTERM
ENTRYPOINT ["/app/killa"]
CMD ["-addr", ":8080", "-static", "/app/client/dist"]
