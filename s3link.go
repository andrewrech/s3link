package main

import (
	"bufio"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
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
	expire := flag.String("expire", "1m", "URL lifetime")
	qr := flag.Bool("qr", false, "Generate QR code?")
	public := flag.Bool("public", false, "Create obfuscated public link?")
	flag.Parse()

	minutes := checkDuration(expire)

	vars, err := loadVars()
	if err != nil {
		log.Fatalln(err)
	}

	conn, uploader := connect(vars)

	in, readDone := read(os.Stdin)

	toURL, upDone := upload(vars, uploader, conn, in, public)

	urlDone := url(conn, toURL, minutes, public, qr)

	<-readDone
	<-upDone
	<-urlDone
}

// usage prints package usage.
func usage() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "\nUpload files to AWS S3 and generate authenticated URLs.\n")
		fmt.Fprintf(os.Stderr, "\nUsage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  echo 'file.txt' | s3link\n")
		fmt.Fprintf(os.Stderr, "  echo 'pre-existing/bucket/key.ext' | s3link\n")
		fmt.Fprintf(os.Stderr, "\nDefaults:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Environmental variables:

    export S3LINK_BUCKET=upload-bucket
    export AWS_SHARED_CREDENTIALS_PROFILE=default
    export S3LINK_OBFUSCATION_KEY=key-for-filename-obfuscation

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
		log.Fatalln("expiration duration is greater than maximum expiration duration of 7 days (10800 minutes)")
	}

	return &m
}

// connect reads shared AWS credentials and returns a connection.
func connect(vars envVars) (conn *s3.S3, uploader *s3manager.Uploader) {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Profile:           vars.credentialProfile,
	}))

	conn = s3.New(sess)

	n := runtime.GOMAXPROCS(0)
	uploader = s3manager.NewUploader(sess, func(u *s3manager.Uploader) {
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

// randomBytes generates n random bytes
func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		log.Fatalln(err)
	}
	return b
}

// randomHex generates a random hex string with length of n
// e.g: 67aab2d956bd7cc621af22cfb169cba8
func randomHex(n int) string { return hex.EncodeToString(randomBytes(n)) }

// key generates an AWS S3 key.
func key(name string) string {
	p, ok := os.LookupEnv("S3LINK_OBFUSCATION_KEY")
	if !ok {
		p = randomHex(64)
	}

	var b strings.Builder

	// create hash from obfuscation key and filename
	b.WriteString(p)
	b.WriteString(filepath.Base(name))
	input := []byte(b.String())
	hash := sha512.Sum512(input)
	output := base64.StdEncoding.EncodeToString(hash[:])

	b.Reset()

	b.WriteString(output)
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

		k := key(fn)

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

type envVars struct {
	credentialProfile string
	bucket            string
}

// loadVars loads environmental variables.
func loadVars() (vars envVars, err error) {
	credentialProfile, ok := os.LookupEnv("AWS_SHARED_CREDENTIALS_PROFILE")
	if !ok {
		credentialProfile = "default"
	}

	bucket, ok := os.LookupEnv("S3LINK_BUCKET")
	if !ok {
		log.Fatalln("S3LINK_BUCKET is unset")
	}

	vars.credentialProfile = credentialProfile
	vars.bucket = bucket

	return vars, nil
}
