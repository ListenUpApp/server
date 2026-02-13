package abs

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode"

	"github.com/listenupapp/listenup-server/internal/store"
)

// BookCache holds pre-loaded book data for fast matching.
type BookCache struct {
	byASIN  map[string]*CachedBook
	byISBN  map[string]*CachedBook
	byPath  map[string]*CachedBook
	byTitle map[string][]*CachedBook // normalized title -> books (may have duplicates)
	all     []*CachedBook
}

// CachedBook is a simplified book for matching.
type CachedBook struct {
	ID            string
	Title         string
	NormTitle     string
	ASIN          string
	ISBN          string
	Path          string
	TotalDuration int64
	Authors       []string // author names
}

// Matcher finds correspondences between ABS and ListenUp entities.
type Matcher struct {
	store     *store.Store
	logger    *slog.Logger
	opts      AnalysisOptions
	bookCache *BookCache
}

// NewMatcher creates a matcher with the given options.
func NewMatcher(s *store.Store, logger *slog.Logger, opts AnalysisOptions) *Matcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Matcher{
		store:  s,
		logger: logger,
		opts:   opts,
	}
}

// MatchUser attempts to find a ListenUp user for an ABS user.
func (m *Matcher) MatchUser(ctx context.Context, absUser *User) *UserMatch {
	match := &UserMatch{
		ABSUser:    absUser,
		Confidence: MatchNone,
	}

	m.logger.Info("matching user",
		"abs_user_id", absUser.ID,
		"abs_username", absUser.Username,
		"abs_email", absUser.Email,
		"match_by_email", m.opts.MatchByEmail,
	)

	// 1. Check admin-specified mapping (definitive)
	if listenUpID, ok := m.opts.UserMappings[absUser.ID]; ok {
		match.ListenUpID = listenUpID
		match.Confidence = MatchDefinitive
		match.MatchReason = "admin-configured mapping"
		return match
	}

	// 2. Try email match (definitive - emails are unique)
	if m.opts.MatchByEmail && absUser.Email != "" {
		user, err := m.store.GetUserByEmail(ctx, absUser.Email)
		m.logger.Info("email lookup result",
			"abs_email", absUser.Email,
			"found", user != nil,
			"error", err,
		)
		if err == nil && user != nil {
			match.ListenUpID = user.ID
			match.Confidence = MatchDefinitive
			match.MatchReason = "email match: " + absUser.Email
			return match
		}
	}

	// 3. Try username/display name match (strong if very similar)
	if absUser.Username != "" {
		if nameMatch := m.matchUserByName(ctx, absUser); nameMatch != nil {
			return nameMatch
		}
	}

	// 4. No match found - try to find suggestions for admin
	match.Suggestions = m.suggestUsers(ctx, absUser)

	return match
}

// matchUserByName attempts to match by username/display name.
// Returns a strong match if there's exactly one user with a very similar name.
func (m *Matcher) matchUserByName(ctx context.Context, absUser *User) *UserMatch {
	absUsername := normalizeString(absUser.Username)

	var bestMatch *UserMatch
	var matchCount int

	// Iterate over all users
	for user, err := range m.store.Users.List(ctx) {
		if err != nil {
			m.logger.Warn("failed to iterate users for name matching", "error", err)
			break
		}

		// Check display name similarity
		displayNameSim := stringSimilarity(normalizeString(user.DisplayName), absUsername)

		// Check first+last name similarity (if we have the full name)
		var fullNameSim float64
		if user.FirstName != "" || user.LastName != "" {
			fullName := strings.TrimSpace(user.FirstName + " " + user.LastName)
			fullNameSim = stringSimilarity(normalizeString(fullName), absUsername)
		}

		// Use the best similarity
		bestSim := displayNameSim
		matchType := "display name"
		if fullNameSim > bestSim {
			bestSim = fullNameSim
			matchType = "full name"
		}

		// Strong match requires >= 0.9 similarity
		if bestSim >= 0.9 {
			matchCount++
			if bestMatch == nil || bestSim > stringSimilarity(normalizeString(bestMatch.MatchReason), absUsername) {
				bestMatch = &UserMatch{
					ABSUser:     absUser,
					ListenUpID:  user.ID,
					Confidence:  MatchStrong,
					MatchReason: fmt.Sprintf("%s match: %s (%.0f%%)", matchType, user.DisplayName, bestSim*100),
				}
			}
		}
	}

	// Only return if there's exactly one strong match (avoid ambiguity)
	if matchCount == 1 && bestMatch != nil {
		m.logger.Info("name match found",
			"abs_username", absUser.Username,
			"listenup_id", bestMatch.ListenUpID,
			"reason", bestMatch.MatchReason,
		)
		return bestMatch
	}

	if matchCount > 1 {
		m.logger.Info("multiple name matches found, skipping auto-match",
			"abs_username", absUser.Username,
			"match_count", matchCount,
		)
	}

	return nil
}

