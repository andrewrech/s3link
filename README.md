[![GoDoc](https://godoc.org/github.com/andrewrech/s3link?status.svg)](https://godoc.org/github.com/andrewrech/s3link) [![](https://goreportcard.com/badge/github.com/andrewrech/s3link)](https://goreportcard.com/report/github.com/andrewrech/s3link) ![](https://img.shields.io/badge/docker-andrewrech/s3link:0.0.7-blue?style=plastic&logo=docker)

# s3link

Upload files from Stdin to AWS S3 and generate an authenticated URL.

## Installation

See [Releases](https://github.com/andrewrech/s3link/releases).

## Usage

See `s3link -h`:

```text
Upload files to AWS S3 and generate authenticated URLs.

Usage of s3link:
  echo 'file.txt' | s3link
  echo 'pre-existing/bucket/key.ext' | s3link

Defaults:
  -expire string
        URL lifetime (default "1m")
  -public
        Create obfuscated public link?
  -qr
        Generate QR code?

Environmental variables:

    export S3LINK_BUCKET=upload-bucket
    export AWS_SHARED_CREDENTIALS_PROFILE=default
    export S3LINK_OBFUSCATION_KEY=key-for-filename-obfuscation
```

## Authors

- [Andrew J. Rech](mailto:rech@rech.io)

## License

GNU Lesser General Public License v3.0
