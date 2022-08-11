# syntax = docker/dockerfile:1.2
FROM docker.io/golang:1.18.4-alpine3.16 AS build

WORKDIR /src/
RUN apk add git
COPY . .
RUN --mount=type=cache,target=/go go build -v -o vote

FROM docker.io/alpine:3.16
COPY static /static
COPY templates /templates
COPY --from=build /src/vote /vote

ENTRYPOINT [ "/vote" ]
