[![GoDoc](https://godoc.org/github.com/andrewrech/s3link?status.svg)](https://godoc.org/github.com/andrewrech/s3link) ![](https://img.shields.io/badge/version-0.0.6-blue.svg) ![](https://goreportcard.com/badge/github.com/andrewrech/s3link)

# s3link

Upload files from Stdin to AWS S3 and generate an authenticated URL.

## Install

[Releases](https://github.com/andrewrech/s3link/releases)

or

```sh
 brew tap andrewrech/s3link
 brew install s3link
```

or

```sh
go get -u github.com/andrewrech/s3link
```

## Usage

```sh
s3link -h
```

```
Upload files from Stdin to AWS S3 and generate an authenticated URL.

Usage of s3link:
  echo 'file.txt' | s3link
  echo 'pre-existing/bucket/key.ext' | s3link

Defaults:
  -expire string
        URL lifetime (default "1m")
  -public
        Create public link (insecure simple obfuscation)?
  -qr
        Generate QR code? (default true)
  -region string
        AWS region (default "us-east-1")

Environmental variables:

    export S3LINK_BUCKET=bucket
    export S3LINK_PUB_LINK_PREFIX=public-link-obfuscation-prefix

    export S3LINK_AWS_ACCESS_KEY_ID=my_iam_access_key
    export S3LINK_AWS_SECRET_ACCESS_KEY=my_iam_secret
    export S3LINK_AWS_SESSION_TOKEN=my_iam_session_token [optional]
```

## Authors

- [Andrew J. Rech](mailto:rech@rech.io)

## License

GNU Lesser General Public License v3.0
