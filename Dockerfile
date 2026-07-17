FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm install
COPY web/ .
RUN npm run build

FROM golang:1.23-alpine AS build
RUN apk add --no-cache gcc musl-dev
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o /out/stackhost ./cmd/stackhost

FROM alpine:3.20
RUN addgroup -S stackhost && adduser -S -G stackhost stackhost
WORKDIR /app
COPY --from=build /out/stackhost .
COPY --from=web /src/web/dist ./web/dist
RUN mkdir data && chown -R stackhost:stackhost /app
USER stackhost
EXPOSE 8080
VOLUME /app/data
HEALTHCHECK CMD wget -qO- http://127.0.0.1:8080/health/live || exit 1
ENTRYPOINT ["/app/stackhost"]
