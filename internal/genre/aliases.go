package genre

// CanonicalAliases maps common variations to canonical slugs.
// This is the built-in knowledge - users can add more via GenreAlias.
var CanonicalAliases = map[string][]string{
	// Science Fiction variations -> science-fiction
	"sci-fi":          {"science-fiction"},
	"scifi":           {"science-fiction"},
	"sf":              {"science-fiction"},
	"science fiction": {"science-fiction"},

	// Fantasy variations
	"high fantasy":      {"epic-fantasy"},
	"sword and sorcery": {"sword-and-sorcery"},
	"s&s":               {"sword-and-sorcery"},

	// Combined genres -> multiple
	"sci-fi-fantasy":          {"science-fiction", "fantasy"},
	"science-fiction-fantasy": {"science-fiction", "fantasy"},
	"fantasy-romance":         {"fantasy", "romance"},
	"romantic-fantasy":        {"romantasy"},

	// Young Adult variations
	"ya":          {"young-adult"},
	"young adult": {"young-adult"},
	"teen":        {"young-adult"},

	// Mystery/Thriller
	"thriller":         {"thriller"},
	"suspense":         {"thriller"},
	"mystery-thriller": {"mystery", "thriller"},

	// Non-fiction variations
	"self-help":            {"self-help"},
	"selfhelp":             {"self-help"},
	"self help":            {"self-help"},
	"personal-development": {"self-help"},

	// LitRPG variations
	"litrpg":  {"litrpg"},
	"lit-rpg": {"litrpg"},
	"lit rpg": {"litrpg"},
	"gamelit": {"litrpg"},

	// Progression Fantasy
	"progression":         {"progression-fantasy"},
	"progression-fantasy": {"progression-fantasy"},
	"cultivation":         {"progression-fantasy"},

	// Romance variations
	"contemporary-romance": {"contemporary-romance"},
	"modern-romance":       {"contemporary-romance"},
	"paranormal-romance":   {"paranormal-romance"},
	"pnr":                  {"paranormal-romance"},

	// Horror
	"horror": {"horror"},
	"scary":  {"horror"},

	// Historical
	"historical":         {"historical-fiction"},
	"historical-fiction": {"historical-fiction"},
}

// NormalizeToSlugs takes a raw genre string and returns canonical slug(s).
// Returns the slugified input if no specific mapping found.
func NormalizeToSlugs(raw string) []string {
	slug := Slugify(raw)

	// Check built-in aliases first.
	if canonical, ok := CanonicalAliases[slug]; ok {
		return canonical
	}

	// Return the slug itself (will be checked against actual genres in the store).
	return []string{slug}
}
