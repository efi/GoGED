package main

import (
	"bufio"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

func TestServeFlag(t *testing.T) {
	if r := runCLI(t, "", "-serve", "localhost:http-no", sample); r.code != 1 || !strings.Contains(r.stderr, "goged:") {
		t.Errorf("bad address: exit %d, %q", r.code, r.stderr)
	}

	// A server on a free port says where it is, with the token.
	out, w := io.Pipe()
	go func() { run([]string{"-serve", "localhost:0", sample}, strings.NewReader(""), w, io.Discard, nil) }()
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`^goged: serving family\.ged at (http://localhost:\d+/)#([0-9a-f]{32})\n$`).FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("first line %q", line)
	}
	resp, err := http.Get(m[1])
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("the page: %s", resp.Status)
	}
}
