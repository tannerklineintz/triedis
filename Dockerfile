# go build
FROM golang:1.24 AS builder
WORKDIR /usr/src/
COPY . .
RUN CGO_ENABLED=0 go build -v -o triedis

# small secure image
FROM gcr.io/distroless/static:nonroot
EXPOSE 6379
COPY --from=builder /usr/src/triedis /
ENTRYPOINT [ "/triedis" ]