package notify_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/daviddwlee84/sshbbs/internal/notify"
	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

// fakeOnline mocks chat.Broker.IsOnline for the only_when_offline pref.
type fakeOnline struct {
	mu     sync.Mutex
	online map[int64]bool
}

func (f *fakeOnline) IsOnline(uid int64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.online[uid]
}

// recorder records every POST body for assertion.
type recorder struct {
	mu      sync.Mutex
	bodies  [][]byte
	headers []http.Header
	hits    int32
}

func (r *recorder) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		atomic.AddInt32(&r.hits, 1)
		body, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.bodies = append(r.bodies, body)
		r.headers = append(r.headers, req.Header.Clone())
		r.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
}

func (r *recorder) latestBody() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.bodies) == 0 {
		return nil
	}
	return r.bodies[len(r.bodies)-1]
}

// waitFor polls until the condition is true or the timeout elapses. Used
// because Dispatch is async — we block on a webhook arriving rather than
// sleeping a fixed duration.
func waitFor(t *testing.T, want int32, hits *int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(hits) >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d webhook hit(s); got %d", want, atomic.LoadInt32(hits))
}

func TestDispatcher_PostsTitleBodyJSON(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	if _, err := st.Notify().AddTarget(context.Background(), alice.ID, "discord", srv.URL); err != nil {
		t.Fatalf("AddTarget: %v", err)
	}

	mgr := notify.New(st, &fakeOnline{online: map[int64]bool{}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)
	defer mgr.Stop()

	mgr.Dispatch(notify.Event{
		Kind:       notify.KindPush,
		ToUserID:   alice.ID,
		FromUserID: "bob",
		Title:      "alice 推了你的文章 «Hello»",
		Body:       "→ 推 cool post",
	})
	waitFor(t, 1, &rec.hits)

	var got struct{ Title, Body string }
	if err := json.Unmarshal(rec.latestBody(), &got); err != nil {
		t.Fatalf("unmarshal body: %v (raw=%q)", err, rec.latestBody())
	}
	if got.Title != "alice 推了你的文章 «Hello»" || got.Body != "→ 推 cool post" {
		t.Errorf("payload = %+v", got)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.headers[0].Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", rec.headers[0].Get("Content-Type"))
	}
}

func TestDispatcher_RespectsPrefsKind(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	if _, err := st.Notify().AddTarget(context.Background(), alice.ID, "x", srv.URL); err != nil {
		t.Fatal(err)
	}
	// Disable the push pref. A push event should NOT fire; a wb event should.
	if err := st.Notify().SetPrefs(context.Background(), alice.ID,
		store.NotifyPrefs{OnPush: false, OnWB: true, OnMail: true, OnReply: true}); err != nil {
		t.Fatal(err)
	}

	mgr := notify.New(st, &fakeOnline{online: map[int64]bool{}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)
	defer mgr.Stop()

	mgr.Dispatch(notify.Event{Kind: notify.KindPush, ToUserID: alice.ID, Title: "p", Body: "p"})
	mgr.Dispatch(notify.Event{Kind: notify.KindWB, ToUserID: alice.ID, Title: "w", Body: "w"})
	waitFor(t, 1, &rec.hits)
	// Give a small grace window in case OnPush WAS dispatched and we missed it.
	time.Sleep(100 * time.Millisecond)
	if got := atomic.LoadInt32(&rec.hits); got != 1 {
		t.Errorf("hits = %d, want exactly 1 (only WB)", got)
	}
}

func TestDispatcher_SkipsDisabledTargets(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	id, _ := st.Notify().AddTarget(context.Background(), alice.ID, "x", srv.URL)
	_ = st.Notify().SetTargetEnabled(context.Background(), id, alice.ID, false)

	mgr := notify.New(st, &fakeOnline{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)
	defer mgr.Stop()

	mgr.Dispatch(notify.Event{Kind: notify.KindMail, ToUserID: alice.ID, Title: "x", Body: "x"})
	time.Sleep(150 * time.Millisecond)
	if got := atomic.LoadInt32(&rec.hits); got != 0 {
		t.Errorf("disabled target was hit %d times", got)
	}
}

func TestDispatcher_OnlyWhenOfflineSkipsOnlineUser(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	_, _ = st.Notify().AddTarget(context.Background(), alice.ID, "x", srv.URL)
	_ = st.Notify().SetPrefs(context.Background(), alice.ID, store.NotifyPrefs{
		OnPush: true, OnWB: true, OnMail: true, OnReply: true, OnlyWhenOffline: true,
	})

	online := &fakeOnline{online: map[int64]bool{alice.ID: true}}
	mgr := notify.New(st, online)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)
	defer mgr.Stop()

	mgr.Dispatch(notify.Event{Kind: notify.KindWB, ToUserID: alice.ID, Title: "x", Body: "x"})
	time.Sleep(150 * time.Millisecond)
	if got := atomic.LoadInt32(&rec.hits); got != 0 {
		t.Errorf("dispatch fired while user was online: hits=%d", got)
	}

	// Take alice offline; next dispatch should fire.
	online.mu.Lock()
	online.online[alice.ID] = false
	online.mu.Unlock()
	mgr.Dispatch(notify.Event{Kind: notify.KindWB, ToUserID: alice.ID, Title: "x", Body: "x"})
	waitFor(t, 1, &rec.hits)
}

// Stresses the queue under concurrent producers — race-detector bait.
// Mirrors the spirit of TestPushes_ConcurrentScoreAtomicity.
func TestDispatcher_ConcurrentDispatchNoLeak(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	_, _ = st.Notify().AddTarget(context.Background(), alice.ID, "x", srv.URL)

	mgr := notify.New(st, &fakeOnline{})
	ctx, cancel := context.WithCancel(context.Background())
	mgr.Start(ctx)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.Dispatch(notify.Event{Kind: notify.KindPush, ToUserID: alice.ID, Title: "x", Body: "x"})
		}()
	}
	wg.Wait()
	cancel()
	mgr.Stop()
}

