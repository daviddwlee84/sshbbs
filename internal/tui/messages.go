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
)

// NavigateMsg is emitted by sub-screens to ask the Root to swap screens.
// Optional fields carry context the destination screen needs.
type NavigateMsg struct {
	To        Screen
	BoardID   int64
	ArticleID int64
	Recipient string // for ScreenWBCompose: prefilled recipient userid
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
