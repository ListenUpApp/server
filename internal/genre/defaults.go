package genre

// GenreSeed defines a genre for seeding the default tree.
type GenreSeed struct {
	Name     string
	Slug     string
	Children []GenreSeed
}

// DefaultGenres is the default genre hierarchy.
// Users can customize this after initial setup.
var DefaultGenres = []GenreSeed{
	{
		Name: "Fiction",
		Slug: "fiction",
		Children: []GenreSeed{
			{
				Name: "Fantasy",
				Slug: "fantasy",
				Children: []GenreSeed{
					{Name: "Epic Fantasy", Slug: "epic-fantasy"},
					{Name: "Urban Fantasy", Slug: "urban-fantasy"},
					{Name: "Dark Fantasy", Slug: "dark-fantasy"},
					{Name: "LitRPG", Slug: "litrpg"},
					{Name: "Progression Fantasy", Slug: "progression-fantasy"},
					{Name: "Sword & Sorcery", Slug: "sword-and-sorcery"},
					{Name: "Romantasy", Slug: "romantasy"},
					{Name: "Grimdark", Slug: "grimdark"},
					{Name: "Portal Fantasy", Slug: "portal-fantasy"},
					{Name: "Fairy Tale Retelling", Slug: "fairy-tale-retelling"},
				},
			},
			{
				Name: "Science Fiction",
				Slug: "science-fiction",
				Children: []GenreSeed{
					{Name: "Hard Sci-Fi", Slug: "hard-sci-fi"},
					{Name: "Space Opera", Slug: "space-opera"},
					{Name: "Cyberpunk", Slug: "cyberpunk"},
					{Name: "Post-Apocalyptic", Slug: "post-apocalyptic"},
					{Name: "Military Sci-Fi", Slug: "military-sci-fi"},
					{Name: "First Contact", Slug: "first-contact"},
					{Name: "Time Travel", Slug: "time-travel"},
					{Name: "Dystopian", Slug: "dystopian"},
				},
			},
			{
				Name: "Mystery & Thriller",
				Slug: "mystery-thriller",
				Children: []GenreSeed{
					{Name: "Cozy Mystery", Slug: "cozy-mystery"},
					{Name: "Police Procedural", Slug: "police-procedural"},
					{Name: "Legal Thriller", Slug: "legal-thriller"},
					{Name: "Psychological Thriller", Slug: "psychological-thriller"},
					{Name: "Espionage", Slug: "espionage"},
					{Name: "Crime Fiction", Slug: "crime-fiction"},
					{Name: "Noir", Slug: "noir"},
				},
			},
			{
				Name: "Romance",
				Slug: "romance",
				Children: []GenreSeed{
					{Name: "Contemporary Romance", Slug: "contemporary-romance"},
					{Name: "Historical Romance", Slug: "historical-romance"},
					{Name: "Paranormal Romance", Slug: "paranormal-romance"},
					{Name: "Romantic Suspense", Slug: "romantic-suspense"},
					{Name: "Romantic Comedy", Slug: "romantic-comedy"},
				},
			},
			{
				Name: "Horror",
				Slug: "horror",
				Children: []GenreSeed{
					{Name: "Supernatural Horror", Slug: "supernatural-horror"},
					{Name: "Cosmic Horror", Slug: "cosmic-horror"},
					{Name: "Gothic Horror", Slug: "gothic-horror"},
					{Name: "Slasher", Slug: "slasher"},
				},
			},
			{
				Name: "Literary Fiction",
				Slug: "literary-fiction",
			},
			{
				Name: "Historical Fiction",
				Slug: "historical-fiction",
				Children: []GenreSeed{
					{Name: "Ancient History", Slug: "ancient-history-fiction"},
					{Name: "Medieval", Slug: "medieval-fiction"},
					{Name: "Victorian", Slug: "victorian-fiction"},
					{Name: "World War", Slug: "world-war-fiction"},
				},
			},
			{
				Name: "Adventure",
				Slug: "adventure",
			},
			{
				Name: "Humor",
				Slug: "humor",
			},
			{
				Name: "Western",
				Slug: "western",
			},
		},
	},
	{
		Name: "Non-Fiction",
		Slug: "non-fiction",
		Children: []GenreSeed{
			{
				Name: "Biography & Memoir",
				Slug: "biography-memoir",
				Children: []GenreSeed{
					{Name: "Autobiography", Slug: "autobiography"},
					{Name: "Biography", Slug: "biography"},
					{Name: "Memoir", Slug: "memoir"},
				},
			},
			{
				Name: "Self-Help & Personal Development",
				Slug: "self-help",
				Children: []GenreSeed{
					{Name: "Productivity", Slug: "productivity"},
					{Name: "Relationships", Slug: "relationships"},
					{Name: "Mental Health", Slug: "mental-health"},
					{Name: "Mindfulness", Slug: "mindfulness"},
				},
			},
			{
				Name: "Business & Finance",
				Slug: "business-finance",
				Children: []GenreSeed{
					{Name: "Entrepreneurship", Slug: "entrepreneurship"},
					{Name: "Investing", Slug: "investing"},
					{Name: "Leadership", Slug: "leadership"},
					{Name: "Marketing", Slug: "marketing"},
				},
			},
			{
				Name: "History",
				Slug: "history",
				Children: []GenreSeed{
					{Name: "Ancient History", Slug: "ancient-history"},
					{Name: "Modern History", Slug: "modern-history"},
					{Name: "Military History", Slug: "military-history"},
				},
			},
			{
				Name: "Science & Nature",
				Slug: "science-nature",
				Children: []GenreSeed{
					{Name: "Physics", Slug: "physics"},
					{Name: "Biology", Slug: "biology"},
					{Name: "Astronomy", Slug: "astronomy"},
					{Name: "Environment", Slug: "environment"},
				},
			},
			{
				Name: "True Crime",
				Slug: "true-crime",
			},
			{
				Name: "Religion & Spirituality",
				Slug: "religion-spirituality",
			},
			{
				Name: "Philosophy",
				Slug: "philosophy",
			},
			{
				Name: "Health & Fitness",
				Slug: "health-fitness",
			},
			{
				Name: "Travel",
				Slug: "travel",
			},
			{
				Name: "Cooking & Food",
				Slug: "cooking-food",
			},
			{
				Name: "Politics & Social Sciences",
				Slug: "politics-social",
			},
			{
				Name: "Technology",
				Slug: "technology",
			},
		},
	},
	{
		Name: "Children's & Young Adult",
		Slug: "children-young-adult",
		Children: []GenreSeed{
			{Name: "Picture Books", Slug: "picture-books"},
			{Name: "Middle Grade", Slug: "middle-grade"},
			{
				Name: "Young Adult",
				Slug: "young-adult",
				Children: []GenreSeed{
					{Name: "YA Fantasy", Slug: "ya-fantasy"},
					{Name: "YA Sci-Fi", Slug: "ya-sci-fi"},
					{Name: "YA Romance", Slug: "ya-romance"},
					{Name: "YA Contemporary", Slug: "ya-contemporary"},
				},
			},
		},
	},
}
