package main

import (
	"bufio"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	qrcode "github.com/skip2/go-qrcode"
)

// Upload files from Stdin to AWS S3 and generate an authenticated URL.
// Usage:
//    echo "bucket/key.txt" | s3link
//    ls *pdf               | s3link -bucket myBucket
func main() {
	usage()

	// parse command line options
	region := flag.String("region", "us-east-1", "AWS region")
	expire := flag.String("expire", "1m", "URL lifetime")
	qr := flag.Bool("qr", true, "Generate QR code?")
	public := flag.Bool("public", false, "Create public link (insecure simple obfuscation)?")
	flag.Parse()

	vars, err := loadVars()
	if err != nil {
		log.Fatalln(err)
	}

	minutes := checkDuration(expire)

	conn, uploader := connect(vars, region)

	in, readDone := read(os.Stdin)

	toURL, upDone := upload(vars, uploader, conn, in, public)

	urlDone := url(conn, toURL, minutes, public, qr)

	<-readDone
	<-upDone
	<-urlDone
}

type envVars struct {
	bucket string
	prefix string
	id     string
	secret string
	token  string
}

// loadVars loads required environmental variables.
func loadVars() (vars envVars, err error) {
	b, ok := os.LookupEnv("S3LINK_BUCKET")
	if !ok {
		return vars, errors.New("S3LIMK_BUCKET is unset")
	}

	prefix, ok := os.LookupEnv("S3LINK_PUB_LINK_PREFIX")
	if !ok {
		return vars, errors.New("S3LINK_PUB_LINK_PREFIX is unset")
	}

	id, ok := os.LookupEnv("S3LINK_AWS_ACCESS_KEY_ID")
	if !ok {
		log.Fatalln()
		return vars, errors.New("S3LINK_AWS_ACCESS_KEY_ID is unset")
	}

	secret, ok := os.LookupEnv("S3LINK_AWS_SECRET_ACCESS_KEY")
	if !ok {
		return vars, errors.New("S3LINK_AWS_SECRET_ACCESS_KEY is unset")
	}

	token, _ := os.LookupEnv("S3LINK_AWS_SESSION_TOKEN")
	if token != "" {
		log.Println("Using temporary AWS credentials.")
	}

	vars.bucket = b
	vars.prefix = prefix
	vars.id = id
	vars.secret = secret
	vars.token = token

	return vars, nil
}