// suggestUsers finds potential ListenUp users that might match an ABS user.
func (m *Matcher) suggestUsers(ctx context.Context, absUser *User) []UserSuggestion {
	var suggestions []UserSuggestion

	absUsername := normalizeString(absUser.Username)

	// Iterate over all users using the iterator pattern
	for user, err := range m.store.Users.List(ctx) {
		if err != nil {
			m.logger.Warn("failed to iterate users for suggestions", "error", err)
			break
		}

		// Skip if email matches - should have been caught above
		if user.Email != "" && strings.EqualFold(user.Email, absUser.Email) {
			continue
		}

		// Score based on display name / username similarity
		score := stringSimilarity(normalizeString(user.DisplayName), absUsername)

		if score >= 0.7 {
			suggestions = append(suggestions, UserSuggestion{
				UserID:      user.ID,
				Email:       user.Email,
				DisplayName: user.DisplayName,
				Score:       score,
				Reason:      "similar username/display name",
			})
		}
	}

	return suggestions
}

// PreloadBooks loads all ListenUp books into memory for fast matching.
func (m *Matcher) PreloadBooks(ctx context.Context) {
	m.bookCache = &BookCache{
		byASIN:  make(map[string]*CachedBook),
		byISBN:  make(map[string]*CachedBook),
		byPath:  make(map[string]*CachedBook),
		byTitle: make(map[string][]*CachedBook),
	}

	// Load all books
	books, err := m.store.ListAllBooks(ctx)
	if err != nil {
		m.logger.Warn("failed to load books", "error", err)
		return
	}

	for _, book := range books {

		cached := &CachedBook{
			ID:            book.ID,
			Title:         book.Title,
			NormTitle:     normalizeTitle(book.Title),
			ASIN:          book.ASIN,
			ISBN:          book.ISBN,
			Path:          book.Path,
			TotalDuration: book.TotalDuration,
		}

		// Load author names
		for _, bc := range book.Contributors {
			if contributor, err := m.store.GetContributor(ctx, bc.ContributorID); err == nil {
				cached.Authors = append(cached.Authors, contributor.Name)
			}
		}

		m.bookCache.all = append(m.bookCache.all, cached)

		// Index by ASIN
		if cached.ASIN != "" {
			m.bookCache.byASIN[cached.ASIN] = cached
		}

		// Index by ISBN
		if cached.ISBN != "" {
			m.bookCache.byISBN[cached.ISBN] = cached
		}

		// Index by path
		if cached.Path != "" {
			m.bookCache.byPath[cached.Path] = cached
		}

		// Index by normalized title (allows multiple books with same title)
		m.bookCache.byTitle[cached.NormTitle] = append(m.bookCache.byTitle[cached.NormTitle], cached)
	}

	m.logger.Info("book cache populated", "total", len(m.bookCache.all))
}

