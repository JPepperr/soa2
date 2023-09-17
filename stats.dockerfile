FROM golang:1.19.4

WORKDIR /mafia/
COPY . /mafia/

WORKDIR /mafia/stats
RUN go mod download
RUN go build -o stats

ENTRYPOINT ["/mafia/stats/stats"]
