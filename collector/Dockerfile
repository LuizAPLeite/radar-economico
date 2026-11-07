# Build em duas etapas: a primeira tem o compilador Go; a segunda só leva o
# binário, resultando numa imagem pequena e sem shell (menos superfície de ataque).
FROM golang:1.27-alpine AS build
WORKDIR /src

# Copia go.mod/go.sum antes do código: o Docker reaproveita a camada de
# dependências enquanto elas não mudarem, e o build fica bem mais rápido.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO_ENABLED=0 gera um binário estático, que roda sem bibliotecas do sistema.
RUN CGO_ENABLED=0 GOWORK=off go build -trimpath -ldflags="-s -w" -o /out/collector .

# distroless/static: só certificados TLS (para HTTPS) e um usuário sem privilégios.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/collector /collector
ENTRYPOINT ["/collector"]