// MatchBookFast matches using the pre-loaded cache (no DB calls).
func (m *Matcher) MatchBookFast(absItem *LibraryItem) *BookMatch {
	match := &BookMatch{
		ABSItem:    absItem,
		Confidence: MatchNone,
	}

	meta := absItem.Media.Metadata

	// 1. Check admin-specified mapping (definitive)
	if listenUpID, ok := m.opts.BookMappings[absItem.ID]; ok {
		match.ListenUpID = listenUpID
		match.Confidence = MatchDefinitive
		match.MatchReason = "admin-configured mapping"
		return match
	}

	// 2. ASIN match (definitive)
	if meta.HasASIN() {
		if cached, ok := m.bookCache.byASIN[meta.ASIN]; ok {
			match.ListenUpID = cached.ID
			match.Confidence = MatchDefinitive
			match.MatchReason = "ASIN match: " + meta.ASIN
			return match
		}
	}

	// 3. ISBN match (definitive)
	if meta.HasISBN() {
		if cached, ok := m.bookCache.byISBN[meta.ISBN]; ok {
			match.ListenUpID = cached.ID
			match.Confidence = MatchDefinitive
			match.MatchReason = "ISBN match: " + meta.ISBN
			return match
		}
	}

	// 4. Path match (strong)
	if m.opts.MatchByPath {
		if cached, ok := m.bookCache.byPath[absItem.Path]; ok {
			match.ListenUpID = cached.ID
			match.Confidence = MatchStrong
			match.MatchReason = "path match: " + absItem.Path
			return match
		}
	}

	// 5. Fuzzy match by title + author + duration
	if m.opts.FuzzyMatchBooks {
		if fuzzyMatch := m.fuzzyMatchBookFast(absItem); fuzzyMatch != nil {
			return fuzzyMatch
		}
	}

	// 6. No match - build suggestions from cache
	match.Suggestions = m.suggestBooksFast(absItem)

	return match
}

// fuzzyMatchBookFast matches by title + author + duration using cache.
func (m *Matcher) fuzzyMatchBookFast(absItem *LibraryItem) *BookMatch {
	meta := absItem.Media.Metadata
	absDurationMs := absItem.Media.DurationMs()
	absNormTitle := normalizeTitle(meta.Title)

	// Get candidates with similar titles
	candidates := m.bookCache.byTitle[absNormTitle]

	// Also check books with slightly different titles
	for normTitle, books := range m.bookCache.byTitle {
		if normTitle == absNormTitle {
			continue
		}
		titleSim := stringSimilarity(normTitle, absNormTitle)
		if titleSim >= m.opts.FuzzyThreshold {
			candidates = append(candidates, books...)
		}
	}

	for _, candidate := range candidates {
		titleSim := stringSimilarity(candidate.NormTitle, absNormTitle)

		if titleSim < m.opts.FuzzyThreshold {
			continue
		}

		// Check for common author
		if !m.hasCommonAuthorFast(candidate.Authors, meta.Authors) {
			continue
		}

		// Duration must be within tolerance (2% or 60 seconds)
		tolerance := max(absDurationMs/50, 60000)
		durationDiff := abs(candidate.TotalDuration - absDurationMs)

		if durationDiff > tolerance {
			continue
		}

		// Match found
		confidence := MatchStrong
		if titleSim < 0.95 || durationDiff > 30000 {
			confidence = MatchWeak
		}

		return &BookMatch{
			ABSItem:     absItem,
			ListenUpID:  candidate.ID,
			Confidence:  confidence,
			MatchReason: formatMatchReason(titleSim, durationDiff, absDurationMs),
		}
	}

	return nil
}

// hasCommonAuthorFast checks if cached authors match ABS authors.
func (m *Matcher) hasCommonAuthorFast(cachedAuthors []string, absAuthors []PersonRef) bool {
	if len(absAuthors) == 0 {
		return true
	}

	for _, cachedAuthor := range cachedAuthors {
		cachedNorm := normalizeString(cachedAuthor)
		for _, absAuthor := range absAuthors {
			if stringSimilarity(cachedNorm, normalizeString(absAuthor.Name)) >= 0.85 {
				return true
			}
		}
	}

	return false
}

// suggestBooksFast builds suggestions from cache.
func (m *Matcher) suggestBooksFast(absItem *LibraryItem) []BookSuggestion {
	var suggestions []BookSuggestion
	meta := absItem.Media.Metadata
	absNormTitle := normalizeTitle(meta.Title)
	absDurationMs := absItem.Media.DurationMs()

	for _, candidate := range m.bookCache.all {
		titleSim := stringSimilarity(candidate.NormTitle, absNormTitle)

		if titleSim < 0.5 {
			continue
		}

		durationDiff := abs(candidate.TotalDuration - absDurationMs)
		durationScore := 1.0 - float64(durationDiff)/float64(max(absDurationMs, 1))
		if durationScore < 0 {
			durationScore = 0
		}

		score := titleSim*0.7 + durationScore*0.3

		if score >= 0.5 {
			author := ""
			if len(candidate.Authors) > 0 {
				author = candidate.Authors[0]
			}

			suggestions = append(suggestions, BookSuggestion{
				BookID:     candidate.ID,
				Title:      candidate.Title,
				Author:     author,
				DurationMs: candidate.TotalDuration,
				Score:      score,
				Reason:     formatSuggestionReason(titleSim, durationDiff),
			})
		}

		// Limit suggestions
		if len(suggestions) >= 5 {
			break
		}
	}

	return suggestions
}

