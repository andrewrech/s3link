[![GoDoc](https://godoc.org/github.com/andrewrech/s3link?status.svg)](https://godoc.org/github.com/andrewrech/s3link) [![](https://goreportcard.com/badge/github.com/andrewrech/s3link)](https://goreportcard.com/report/github.com/andrewrech/s3link) ![](https://img.shields.io/badge/docker-andrewrech/s3link:0.0.7-blue?style=plastic&logo=docker)

# s3link

Upload files from Stdin to AWS S3 and generate an authenticated URL.

## Installation

See [Releases](https://github.com/andrewrech/s3link/releases).

```zsh
go get -u -v github.com/andrewrech/s3link
```

## Usage

See `s3link -h` or [documentation](https://github.com/andrewrech/s3link/blob/main/docs.md)).

## Testing

```zsh
git clone https://github.com/andrewrech/s3link &&
cd s3link

go test
```

## Authors

- [Andrew J. Rech](mailto:rech@rech.io)

## License

GNU Lesser General Public License v3.0
