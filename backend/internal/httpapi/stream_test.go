package httpapi_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/analysis"
	"github.com/yefersongallo/bia-energy/backend/internal/app"
	"github.com/yefersongallo/bia-energy/backend/internal/dataset"
	"github.com/yefersongallo/bia-energy/backend/internal/explain"
	"github.com/yefersongallo/bia-energy/backend/internal/httpapi"
	"github.com/yefersongallo/bia-energy/backend/internal/live"
	"github.com/yefersongallo/bia-energy/backend/internal/store/memory"
)

type sse struct{ id, event, data string }

// readEvents reads SSE frames until n events arrive or the deadline passes.
func readEvents(t *testing.T, sc *bufio.Scanner, n int) []sse {
	t.Helper()
	var out []sse
	var cur sse
	for len(out) < n && sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if cur.event != "" {
				out = append(out, cur)
			}
			cur = sse{}
		case strings.HasPrefix(line, "id: "):
			cur.id = line[4:]
		case strings.HasPrefix(line, "event: "):
			cur.event = line[7:]
		case strings.HasPrefix(line, "data: "):
			cur.data = line[6:]
		}
	}
	return out
}

func liveServer(t *testing.T) (*httptest.Server, *live.Runner, string) {
	t.Helper()
	st := memory.New()
	ds := dataset.Generate()
	_ = st.Seed(context.Background(), ds.Meters, ds.Readings, ds.Events)
	eng := analysis.New(analysis.DefaultConfig())
	svc := app.New(st, eng, explain.Template{}, app.Options{})
	runner := live.NewRunner(eng, live.NewHub(0), ds.Meters, ds.Readings, ds.Events, live.Options{})
	auth := httpapi.Auth{Secret: []byte("test"), User: "u", Password: "p", TTL: time.Hour}
	srv := httptest.NewServer(httpapi.New(svc, httpapi.Config{Auth: auth, Live: runner}))
	t.Cleanup(srv.Close)
	tok, _, _ := auth.Login("u", "p")
	return srv, runner, tok
}

func open(t *testing.T, url, lastID string) (*bufio.Scanner, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream = %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<22) // the snapshot is one long data line
	return sc, func() { cancel(); res.Body.Close() }
}

func TestStreamSnapshotTicksAndResume(t *testing.T) {
	srv, runner, tok := liveServer(t)
	res, err := http.Get(srv.URL + "/api/stream")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("stream without token = %d", res.StatusCode)
	}

	sc, closeA := open(t, srv.URL+"/api/stream?token="+tok, "")
	first := readEvents(t, sc, 1)
	if len(first) != 1 || first[0].event != "snapshot" || !strings.Contains(first[0].data, `"meters"`) {
		t.Fatalf("first event = %+v", first)
	}
	runner.Step()
	runner.Step()
	ticks := readEvents(t, sc, 2)
	if len(ticks) != 2 || ticks[0].event != "tick" || !strings.Contains(ticks[0].data, `"readings"`) {
		t.Fatalf("ticks = %+v", ticks)
	}
	closeA()

	// Missed while disconnected: resumes from the last id without a new snapshot.
	runner.Step()
	sc, closeB := open(t, srv.URL+"/api/stream?token="+tok, ticks[0].id)
	defer closeB()
	resumed := readEvents(t, sc, 2)
	if len(resumed) != 2 || resumed[0].id != ticks[1].id || resumed[0].event != "tick" {
		t.Fatalf("resumed = %+v (after %s)", resumed, ticks[0].id)
	}
}

func TestStreamControls(t *testing.T) {
	srv, _, tok := liveServer(t)
	post := func(body string) int {
		req, _ := http.NewRequest("POST", srv.URL+"/api/stream/control", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if c := post(`{"action":"speed","speed":4}`); c != 200 {
		t.Fatalf("speed = %d", c)
	}
	if c := post(`{"action":"warp"}`); c != 400 {
		t.Fatalf("unknown action = %d", c)
	}
	if c := post(`{"action":"reset"}`); c != 200 {
		t.Fatalf("reset = %d", c)
	}
}