// MatchBook attempts to find a ListenUp book for an ABS library item.
func (m *Matcher) MatchBook(ctx context.Context, absItem *LibraryItem) *BookMatch {
	match := &BookMatch{
		ABSItem:    absItem,
		Confidence: MatchNone,
	}

	meta := absItem.Media.Metadata

	// 1. Check admin-specified mapping (definitive)
	if listenUpID, ok := m.opts.BookMappings[absItem.ID]; ok {
		match.ListenUpID = listenUpID
		match.Confidence = MatchDefinitive
		match.MatchReason = "admin-configured mapping"
		return match
	}

	// 2. ASIN match (definitive)
	if meta.HasASIN() {
		book, err := m.store.GetBookByASIN(ctx, meta.ASIN)
		if err == nil && book != nil {
			match.ListenUpID = book.ID
			match.Confidence = MatchDefinitive
			match.MatchReason = "ASIN match: " + meta.ASIN
			return match
		}
	}

	// 3. ISBN match (definitive)
	if meta.HasISBN() {
		book, err := m.store.GetBookByISBN(ctx, meta.ISBN)
		if err == nil && book != nil {
			match.ListenUpID = book.ID
			match.Confidence = MatchDefinitive
			match.MatchReason = "ISBN match: " + meta.ISBN
			return match
		}
	}

	// 4. Path match (strong)
	if m.opts.MatchByPath {
		book, err := m.store.GetBookByPath(ctx, absItem.Path)
		if err == nil && book != nil {
			match.ListenUpID = book.ID
			match.Confidence = MatchStrong
			match.MatchReason = "path match: " + absItem.Path
			return match
		}
	}

	// 5. Fuzzy match (strong if very similar, weak otherwise)
	if m.opts.FuzzyMatchBooks {
		if fuzzyMatch := m.fuzzyMatchBook(ctx, absItem); fuzzyMatch != nil {
			return fuzzyMatch
		}
	}

	// 6. No match - find suggestions
	match.Suggestions = m.suggestBooks(ctx, absItem)

	return match
}

// fuzzyMatchBook attempts to match by title + author + duration.
func (m *Matcher) fuzzyMatchBook(ctx context.Context, absItem *LibraryItem) *BookMatch {
	meta := absItem.Media.Metadata
	absDurationMs := absItem.Media.DurationMs()

	// Search by title
	candidates, err := m.store.SearchBooksByTitle(ctx, meta.Title)
	if err != nil || len(candidates) == 0 {
		return nil
	}

	for _, candidate := range candidates {
		// Calculate title similarity
		titleSim := stringSimilarity(
			normalizeTitle(candidate.Title),
			normalizeTitle(meta.Title),
		)

		// Title must be at least threshold similar
		if titleSim < m.opts.FuzzyThreshold {
			continue
		}

		// Check for common author
		if !m.hasCommonAuthor(ctx, candidate.ID, meta.Authors) {
			continue
		}

		// Duration must be within tolerance (2% or 60 seconds)
		tolerance := max(absDurationMs/50, 60000) // 2% or 60s
		durationDiff := abs(candidate.TotalDuration - absDurationMs)

		if durationDiff > tolerance {
			continue
		}

		// We have a match - determine confidence
		confidence := MatchStrong
		if titleSim < 0.95 || durationDiff > 30000 {
			confidence = MatchWeak // Lower confidence if not perfect match
		}

		return &BookMatch{
			ABSItem:    absItem,
			ListenUpID: candidate.ID,
			Confidence: confidence,
			MatchReason: formatMatchReason(
				titleSim,
				durationDiff,
				absDurationMs,
			),
		}
	}

	return nil
}

