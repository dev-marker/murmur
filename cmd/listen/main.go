// Command listen is a client for GET /live, the way a browser's EventSource is one:
// it opens a stream, prints what arrives, and when the stream ends it comes back
// and says which event it saw last.
//
//	listen 16726               one stream for user 16726, every event on a line
//	listen -count 1            a counter once a second, not the lines
//	listen -count $(seq 2000)  two thousand streams, one for each of the users 1 to 2000
//	listen -count -nap 60s 1   a client that connects, and reads nothing for a minute
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"time"
)

var (
	base  = flag.String("url", "http://localhost:8080", "where the API is")
	count = flag.Bool("count", false, "print a counter once a second, not every event")
	nap   = flag.Duration("nap", 0, "open the stream, and read nothing for this long")

	open, events atomic.Int64
	first, last  atomic.Int64 // when this second's first and last event arrived, in nanoseconds
)

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: listen [-count] [-nap 60s] [-url base] <user id>...")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	for _, user := range flag.Args() {
		go stream(user)
		time.Sleep(time.Millisecond) // a crowd that arrives over a second or two, not in one instant
	}
	if !*count {
		<-ctx.Done()
		return
	}
	var streams, before int64 = -1, 0
	tick := time.NewTicker(time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		now := events.Load()
		if open.Load() == streams && now == before {
			continue // nothing new to say
		}
		streams = open.Load()
		line := fmt.Sprintf("streams %-5d events %-6d", streams, now)
		if now > before {
			took := time.Duration(last.Load() - first.Swap(0)).Round(time.Millisecond)
			line += fmt.Sprintf(" +%d in %v", now-before, took)
		}
		fmt.Println(line)
		before = now
	}
}

// stream keeps one user's stream open: when it ends, it waits a second and connects again.
func stream(user string) {
	lastID := ""
	for {
		req, _ := http.NewRequest("GET", *base+"/live", nil)
		req.Header.Set("Authorization", "Bearer seed-token-"+user)
		if lastID != "" {
			req.Header.Set("Last-Event-ID", lastID)
		}
		res, err := http.DefaultClient.Do(req)
		switch {
		case err != nil:
			say(user, "no answer: %v", err)
		case res.StatusCode != http.StatusOK:
			say(user, "the server said %s", res.Status)
			res.Body.Close()
		default:
			open.Add(1)
			lastID = read(user, res, lastID)
			open.Add(-1)
			res.Body.Close()
			say(user, "the stream ended. Back in a second, with Last-Event-ID %q", lastID)
		}
		time.Sleep(time.Second)
	}
}

// read prints (or counts) the events of one response, until it ends. It returns the id
// of the last event that had one.
func read(user string, res *http.Response, lastID string) string {
	if *nap > 0 {
		fmt.Printf("%-6s connected, and reading nothing for %v\n", user, *nap)
		time.Sleep(*nap)
		defer func() {
			fmt.Printf("%-6s read %d events, and then the stream ended\n", user, events.Load())
			os.Exit(0)
		}()
	}
	var id, kind, data string
	lines := bufio.NewScanner(res.Body)
	for lines.Scan() {
		field, value, _ := strings.Cut(lines.Text(), ": ")
		switch field {
		case "id":
			id = value
		case "event":
			kind = value
		case "data":
			data = value
		case "": // a blank line ends an event; a line that starts with a colon is a comment
			if kind == "" {
				if !*count && strings.HasPrefix(lines.Text(), ":") {
					say(user, "%s", strings.TrimPrefix(lines.Text(), ": "))
				}
				continue
			}
			if id != "" {
				lastID = id
			}
			now := time.Now().UnixNano()
			first.CompareAndSwap(0, now)
			last.Store(now)
			events.Add(1)
			say(user, "%-5s %-6s %s", kind, id, data)
			id, kind, data = "", "", ""
		}
	}
	if err := lines.Err(); err != nil {
		say(user, "the stream broke: %v", err)
	}
	return lastID
}

// say prints one line for one stream, unless we are only counting.
func say(user, format string, args ...any) {
	if !*count {
		fmt.Printf("%-6s %s\n", user, fmt.Sprintf(format, args...))
	}
}
