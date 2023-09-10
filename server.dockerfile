FROM golang:1.19.4

WORKDIR /mafia/
COPY go.mod go.sum /mafia/
COPY protos /mafia/protos/
COPY game /mafia/game/
COPY server /mafia/server/

RUN apt-get update && apt-get install -y protobuf-compiler
RUN go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
ENV PATH /usr/local/go:/go/bin:$PATH

RUN protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative ./protos/connection.proto

WORKDIR /mafia/server
RUN go mod download
RUN go build -o server

ENTRYPOINT ["/mafia/server/server"]
