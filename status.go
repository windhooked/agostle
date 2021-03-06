// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

// Needed: /email/convert?splitted=1&errors=1&id=xxx Accept: images/gif
//  /pdf/merge Accept: application/zip

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/kardianos/osext"
)

type statInfo struct {
	mem                *runtime.MemStats
	startedAt, version string
	top                []byte
	last               time.Time
	mtx                sync.Mutex
}

var stats = new(statInfo)
var (
	topOut      = bytes.NewBuffer(make([]byte, 0, 4096))
	topCmd      = []string{"top", "-b", "-n1", "-s", "-S", "-u", ""} // will be different on windows - see main_windows.go
	onceOnStart = new(sync.Once)
)

func onStart() {
	Log := logger.Log
	var err error
	if self, err = osext.Executable(); err != nil {
		Log("msg", "error getting the path for self", "error", err)
	} else {
		var self2 string
		if self2, err = filepath.Abs(self); err != nil {
			Log("msg", "error getting the absolute path", "for", self, "error", err)
		} else {
			self = self2
		}
	}

	var uname string
	if u, e := user.Current(); e != nil {
		Log("msg", "cannot get current user", "error", e)
		uname = os.Getenv("USER")
	} else {
		uname = u.Username
	}
	i := len(topCmd) - 1
	topCmd[i] = topCmd[i] + uname

	stats.startedAt = time.Now().Format(time.RFC3339)

	http.DefaultServeMux.Handle("/", http.HandlerFunc(statusPage))
}

// getTopOut returns the output of the topCmd - shall be protected with a mutex
func getTopOutput() ([]byte, error) {
	topOut.Reset()
	cmd := exec.Command(topCmd[0], topCmd[1:]...)
	cmd.Stdout = topOut
	cmd.Stderr = os.Stderr
	e := cmd.Run()
	if e != nil {
		logger.Log("msg", "error calling", "cmd", topCmd, "error", e)
		fmt.Fprintf(topOut, "\n\nerror calling %s: %s\n", topCmd, e)
	}
	return topOut.Bytes(), e
}

// fill fills the stat iff the current one is stale
func (st *statInfo) fill() {
	st.mtx.Lock()
	defer st.mtx.Unlock()

	now := time.Now()
	if st.mem == nil {
		st.mem = new(runtime.MemStats)
		st.version = runtime.Version()
	} else if now.Sub(st.last) <= 5*time.Second {
		return
	}
	st.last = now
	runtime.ReadMemStats(st.mem)
	var err error
	if st.top, err = getTopOutput(); err != nil {
		logger.Log("msg", "error calling top", "error", err)
	} else {
		st.top = bytes.Replace(st.top, []byte("\n"), []byte("\n    "), -1)
	}
}

func statusPage(w http.ResponseWriter, r *http.Request) {
	if r.RequestURI == "/favicon.ico" {
		http.Error(w, "", 404)
		return
	}
	stats.fill()
	w.Header().Add("Content-Type", "text/html")
	w.WriteHeader(200)
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
  <head><title>Agostle</title></head>
  <body>
    <h1>Agostle</h1>
    <p>%s compiled with Go version %s</p>
    <p>%d started at %s<br/>
    Allocated: %.03fMb (Sys: %.03fMb)</p>

    <p><a href="/_admin/stop">Stop</a> (hopefully supervisor runit will restart).</p>

    <h2>Top</h2>
    <pre>    `,
		self, stats.version,
		os.Getpid(), stats.startedAt,
		float64(stats.mem.Alloc)/1024/1024, float64(stats.mem.Sys)/1024/1024)
	//io.WriteString(w, stats.top)
	_, _ = w.Write(stats.top)
	_, _ = io.WriteString(w, `</pre></body></html>`)
}
