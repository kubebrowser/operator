FROM golang:1.24 AS build-stage

WORKDIR /src

COPY go.mod go.sum ./

RUN go mod download

COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /dist/ -a -installsuffix cgo github.com/kubebrowser/operator/cmd/browser-api

FROM redhat/ubi9-minimal:9.6 AS serve-stage

WORKDIR /app

COPY  --from=build-stage /dist/browser-api .

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build-stage /dist/browser-api .
USER 65532:65532
CMD ["/browser-api"]
EXPOSE 8443
