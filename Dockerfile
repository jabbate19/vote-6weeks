FROM docker.io/golang:1.18-alpine3.15 AS build

WORKDIR /src/
COPY . .
RUN apk add git && \
    go build -v -o vote

FROM docker.io/alpine:3.15
COPY static /static
COPY templates /templates
COPY --from=build /src/vote /vote

ENTRYPOINT [ "/vote" ]
