FROM golang AS build-env
ENV GOPATH /go
RUN curl -fsSL -o /usr/local/bin/dep https://github.com/golang/dep/releases/download/v0.5.0/dep-linux-amd64 && chmod +x /usr/local/bin/dep
RUN mkdir -p ${GOPATH}/src/cpu-device-plugin
WORKDIR $GOPATH/src/cpu-device-plugin
ADD Gopkg.toml Gopkg.lock ./
RUN dep ensure --vendor-only
ADD *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -a  -ldflags '-extldflags "-static"' -o cpu-device-plugin


FROM centos:latest

COPY --from=build-env /go/src/cpu-device-plugin/cpu-device-plugin .
ENTRYPOINT ["/cpu-device-plugin"]