// hasCommonAuthor checks if a book has any author in common with the ABS authors.
func (m *Matcher) hasCommonAuthor(ctx context.Context, bookID string, absAuthors []PersonRef) bool {
	if len(absAuthors) == 0 {
		return true // No authors to check, assume match
	}

	book, err := m.store.GetBookNoAccessCheck(ctx, bookID)
	if err != nil || book == nil {
		return false
	}

	for _, bc := range book.Contributors {
		contributor, err := m.store.GetContributor(ctx, bc.ContributorID)
		if err != nil {
			continue
		}

		for _, absAuthor := range absAuthors {
			if stringSimilarity(
				normalizeString(contributor.Name),
				normalizeString(absAuthor.Name),
			) >= 0.85 {
				return true
			}
		}
	}

	return false
}

// suggestBooks finds potential ListenUp books that might match an ABS item.
func (m *Matcher) suggestBooks(ctx context.Context, absItem *LibraryItem) []BookSuggestion {
	var suggestions []BookSuggestion
	meta := absItem.Media.Metadata
	absDurationMs := absItem.Media.DurationMs()

	// Search by title
	candidates, err := m.store.SearchBooksByTitle(ctx, meta.Title)
	if err != nil {
		return nil
	}

	for _, candidate := range candidates {
		titleSim := stringSimilarity(
			normalizeTitle(candidate.Title),
			normalizeTitle(meta.Title),
		)

		// Score considers title similarity and duration closeness
		durationDiff := abs(candidate.TotalDuration - absDurationMs)
		durationScore := 1.0 - float64(durationDiff)/float64(max(absDurationMs, 1))
		if durationScore < 0 {
			durationScore = 0
		}

		score := titleSim*0.7 + durationScore*0.3

		if score >= 0.5 {
			// Get primary author for display
			author := m.getBookPrimaryAuthor(ctx, candidate.ID)

			suggestions = append(suggestions, BookSuggestion{
				BookID:     candidate.ID,
				Title:      candidate.Title,
				Author:     author,
				DurationMs: candidate.TotalDuration,
				Score:      score,
				Reason:     formatSuggestionReason(titleSim, durationDiff),
			})
		}
	}

	return suggestions
}

// getBookPrimaryAuthor returns the first author's name for a book.
func (m *Matcher) getBookPrimaryAuthor(ctx context.Context, bookID string) string {
	book, err := m.store.GetBookNoAccessCheck(ctx, bookID)
	if err != nil || len(book.Contributors) == 0 {
		return ""
	}

	contributor, err := m.store.GetContributor(ctx, book.Contributors[0].ContributorID)
	if err != nil {
		return ""
	}

	return contributor.Name
}

// String similarity functions

// normalizeString normalizes a string for comparison.
func normalizeString(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSpace(s)
	return s
}

// normalizeTitle normalizes a book title for comparison.
// Removes common prefixes like "The", "A", "An" and punctuation.
func normalizeTitle(s string) string {
	s = normalizeString(s)

	// Remove leading articles
	for _, article := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(s, article) {
			s = s[len(article):]
			break
		}
	}

	// Remove punctuation
	var result strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsSpace(r) {
			result.WriteRune(r)
		}
	}

	// Collapse multiple spaces
	return strings.Join(strings.Fields(result.String()), " ")
}

// stringSimilarity calculates the similarity between two strings (0.0-1.0).
// Uses Jaro-Winkler-like similarity optimized for book titles.
func stringSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}

	// Simple Levenshtein-based similarity
	distance := levenshteinDistance(a, b)
	maxLen := max(len(b), len(a))

	return 1.0 - float64(distance)/float64(maxLen)
}

// levenshteinDistance calculates the edit distance between two strings.
func levenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Create matrix
	matrix := make([][]int, len(a)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(b)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(a)][len(b)]
}

func formatMatchReason(titleSim float64, durationDiff, totalDuration int64) string {
	durationSec := durationDiff / 1000
	return fmt.Sprintf("fuzzy match: title=%.0f%%, duration diff=%ds", titleSim*100, durationSec)
}

func formatSuggestionReason(titleSim float64, durationDiff int64) string {
	if titleSim >= 0.95 {
		return "exact title match"
	}
	if titleSim >= 0.85 {
		return "similar title"
	}
	return "partial title match"
}

// Helper functions

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func min(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= c {
		return b
	}
	return c
}