// usage prints package usage.
func usage() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "\nUpload files from Stdin to AWS S3 and generate an authenticated URL.\n")
		fmt.Fprintf(os.Stderr, "\nUsage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  echo 'file.txt' | s3link\n")
		fmt.Fprintf(os.Stderr, "  echo 'pre-existing/bucket/key.ext' | s3link\n")
		fmt.Fprintf(os.Stderr, "\nDefaults:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Environmental variables:

    export S3LINK_BUCKET=bucket
    export S3LINK_PUB_LINK_PREFIX=public-link-obfuscation-prefix

    export S3LINK_AWS_ACCESS_KEY_ID=my_iam_access_key
    export S3LINK_AWS_SECRET_ACCESS_KEY=my_iam_secret
    export S3LINK_AWS_SESSION_TOKEN=my_iam_session_token [optional]

`)
	}
}

// checkDuration checks the link expiration duration for correctness.
func checkDuration(expire *string) (minutes *time.Duration) {
	m, err := time.ParseDuration(*expire)
	if err != nil {
		log.Fatalln(err)
	}

	// maximum version 4 link authentication is 7 days
	maxTime, _ := time.ParseDuration("10800m")

	if m > maxTime {
		log.Fatalln("Expiration duration is greater than maximum expiration duration of 7 days (10800 minutes)")
	}

	return &m
}

// connect reads AWS credentials from the environment and returns a connection.
func connect(vars envVars, region *string) (conn *s3.S3, uploader *s3manager.Uploader) {
	config := aws.Config{
		Region:      aws.String(*region),
		Credentials: credentials.NewStaticCredentials(vars.id, vars.secret, vars.token),
	}

	// create S3 upload manager
	s := session.Must(session.NewSession(&config))
	conn = s3.New(s)

	n := runtime.GOMAXPROCS(0)
	uploader = s3manager.NewUploader(s, func(u *s3manager.Uploader) {
		u.Concurrency = n
		u.MaxUploadParts = n
		u.LeavePartsOnError = true
	})

	return conn, uploader
}

// reader reads lines from Stdin, removes ANSI codes, and passes the resulting strings to a channel.
func read(in io.Reader) (out chan string, done chan int) {
	done = make(chan int)

	var buf int64 = 1e6
	out = make(chan string, buf)

	// remove ANSI codes
	ansi := "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"

	re := regexp.MustCompile(ansi)

	r := bufio.NewReader(in)

	go func() {
		for {
			l, err := r.ReadString('\n')
			if err != nil {
				break
			}

			linePlain := re.ReplaceAllString(l, "")
			out <- linePlain
		}

		close(out)
		done <- 1
	}()

	return out, done
}

// url manages parallel URL creation.
func url(conn *s3.S3, in chan string, minutes *time.Duration, public, qr *bool) (done chan int) {
	done = make(chan int)

	signal := make(chan int, runtime.GOMAXPROCS(0))

	go func() {
		for i := 0; i < runtime.GOMAXPROCS(0); i++ {
			go urlLine(conn, in, minutes, public, qr, signal)
		}

		for i := 0; i < runtime.GOMAXPROCS(0); i++ {
			<-signal
		}

		done <- 1
	}()

	return done
}

// urlLine generates a pre-signed AWS S3 URL.
func urlLine(conn *s3.S3, in chan string, minutes *time.Duration, public, qr *bool, done chan int) {
	for l := range in { // for each incoming value
		// extract bucket, key
		b, k, err := parse(l)
		if err != nil {
			log.Fatalln(err)
		}

		var url string

		if *public {
			// construct public URL
			var p strings.Builder

			p.WriteString("https://")
			p.WriteString(b)
			p.WriteString(".s3.amazonaws.com/")
			p.WriteString(k)

			url = p.String()
		} else {
			// get auth URL
			req, _ := conn.GetObjectRequest(&s3.GetObjectInput{
				Bucket: aws.String(b),
				Key:    aws.String(k),
			})
			url, err = req.Presign(*minutes)
			if err != nil {
				log.Fatalln(err)
			}
		}

		fmt.Println(url)

		if *qr {
			q, err := qrcode.New(url, qrcode.Low)
			if err != nil {
				log.Fatalln(err)
			}

			l := log.New(os.Stderr, "", 0)
			l.Println(q.ToSmallString(false))
		}
	}
	done <- 1
}

// transformPrefix generates a hexadecimal encoded SHA-512 string using a
// filename and the value of S3LINK_PREFIX_HASH.
// The purpose is to use the string in an obfuscated public URL.
func transformPrefix(name, h *string) (p string) {
	var x strings.Builder

	x.WriteString(*h)
	x.WriteString(*name)

	sha := sha512.Sum512([]byte(x.String()))

	return (hex.EncodeToString(sha[:]))
}

// key generates an AWS S3 key.
func key(name string, pre *string) string {
	p := transformPrefix(&name, pre)

	var b strings.Builder

	b.WriteString(p)
	b.WriteString("/")
	b.WriteString(filepath.Base(name))

	k := b.String()

	return (k)
}

// parse parses a line of text to a bucket/key combination.
func parse(l string) (b, k string, err error) {
	bPat := regexp.MustCompile("^[^/]+")

	keyPat := regexp.MustCompile("/.*")

	// remove newline
	l = strings.Replace(l, "\n", "", -1)

	b = bPat.FindStringSubmatch(l)[0]
	if len(b) == 0 {
		return "", "", errors.New("cannot extract bucket value")
	}

	k = strings.Trim(keyPat.FindStringSubmatch(l)[0], "/")

	if len(k) == 0 {
		return "", "", errors.New("cannot extract bucket value")
	}

	return b, k, nil
}

// acl assigns a private or public acl to an AWS S3 object.
func acl(conn *s3.S3, b, k string, public bool) {
	var acl s3.PutObjectAclInput

	if !public {
		acl =
			s3.PutObjectAclInput{
				ACL:    aws.String("private"),
				Bucket: &b,
				Key:    aws.String(k),
			}
	} else {
		acl =
			s3.PutObjectAclInput{
				ACL:    aws.String("public-read"),
				Bucket: &b,
				Key:    aws.String(k),
			}
	}

	_, err := conn.PutObjectAcl(&acl)
	if err != nil {
		log.Fatalln(err)
	}
}

// upload manages parallel uploads.
func upload(vars envVars, uploader *s3manager.Uploader, conn *s3.S3, in chan string, public *bool) (out chan string, done chan int) {
	done = make(chan int)

	signal := make(chan int, runtime.GOMAXPROCS(0))

	var buf int64 = 1e6
	out = make(chan string, buf)

	go func() {
		for i := 0; i < runtime.GOMAXPROCS(0); i++ {
			go uploadLine(vars, uploader, conn, in, out, public, signal)
		}

		for i := 0; i < runtime.GOMAXPROCS(0); i++ {
			<-signal
		}

		close(out)

		done <- 1
	}()

	return out, done
}

// uploadLine uploads files interpreted as strings from a
// channel to AWS S3 if the files exist.
func uploadLine(vars envVars, uploader *s3manager.Uploader, conn *s3.S3, in, out chan string, public *bool, done chan int) {
	for l := range in { // for each incoming value
		fn := strings.TrimSuffix(l, "\n")

		if _, err := os.Stat(fn); os.IsNotExist(err) {
			out <- l

			continue
		}

		f, err := os.Open(fn)
		if err != nil {
			log.Fatalln(err)
		}

		k := key(fn, &vars.prefix)

		// print link before finishing upload
		var i strings.Builder

		i.WriteString(vars.bucket)
		i.WriteString("/")
		i.WriteString(k)
		out <- i.String()

		m := mime.TypeByExtension(filepath.Ext(fn))

		// create UploadInput
		up := s3manager.UploadInput{
			Bucket:      &vars.bucket,
			Key:         aws.String(k),
			ContentType: &m,
			Body:        f,
		}

		// upload
		_, err = uploader.Upload(&up)
		if err != nil {
			log.Fatalln(err)
		}

		acl(conn, vars.bucket, k, *public)

		err = f.Close()
		if err != nil {
			log.Fatalln(err)
		}
	}
	done <- 1
}