func TestDispatcher_SendTest_Success(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	st := storetest.New(t)
	mgr := notify.New(st, &fakeOnline{})
	target := &store.NotifyTarget{ID: 1, URL: srv.URL}

	err := mgr.SendTest(context.Background(), target, notify.Event{
		Title: "test", Body: "hello",
	})
	if err != nil {
		t.Fatalf("SendTest: %v", err)
	}
	if got := atomic.LoadInt32(&rec.hits); got != 1 {
		t.Errorf("hits = %d, want 1", got)
	}
}

func TestDispatcher_SendTest_4xxReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"message":"bad payload"}`))
	}))
	defer srv.Close()

	st := storetest.New(t)
	mgr := notify.New(st, &fakeOnline{})
	target := &store.NotifyTarget{ID: 1, URL: srv.URL}

	err := mgr.SendTest(context.Background(), target, notify.Event{Title: "t", Body: "b"})
	if err == nil {
		t.Fatal("SendTest with 400 returned nil error, want error")
	}
	// Body excerpt must be in the error so the user sees the receiver's
	// complaint, not just a bare status code.
	if !contains(err.Error(), "400") || !contains(err.Error(), "bad payload") {
		t.Errorf("err = %q, want HTTP 400 + body excerpt", err)
	}
}

func TestDispatcher_SendTest_BypassesPrefsAndQueue(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	// Disable every kind so a real Dispatch would no-op. SendTest must
	// fire anyway because users testing a webhook URL haven't necessarily
	// configured prefs yet.
	_ = st.Notify().SetPrefs(context.Background(), alice.ID, store.NotifyPrefs{})

	mgr := notify.New(st, &fakeOnline{})
	target := &store.NotifyTarget{ID: 1, UserID: alice.ID, URL: srv.URL}
	if err := mgr.SendTest(context.Background(), target, notify.Event{Title: "t", Body: "b"}); err != nil {
		t.Fatalf("SendTest: %v", err)
	}
	if got := atomic.LoadInt32(&rec.hits); got != 1 {
		t.Errorf("SendTest didn't reach receiver despite prefs all-off (hits=%d)", got)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Dispatch after Stop must not panic — guards the closed-channel send race.
func TestDispatcher_DispatchAfterStop(t *testing.T) {
	st := storetest.New(t)
	mgr := notify.New(st, &fakeOnline{})
	ctx, cancel := context.WithCancel(context.Background())
	mgr.Start(ctx)
	cancel()
	mgr.Stop()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Dispatch after Stop panicked: %v", r)
		}
	}()
	mgr.Dispatch(notify.Event{Kind: notify.KindPush, ToUserID: 1, Title: "x", Body: "x"})
}
