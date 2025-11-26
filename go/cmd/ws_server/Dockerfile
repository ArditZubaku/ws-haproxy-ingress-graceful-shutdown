FROM golang:1.24.3-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o /app

FROM scratch
COPY --from=build /app /app
EXPOSE 8080
CMD ["/app"]

