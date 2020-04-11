# Build the manager binary
FROM golang:latest as builder

WORKDIR /workspace
# Env use china proxy
ENV GO111MODULE=on
ENV GOPROXY=https://goproxy.cn,https://goproxy.io,https://mirrors.aliyun.com/goproxy/,https://athens.azurefd.net,direct
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
# Alpine does not support dynamic library linking , so disable cgo compiled and add ldflags
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build --ldflags "-extldflags -static" -o manager main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM alpine:latest
WORKDIR /
RUN mkdir /metrics
COPY --from=builder /workspace/manager .

ENTRYPOINT ["/manager"]
