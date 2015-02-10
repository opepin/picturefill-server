# Start from a Debian image with the latest version of Go installed
# and a workspace (GOPATH) configured at /go.
FROM golang

MAINTAINER opepin@gmail.com

RUN apt-get update
ENV REFRESHED_AT 2014-11-27


# Image Magic
RUN apt-get install -y libmagickwand-dev

RUN go get github.com/gographics/imagick/imagick

# Build the outyet command inside the container.
# (You may fetch or manage dependencies here,
# either manually or with a tool like "godep".)
RUN go get code.google.com/p/gorest

# Run the outyet command by default when the container starts.
ENTRYPOINT go run /go/src/app/api.go 

# Document that the service listens on port 8080.
EXPOSE 8787

# Copy the local package files to the container's workspace.
ADD app /go/src/app
ADD images /var/run/images
ADD tmp /tmp/var/run/images/
