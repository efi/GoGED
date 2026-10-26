package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/server"
	"github.com/efi/goged/tui"
)

// runServe serves the browser to web browsers at addr until the program
// is stopped. An address without a colon is a port on this computer.
func runServe(doc *gedcom.Document, opts tui.Options, addr string, stdout, stderr io.Writer) int {
	if !strings.Contains(addr, ":") {
		addr = "localhost:" + addr
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(stderr, "goged: %v\n", err)
		return 1
	}
	srv := server.New(server.Config{
		Shared:  tui.NewShared(doc),
		Options: opts,
		Logf:    func(format string, args ...any) { fmt.Fprintf(stderr, format+"\n", args...) },
	})
	host, _, _ := net.SplitHostPort(addr)
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	if host == "" || host == "0.0.0.0" || host == "::" {
		fmt.Fprintln(stdout, "goged: on all networks; whoever has the address below, with its token, can browse the file")
		host = "localhost"
	}
	fmt.Fprintf(stdout, "goged: serving %s at http://%s/#%s\n", opts.Title, net.JoinHostPort(host, port), srv.Token())
	hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	if err := hs.Serve(ln); err != nil {
		fmt.Fprintf(stderr, "goged: %v\n", err)
		return 1
	}
	return 0
}
