FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /mock-upf .

FROM alpine:3.22
COPY --from=build /mock-upf /usr/local/bin/mock-upf
EXPOSE 8080
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/mock-upf"]
