package main

import (
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func mainInner() {
	f, err := os.Open("testdata/test_links.txt")
	if err != nil {
		log.Fatalln(err)
	}

	// load command line flags

	x := "1m"
	expire := &x

	q := true
	qr := &q

	p := false
	public := &p

	// begin test

	vars, err := loadVars()
	if err != nil {
		log.Fatalln(err)
	}

	in, readDone := read(f)

	minutes := checkDuration(expire)

	conn, uploader := connect(vars)

	toURL, upDone := upload(vars, uploader, conn, in, public)

	urlDone := url(conn, toURL, minutes, public, qr)

	<-readDone
	<-upDone
	<-urlDone
}

func TestMain(t *testing.T) {
	t.Run("main", func(t *testing.T) {
		mainInner()
	})
}

func BenchmarkMain(b *testing.B) {
	for i := 0; i < b.N; i++ {
		mainInner()
	}
}

func TestNameParse(t *testing.T) {
	tests := map[string]struct {
		input string
		want  []string
	}{
		"simple":             {input: "a/b", want: []string{"a", "b"}},
		"underscore":         {input: "a_a/b_b", want: []string{"a_a", "b_b"}},
		"numbers":            {input: "123/456", want: []string{"123", "456"}},
		"whitespace":         {input: "a   /b\t", want: []string{"a   ", "b\t"}},
		"two slashes":        {input: "a//b", want: []string{"a", "b"}},
		"two slashes in key": {input: "a//b/c/d", want: []string{"a", "b/c/d"}},
		"illegal chars":      {input: "a/$%^*", want: []string{"a", "$%^*"}},
	}

	for name, tc := range tests {
		name := name
		tc := tc

		t.Run(name, func(t *testing.T) {
			out1, out2, _ := parse(tc.input)

			got := []string{out1, out2}

			diff := cmp.Diff(tc.want, got)
			if diff != "" {
				t.Fatalf(diff)
			}
		})
	}
}

func BenchmarkNameParse(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var (
			res1 string
			res2 string
		)

		bucket, key, _ := parse("test/name")

		res1 = bucket
		res2 = key

		fmt.Println(res1)
		fmt.Println(res2)
	}
}
