FROM golang:1.25.6 AS build

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .


RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o b0k3ts ./cmd/b0k3ts/main.go

FROM alpine:latest


RUN apk --no-cache add ca-certificates

RUN apk update && apk upgrade

RUN adduser b0k3ts -D

RUN mkdir /var/log/b0k3ts/

RUN mkdir -p /opt/b0k3ts/data/

RUN chown b0k3ts /var/log/b0k3ts/

RUN chown b0k3ts /opt/b0k3ts/data/

WORKDIR /app/

COPY --from=build /app/config.yaml .

COPY --from=build /app/b0k3ts .

RUN cp b0k3ts /usr/bin/

EXPOSE 4201

USER b0k3ts

CMD ["b0k3ts"]

