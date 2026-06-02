# Build (produção): binário estático Go
FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.mod ./
# COPY go.sum ./        # descomentar quando houver dependências externas
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/server ./cmd/server

# Imagem final mínima
FROM gcr.io/distroless/static-debian12
COPY --from=build /bin/server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
