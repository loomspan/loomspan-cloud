FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/management-backend ./cmd/management-backend

FROM build AS probe-build
RUN CGO_ENABLED=0 go test -c -trimpath -tags=k3s_integration -o /out/k3s-engine-test ./internal/engine

FROM scratch AS observation-probe
COPY --from=probe-build /out/k3s-engine-test /app/k3s-engine-test
USER 10001:10001
ENTRYPOINT ["/app/k3s-engine-test"]

FROM scratch AS backend
COPY --from=build /out/management-backend /app/management-backend
USER 10001:10001
ENTRYPOINT ["/app/management-backend"]
