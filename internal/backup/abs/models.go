// Package abs handles Audiobookshelf backup import.
//
// Audiobookshelf is a self-hosted audiobook server. Users migrating to ListenUp
// need their listening history preserved—every completed book, every bookmark,
// every position. This package makes that migration seamless.
package abs

import "time"

// Backup represents a parsed Audiobookshelf backup.
// ABS creates backups as .audiobookshelf files (actually tar.gz archives).
type Backup struct {
	Path      string
	Users     []User
	Libraries []Library
	Items     []LibraryItem
	Authors   []Author
	Series    []Series
	Sessions  []Session
}

// User represents an ABS user account.
// ABS embeds listening progress directly in user records.
type User struct {
	ID       string          `json:"id"`
	Username string          `json:"username"`
	Email    string          `json:"email"`
	Type     string          `json:"type"` // "root", "admin", "user", "guest"
	Progress []MediaProgress `json:"mediaProgress"`
}

// IsImportable returns true if this user type should be imported.
// Guest users are typically not imported as they have no persistent identity.
func (u *User) IsImportable() bool {
	return u.Type != "guest"
}

// MediaProgress represents embedded listening progress in ABS user records.
// This is the "current state" — where the user left off.
type MediaProgress struct {
	ID               string  `json:"id"`
	LibraryItemID    string  `json:"libraryItemId"`
	EpisodeID        string  `json:"episodeId,omitempty"` // For podcasts (we skip these)
	MediaItemID      string  `json:"mediaItemId"`
	MediaItemType    string  `json:"mediaItemType"` // "book" or "podcast"
	Duration         float64 `json:"duration"`      // Total book duration in seconds
	CurrentTime      float64 `json:"currentTime"`   // Current position in seconds
	Progress         float64 `json:"progress"`      // 0.0 - 1.0
	IsFinished       bool    `json:"isFinished"`
	HideFromContinue bool    `json:"hideFromContinueListening"`
	LastUpdate       int64   `json:"lastUpdate"` // Unix milliseconds
	StartedAt        int64   `json:"startedAt"`  // Unix milliseconds
	FinishedAt       int64   `json:"finishedAt"` // Unix milliseconds (0 if not finished)
}

// IsBook returns true if this progress is for an audiobook (not podcast).
func (p *MediaProgress) IsBook() bool {
	return p.MediaItemType == "book" || p.MediaItemType == ""
}

// LastUpdateTime returns LastUpdate as a time.Time.
func (p *MediaProgress) LastUpdateTime() time.Time {
	return time.UnixMilli(p.LastUpdate)
}

// Library represents an ABS library configuration.
type Library struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Folders    []Folder `json:"folders"`
	MediaType  string   `json:"mediaType"` // "book" or "podcast"
	Provider   string   `json:"provider"`
	Settings   any      `json:"settings"`
	CreatedAt  int64    `json:"createdAt"`
	LastUpdate int64    `json:"lastUpdate"`
}

// Folder is a library folder path.
type Folder struct {
	ID       string `json:"id"`
	FullPath string `json:"fullPath"`
}

// IsBookLibrary returns true if this is an audiobook library.
func (l *Library) IsBookLibrary() bool {
	return l.MediaType == "book"
}

// LibraryItem represents a book (or podcast) in ABS.
// ABS calls these "library items" — we only care about books.
type LibraryItem struct {
	ID           string    `json:"id"`
	MediaID      string    `json:"mediaId"` // References books.id - used for session matching
	INO          string    `json:"ino"`     // Inode of root folder
	LibraryID    string    `json:"libraryId"`
	FolderID     string    `json:"folderId"`
	Path         string    `json:"path"`
	RelPath      string    `json:"relPath"`
	MediaType    string    `json:"mediaType"` // "book" or "podcast"
	IsFile       bool      `json:"isFile"`
	IsMissing    bool      `json:"isMissing"`
	IsInvalid    bool      `json:"isInvalid"`
	Media        BookMedia `json:"media"`
	LibraryFiles []any     `json:"libraryFiles,omitempty"` // Raw file info
	AddedAt      int64     `json:"addedAt"`
	UpdatedAt    int64     `json:"updatedAt"`
}

// IsBook returns true if this item is an audiobook.
func (i *LibraryItem) IsBook() bool {
	return i.MediaType == "book"
}

// IsValid returns true if this item should be considered for import.
func (i *LibraryItem) IsValid() bool {
	return i.IsBook() && !i.IsMissing && !i.IsInvalid
}

// BookMedia contains book-specific metadata.
type BookMedia struct {
	Metadata   BookMetadata `json:"metadata"`
	CoverPath  string       `json:"coverPath,omitempty"`
	Tags       []string     `json:"tags,omitempty"`
	AudioFiles []AudioFile  `json:"audioFiles"`
	Chapters   []Chapter    `json:"chapters,omitempty"`
	Duration   float64      `json:"duration"`  // Total duration in seconds
	Size       int64        `json:"size"`      // Total size in bytes
	EbookFile  any          `json:"ebookFile"` // We ignore ebooks
}

// DurationMs returns duration in milliseconds (ListenUp's format).
func (m *BookMedia) DurationMs() int64 {
	return int64(m.Duration * 1000)
}

