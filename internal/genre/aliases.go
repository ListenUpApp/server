package genre

// CanonicalAliases maps common variations to canonical slugs.
// This includes Audible's category taxonomy and common variations.
// Users can add more via GenreAlias for custom mappings.
var CanonicalAliases = map[string][]string{
	// ===========================================
	// AUDIBLE TOP-LEVEL CATEGORIES (24 categories)
	// ===========================================

	// Arts & Entertainment
	"arts-entertainment":     {"fiction"},
	"arts-and-entertainment": {"fiction"},
	"arts & entertainment":   {"fiction"},

	// Biographies & Memoirs
	"biographies-memoirs":     {"biography-memoir"},
	"biographies-and-memoirs": {"biography-memoir"},
	"biographies & memoirs":   {"biography-memoir"},
	"biography-memoirs":       {"biography-memoir"},
	"biography & memoir":      {"biography-memoir"},

	// Business & Careers
	"business-careers":     {"business-finance"},
	"business-and-careers": {"business-finance"},
	"business & careers":   {"business-finance"},

	// Children's Audiobooks
	"children-s-audiobooks": {"children-young-adult"},
	"childrens-audiobooks":  {"children-young-adult"},
	"children-s-books":      {"children-young-adult"},
	"childrens-books":       {"children-young-adult"},
	"children":              {"children-young-adult"},

	// Comedy & Humor
	"comedy-humor":     {"humor"},
	"comedy-and-humor": {"humor"},
	"comedy & humor":   {"humor"},
	"comedy":           {"humor"},

	// Computers & Technology
	"computers-technology":     {"technology"},
	"computers-and-technology": {"technology"},
	"computers & technology":   {"technology"},

	// Education & Learning
	"education-learning":     {"non-fiction"},
	"education-and-learning": {"non-fiction"},
	"education & learning":   {"non-fiction"},
	"education":              {"non-fiction"},

	// Erotica (& Sexuality)
	"erotica-sexuality": {"romance"},
	"erotica":           {"romance"},

	// Health & Wellness
	"health-wellness":     {"health-fitness"},
	"health-and-wellness": {"health-fitness"},
	"health & wellness":   {"health-fitness"},

	// History (top-level)
	"history": {"history"},

	// Home & Garden
	"home-garden":     {"non-fiction"},
	"home-and-garden": {"non-fiction"},
	"home & garden":   {"non-fiction"},

	// LGBTQ+ Audiobooks
	"lgbtq-audiobooks": {"fiction"},
	"lgbtq":            {"fiction"},
	"lgbt":             {"fiction"},

	// Literature & Fiction
	"literature-fiction":     {"fiction"},
	"literature-and-fiction": {"fiction"},
	"literature & fiction":   {"fiction"},
	"literature":             {"fiction"},

	// Money & Finance
	"money-finance":     {"business-finance"},
	"money-and-finance": {"business-finance"},
	"money & finance":   {"business-finance"},

	// Mystery, Thriller & Suspense
	"mystery-thriller-suspense":      {"mystery-thriller"},
	"mystery-thriller-and-suspense":  {"mystery-thriller"},
	"mystery-thriller & suspense":    {"mystery-thriller"},
	"mystery, thriller & suspense":   {"mystery-thriller"},
	"mystery, thriller and suspense": {"mystery-thriller"},

	// Politics & Social Sciences
	"politics-social-sciences":     {"politics-social"},
	"politics-and-social-sciences": {"politics-social"},
	"politics & social sciences":   {"politics-social"},

	// Relationships, Parenting & Personal Development
	"relationships-parenting-personal-development":     {"self-help"},
	"relationships-parenting-and-personal-development": {"self-help"},
	"parenting-families":                               {"self-help"},
	"parenting-and-families":                           {"self-help"},
	"parenting & families":                             {"self-help"},
	"personal-development":                             {"self-help"},

	// Religion & Spirituality
	"religion-spirituality":     {"religion-spirituality"},
	"religion-and-spirituality": {"religion-spirituality"},
	"religion & spirituality":   {"religion-spirituality"},

	// Romance
	"romance": {"romance"},

	// Science & Engineering
	"science-engineering":     {"science-nature"},
	"science-and-engineering": {"science-nature"},
	"science & engineering":   {"science-nature"},

	// Science Fiction & Fantasy
	"science-fiction-fantasy":     {"science-fiction", "fantasy"},
	"science-fiction-and-fantasy": {"science-fiction", "fantasy"},
	"science fiction & fantasy":   {"science-fiction", "fantasy"},
	"sci-fi-fantasy":              {"science-fiction", "fantasy"},
	"sci-fi & fantasy":            {"science-fiction", "fantasy"},

	// Sports & Outdoors
	"sports-outdoors":     {"non-fiction"},
	"sports-and-outdoors": {"non-fiction"},
	"sports & outdoors":   {"non-fiction"},

	// Teen & Young Adult
	"teens-young-adult":    {"young-adult"},
	"teen-young-adult":     {"young-adult"},
	"teen-and-young-adult": {"young-adult"},
	"teen & young adult":   {"young-adult"},

	// Travel & Tourism
	"travel-tourism":     {"travel"},
	"travel-and-tourism": {"travel"},
	"travel & tourism":   {"travel"},

	// ===========================================
	// LITERATURE & FICTION SUBCATEGORIES
	// ===========================================

	// Action & Adventure
	"action-adventure":     {"adventure"},
	"action-and-adventure": {"adventure"},
	"action & adventure":   {"adventure"},

	// African American
	"african-american":         {"fiction"},
	"african-american-fiction": {"fiction"},

	// Ancient, Classical & Medieval Literature
	"ancient-classical-medieval-literature": {"literary-fiction"},
	"classical-literature":                  {"literary-fiction"},
	"medieval-literature":                   {"literary-fiction"},

	// Anthologies & Short Stories
	"anthologies-short-stories":     {"fiction"},
	"anthologies-and-short-stories": {"fiction"},
	"anthologies & short stories":   {"fiction"},
	"short-stories":                 {"fiction"},
	"anthologies":                   {"fiction"},

	// Classics
	"classics":        {"literary-fiction"},
	"classic":         {"literary-fiction"},
	"classic-fiction": {"literary-fiction"},

	// Drama & Plays
	"drama-plays":     {"fiction"},
	"drama-and-plays": {"fiction"},
	"drama & plays":   {"fiction"},
	"drama":           {"fiction"},
	"plays":           {"fiction"},

	// Essays
	"essays": {"non-fiction"},

	// Genre Fiction
	"genre-fiction":   {"fiction"},
	"general-fiction": {"fiction"},

	// Historical Fiction
	"historical":         {"historical-fiction"},
	"historical-fiction": {"historical-fiction"},
	"historical fiction": {"historical-fiction"},

	// Horror
	"horror": {"horror"},
	"scary":  {"horror"},

	// Humor & Satire
	"humor-satire":     {"humor"},
	"humor-and-satire": {"humor"},
	"humor & satire":   {"humor"},
	"satire":           {"humor"},

	// Literary History & Criticism
	"literary-history-criticism":     {"non-fiction"},
	"literary-history-and-criticism": {"non-fiction"},
	"literary-criticism":             {"non-fiction"},

	// Poetry
	"poetry": {"fiction"},

	// Women's Fiction
	"womens-fiction":  {"fiction"},
	"women-s-fiction": {"fiction"},

	// World Literature
	"world-literature": {"literary-fiction"},

	// ===========================================
	// SCIENCE FICTION & FANTASY SUBCATEGORIES
	// ===========================================

	// Science Fiction variations
	"sci-fi":          {"science-fiction"},
	"scifi":           {"science-fiction"},
	"sf":              {"science-fiction"},
	"science fiction": {"science-fiction"},

	// Fantasy variations
	"high fantasy":      {"epic-fantasy"},
	"sword and sorcery": {"sword-and-sorcery"},
	"s&s":               {"sword-and-sorcery"},

	// Combined
	"fantasy-romance":  {"fantasy", "romance"},
	"romantic-fantasy": {"romantasy"},

	// ===========================================
	// MYSTERY, THRILLER & SUSPENSE SUBCATEGORIES
	// ===========================================

	"thriller":               {"mystery-thriller"},
	"suspense":               {"mystery-thriller"},
	"mystery-thriller":       {"mystery-thriller"},
	"mystery":                {"mystery-thriller"},
	"crime":                  {"crime-fiction"},
	"crime-fiction":          {"crime-fiction"},
	"crime fiction":          {"crime-fiction"},
	"detective":              {"mystery-thriller"},
	"traditional-detectives": {"mystery-thriller"},
	"traditional detectives": {"mystery-thriller"},
	"cozy":                   {"cozy-mystery"},
	"cozy-mystery":           {"cozy-mystery"},
	"whodunit":               {"mystery-thriller"},

	// ===========================================
	// YOUNG ADULT VARIATIONS
	// ===========================================

	"ya":          {"young-adult"},
	"young adult": {"young-adult"},
	"teen":        {"young-adult"},

	// ===========================================
	// NON-FICTION VARIATIONS
	// ===========================================

	"self-help": {"self-help"},
	"selfhelp":  {"self-help"},
	"self help": {"self-help"},

	// ===========================================
	// LITRPG / PROGRESSION FANTASY
	// ===========================================

	"litrpg":              {"litrpg"},
	"lit-rpg":             {"litrpg"},
	"lit rpg":             {"litrpg"},
	"gamelit":             {"litrpg"},
	"progression":         {"progression-fantasy"},
	"progression-fantasy": {"progression-fantasy"},
	"cultivation":         {"progression-fantasy"},

	// ===========================================
	// ROMANCE SUBCATEGORIES
	// ===========================================

	"contemporary-romance": {"contemporary-romance"},
	"modern-romance":       {"contemporary-romance"},
	"paranormal-romance":   {"paranormal-romance"},
	"pnr":                  {"paranormal-romance"},

	// ===========================================
	// ANIMAL/CHILDREN'S FICTION
	// ===========================================

	"animal-fiction": {"fiction"},
	"animal fiction": {"fiction"},
	"animals":        {"fiction"},

	// ===========================================
	// BASE CATEGORIES (identity mappings for validation)
	// ===========================================

	"fiction":     {"fiction"},
	"non-fiction": {"non-fiction"},
	"nonfiction":  {"non-fiction"},
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
