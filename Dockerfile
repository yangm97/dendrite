FROM golang:alpine3.6 as builder
RUN apk add --no-cache git && \
    go get github.com/constabulary/gb/...
COPY vendor /go/vendor
COPY src /go/src
RUN gb build

FROM alpine:3.6
EXPOSE 8008 8448
ENTRYPOINT ["dendrite-monolith-server"]
COPY --from=builder /go/bin/* /usr/local/bin/