// BookMetadata is the core book information.
type BookMetadata struct {
	Title         string      `json:"title"`
	Subtitle      string      `json:"subtitle,omitempty"`
	Authors       []PersonRef `json:"authors"`
	Narrators     []PersonRef `json:"narrators"`
	Series        []SeriesRef `json:"series"`
	Genres        []string    `json:"genres,omitempty"`
	PublishedYear string      `json:"publishedYear,omitempty"`
	Publisher     string      `json:"publisher,omitempty"`
	Description   string      `json:"description,omitempty"`
	ISBN          string      `json:"isbn,omitempty"`
	ASIN          string      `json:"asin,omitempty"`
	Language      string      `json:"language,omitempty"`
	Explicit      bool        `json:"explicit"`
	Abridged      bool        `json:"abridged"`
}

// HasASIN returns true if the book has an Amazon ASIN.
func (m *BookMetadata) HasASIN() bool {
	return m.ASIN != ""
}

// HasISBN returns true if the book has an ISBN.
func (m *BookMetadata) HasISBN() bool {
	return m.ISBN != ""
}

// PrimaryAuthor returns the first author's name, or empty string.
func (m *BookMetadata) PrimaryAuthor() string {
	if len(m.Authors) > 0 {
		return m.Authors[0].Name
	}
	return ""
}

// PersonRef is a reference to an author, narrator, or other person.
type PersonRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SeriesRef is a reference to a series with position.
type SeriesRef struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Sequence string `json:"sequence,omitempty"` // "1", "1.5", "Book Zero", etc.
}

// AudioFile represents an audio file within a book.
type AudioFile struct {
	Index                int     `json:"index"`
	INO                  string  `json:"ino"`      // Inode
	Metadata             any     `json:"metadata"` // Raw ffprobe data
	AddedAt              int64   `json:"addedAt"`
	UpdatedAt            int64   `json:"updatedAt"`
	TrackNumFromMeta     int     `json:"trackNumFromMeta"`
	DiscNumFromMeta      int     `json:"discNumFromMeta"`
	TrackNumFromFilename int     `json:"trackNumFromFilename"`
	DiscNumFromFilename  int     `json:"discNumFromFilename"`
	Duration             float64 `json:"duration"` // Seconds
	BitRate              int     `json:"bitRate"`
	Language             string  `json:"language,omitempty"`
	Codec                string  `json:"codec"`
	TimeBase             string  `json:"timeBase"`
	Channels             int     `json:"channels"`
	ChannelLayout        string  `json:"channelLayout"`
	MimeType             string  `json:"mimeType"`
}

// DurationMs returns duration in milliseconds.
func (f *AudioFile) DurationMs() int64 {
	return int64(f.Duration * 1000)
}

// Chapter represents a chapter marker.
type Chapter struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"` // Seconds
	End   float64 `json:"end"`   // Seconds
	Title string  `json:"title"`
}

// Author represents an ABS author entity.
type Author struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ASIN        string `json:"asin,omitempty"`
	Description string `json:"description,omitempty"`
	ImagePath   string `json:"imagePath,omitempty"`
	AddedAt     int64  `json:"addedAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

// Series represents an ABS series entity.
type Series struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	AddedAt     int64  `json:"addedAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

// Session represents a listening session.
// This is the historical record of a listening span — when the user listened,
// for how long, and what positions they moved between.
type Session struct {
	ID            string  `json:"id"`
	UserID        string  `json:"userId"`
	LibraryID     string  `json:"libraryId"`
	LibraryItemID string  `json:"libraryItemId"`
	EpisodeID     string  `json:"episodeId,omitempty"` // For podcasts
	MediaType     string  `json:"mediaType"`           // "book" or "podcast"
	MediaMetadata any     `json:"mediaMetadata"`       // Book info at time of session
	Chapters      []any   `json:"chapters,omitempty"`
	DisplayTitle  string  `json:"displayTitle"`
	DisplayAuthor string  `json:"displayAuthor"`
	CoverPath     string  `json:"coverPath,omitempty"`
	Duration      float64 `json:"duration"`   // Duration of listening in seconds
	PlayMethod    int     `json:"playMethod"` // 0=direct, 1=transcode, etc.
	MediaPlayer   string  `json:"mediaPlayer"`
	DeviceInfo    any     `json:"deviceInfo,omitempty"`
	ServerVersion string  `json:"serverVersion,omitempty"`
	Date          string  `json:"date,omitempty"`      // "YYYY-MM-DD"
	DayOfWeek     string  `json:"dayOfWeek,omitempty"` // "Monday", etc.
	TimeListening float64 `json:"timeListening"`       // Same as duration?
	StartTime     float64 `json:"startTime"`           // Position at session start (seconds)
	CurrentTime   float64 `json:"currentTime"`         // Position at session end (seconds)
	StartedAt     int64   `json:"startedAt"`           // Unix milliseconds
	UpdatedAt     int64   `json:"updatedAt"`           // Unix milliseconds
}

// IsBook returns true if this session is for an audiobook.
func (s *Session) IsBook() bool {
	return s.MediaType == "book"
}

// StartedAtTime returns StartedAt as time.Time.
func (s *Session) StartedAtTime() time.Time {
	return time.UnixMilli(s.StartedAt)
}

// EndedAtTime returns UpdatedAt as time.Time (session end).
func (s *Session) EndedAtTime() time.Time {
	return time.UnixMilli(s.UpdatedAt)
}

// StartPositionMs returns start position in milliseconds.
func (s *Session) StartPositionMs() int64 {
	return int64(s.StartTime * 1000)
}

// EndPositionMs returns end position in milliseconds.
func (s *Session) EndPositionMs() int64 {
	return int64(s.CurrentTime * 1000)
}

// DurationMs returns listening duration in milliseconds.
func (s *Session) DurationMs() int64 {
	return int64(s.Duration * 1000)
}
