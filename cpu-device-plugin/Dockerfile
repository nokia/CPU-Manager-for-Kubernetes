FROM golang AS build-env
ENV GOPATH /go
RUN mkdir -p ${GOPATH}/src/cpu-device-plugin
WORKDIR $GOPATH/src/cpu-device-plugin
COPY glide.yaml .
COPY glide.lock .
RUN curl https://glide.sh/get | sh
RUN glide install --strip-vendor
COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -a  -ldflags '-extldflags "-static"' -o cpu-device-plugin


FROM centos:latest

COPY --from=build-env /go/src/cpu-device-plugin/cpu-device-plugin .
ENTRYPOINT ["/cpu-device-plugin"]

