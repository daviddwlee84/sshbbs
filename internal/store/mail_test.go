package store_test

import (
	"context"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func TestMail_InsertRoot_ThreadIDIsSelf(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")

	m, err := st.Mail().Insert(ctx, alice.ID, alice.UserID, bob.ID, "hi", "body", nil)
	if err != nil {
		t.Fatalf("Insert root: %v", err)
	}
	if m.ThreadID != m.ID {
		t.Errorf("root ThreadID = %d, want self %d", m.ThreadID, m.ID)
	}
	if m.ParentID.Valid {
		t.Errorf("root ParentID should be NULL, got %d", m.ParentID.Int64)
	}
	if m.ReadAt.Valid {
		t.Errorf("new mail should be unread, got read_at=%v", m.ReadAt.Time)
	}
}

func TestMail_InsertReply_InheritsThreadID(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")

	root, err := st.Mail().Insert(ctx, alice.ID, alice.UserID, bob.ID, "subj", "body", nil)
	if err != nil {
		t.Fatalf("Insert root: %v", err)
	}
	parentID := root.ID
	reply, err := st.Mail().Insert(ctx, bob.ID, bob.UserID, alice.ID, "Re: subj", "thanks", &parentID)
	if err != nil {
		t.Fatalf("Insert reply: %v", err)
	}
	if reply.ThreadID != root.ID {
		t.Errorf("reply ThreadID = %d, want %d", reply.ThreadID, root.ID)
	}
	if !reply.ParentID.Valid || reply.ParentID.Int64 != root.ID {
		t.Errorf("reply ParentID = %v, want valid=%d", reply.ParentID, root.ID)
	}
}

func TestMail_InsertReply_NonexistentParent(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	missing := int64(9999)
	_, err := st.Mail().Insert(ctx, alice.ID, alice.UserID, bob.ID, "subj", "body", &missing)
	if err == nil {
		t.Errorf("Insert with missing parent: want error, got nil")
	}
}

func TestMail_ListInboxFor_UnreadFirstNewestFirst(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")

	m1, _ := st.Mail().Insert(ctx, alice.ID, alice.UserID, bob.ID, "first", "", nil)
	m2, _ := st.Mail().Insert(ctx, alice.ID, alice.UserID, bob.ID, "second", "", nil)
	m3, _ := st.Mail().Insert(ctx, alice.ID, alice.UserID, bob.ID, "third", "", nil)
	m4, _ := st.Mail().Insert(ctx, alice.ID, alice.UserID, bob.ID, "fourth", "", nil)
	_ = st.Mail().MarkRead(ctx, m2.ID)
	_ = m1
	_ = m3
	_ = m4

	got, err := st.Mail().ListInboxFor(ctx, bob.ID, 0)
	if err != nil {
		t.Fatalf("ListInboxFor: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d, want 4", len(got))
	}
	wantSubjects := []string{"fourth", "third", "first", "second"}
	for i, w := range wantSubjects {
		if got[i].Subject != w {
			t.Errorf("[%d].Subject = %q, want %q", i, got[i].Subject, w)
		}
	}
}

func TestMail_ListThread_Chronological(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")

	root, _ := st.Mail().Insert(ctx, alice.ID, alice.UserID, bob.ID, "topic", "1", nil)
	parentID := root.ID
	r1, _ := st.Mail().Insert(ctx, bob.ID, bob.UserID, alice.ID, "Re: topic", "2", &parentID)
	r1Parent := r1.ID
	r2, _ := st.Mail().Insert(ctx, alice.ID, alice.UserID, bob.ID, "Re: topic", "3", &r1Parent)

	got, err := st.Mail().ListThread(ctx, root.ID)
	if err != nil {
		t.Fatalf("ListThread: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d, want 3", len(got))
	}
	wantBodies := []string{"1", "2", "3"}
	for i, w := range wantBodies {
		if got[i].Body != w {
			t.Errorf("[%d].Body = %q, want %q", i, got[i].Body, w)
		}
	}
	_ = r2
}

func TestMail_MarkAllReadFor(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	for i := 0; i < 4; i++ {
		_, _ = st.Mail().Insert(ctx, alice.ID, alice.UserID, bob.ID, "s", "b", nil)
	}
	if err := st.Mail().MarkAllReadFor(ctx, bob.ID); err != nil {
		t.Fatalf("MarkAllReadFor: %v", err)
	}
	n, err := st.Mail().CountUnreadFor(ctx, bob.ID)
	if err != nil {
		t.Fatalf("CountUnreadFor: %v", err)
	}
	if n != 0 {
		t.Errorf("after MarkAllReadFor: unread=%d, want 0", n)
	}
}

func TestMail_CountUnreadFor(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	for i := 0; i < 3; i++ {
		_, _ = st.Mail().Insert(ctx, alice.ID, alice.UserID, bob.ID, "s", "b", nil)
	}
	one, _ := st.Mail().Insert(ctx, alice.ID, alice.UserID, bob.ID, "s", "b", nil)
	_ = st.Mail().MarkRead(ctx, one.ID)
	n, _ := st.Mail().CountUnreadFor(ctx, bob.ID)
	if n != 3 {
		t.Errorf("CountUnreadFor: %d, want 3", n)
	}
}

// Concurrent inserts must produce distinct IDs and distinct thread IDs for
// roots — guards against the writeMu discipline regressing.
func TestMail_ConcurrentInserts(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")

	const n = 50
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, err := st.Mail().Insert(ctx, alice.ID, alice.UserID, bob.ID, "s", "b", nil)
			errs <- err
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent insert %d: %v", i, err)
		}
	}
	count, err := st.Mail().CountUnreadFor(ctx, bob.ID)
	if err != nil {
		t.Fatalf("CountUnreadFor: %v", err)
	}
	if count != n {
		t.Errorf("after %d concurrent inserts: unread=%d, want %d", n, count, n)
	}
}

func TestMail_GetByID_NotFound(t *testing.T) {
	st := storetest.New(t)
	_, err := st.Mail().GetByID(context.Background(), 9999)
	if err != store.ErrMailNotFound {
		t.Errorf("got %v, want ErrMailNotFound", err)
	}
}
