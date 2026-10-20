FROM golang:1.27.3-alpine AS build

WORKDIR /src

COPY . .

RUN CGO_ENABLED=0 go build -trimpath -o /out/arbiterer ./cmd/arbiterer

FROM alpine:3.24

RUN apk add --no-cache ca-certificates \
    && adduser -D -H -u 10001 arbiterer

COPY --from=build /out/arbiterer /usr/local/bin/arbiterer

USER arbiterer

ENTRYPOINT ["arbiterer", "serve"]
