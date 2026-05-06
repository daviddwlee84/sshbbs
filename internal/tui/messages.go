package tui

// Screen identifies which sub-model the Root should be showing.
type Screen int

const (
	ScreenMainMenu Screen = iota
	ScreenBoardList
	ScreenBoardView
	ScreenArticleView
	ScreenPostCompose
	ScreenWBInbox
	ScreenWBCompose
	ScreenOnline
	ScreenMailInbox
	ScreenMailThread
	ScreenMailCompose
	ScreenPasswordChange
	ScreenAdminUsers
	ScreenArticleEdit
	ScreenArticleExport
	ScreenBoardSplash
	ScreenBoardBannerEdit
)

// NavigateMsg is emitted by sub-screens to ask the Root to swap screens.
// Optional fields carry context the destination screen needs.
type NavigateMsg struct {
	To           Screen
	BoardID      int64
	ArticleID    int64
	Recipient    string // for ScreenWBCompose / ScreenMailCompose: prefilled recipient userid
	MailID       int64  // for ScreenMailCompose: parent mail id when replying
	MailThreadID int64  // for ScreenMailThread: which thread to open
}

// ErrorMsg surfaces an error as a transient toast.
type ErrorMsg struct{ Err error }

// WBIncomingMsg is delivered by the chat broker (or replayed from DB on Init)
// to notify the recipient that a new water balloon arrived.
type WBIncomingMsg struct {
	ID         int64
	FromUserID string
	Body       string
}

// PushAddedMsg is broadcast by the chat broker when a push is added to an
// article. The article-view model checks ArticleID to decide whether to
// re-render.
type PushAddedMsg struct {
	ArticleID  int64
	UserUserID string
	Kind       string
	Body       string
}

// MailIncomingMsg notifies the recipient that a new mail has arrived.
// Unlike WBIncomingMsg it does NOT auto-toast — mail is asynchronous; the
// open mail inbox can choose to refresh, and the main menu reads the unread
// counter from DB.
type MailIncomingMsg struct {
	ID         int64
	ThreadID   int64
	FromUserID string
	Subject    string
}

// ArticleAddedMsg is broadcast by the chat broker when a new article is
// posted. Board-view models filter by BoardID and re-fetch the article
// list from DB so timestamps and ordering reflect canonical state.
type ArticleAddedMsg struct {
	BoardID      int64
	ArticleID    int64
	AuthorUserID string
	Title        string
}

// ArticleUpdatedMsg is broadcast by the chat broker when an article's
// title or body is edited. Article-view models filter by ArticleID and
// re-fetch from DB so they don't render stale text.
type ArticleUpdatedMsg struct {
	ArticleID int64
}

// ArticlePinChangedMsg is broadcast when a mod pins or unpins an article.
// Board-view models filter by BoardID and re-fetch the article list so the
// pinned-first ordering and the [M] marker stay current across all live
// sessions viewing the same board. ArticleID + Pinned are advisory; the
// re-fetch is the source of truth.
type ArticlePinChangedMsg struct {
	BoardID   int64
	ArticleID int64
	Pinned    bool
}
