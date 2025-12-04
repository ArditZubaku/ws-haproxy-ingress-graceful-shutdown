FROM golang:1.24.3-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o /app

FROM alpine:3.22.2
# Add a version to curl
RUN apk --no-cache add curl=8.14.1-r2
COPY --from=build /app /app
EXPOSE 8080
EXPOSE 9999
CMD ["/app"]
