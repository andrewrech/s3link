FROM golang:latest
COPY s3link /
ENTRYPOINT ["/polly"]
