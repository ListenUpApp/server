// Package main provides a tool to seed the database with test listening data.
//
// This reads existing books and users from the database and creates realistic
// listening events to test stats and leaderboard features.
//
// Usage:
//
//	DB_PATH=~/listenUp/db go run ./cmd/seed
//	DB_PATH=~/listenUp/db go run ./cmd/seed --create-users  # Also create test users
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/store"
	"golang.org/x/crypto/bcrypt"
)

var createUsers = flag.Bool("create-users", false, "Create test users for leaderboard testing")

func main() {
	flag.Parse()

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = os.ExpandEnv("$HOME/listenUp/db")
	}

	fmt.Printf("Opening database at: %s\n", dbPath)

	// Open store (not read-only since we're writing)
	s, err := store.New(dbPath, nil, store.NewNoopEmitter())
	if err != nil {
		log.Fatalf("Failed to open store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Optionally create test users
	if *createUsers {
		createTestUsers(ctx, s)
	}

	// Get all users
	users, err := s.ListUsers(ctx)
	if err != nil {
		log.Fatalf("Failed to get users: %v", err)
	}

	if len(users) == 0 {
		log.Fatal("No users found in database. Create a user first.")
	}

	fmt.Printf("Found %d users\n", len(users))

	// Seed random for variety (Go 1.20+ auto-seeds, but explicit for clarity)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// For each user, create listening events using books they have access to
	for _, user := range users {
		fmt.Printf("\nSeeding data for user: %s (%s)\n", user.DisplayName, user.ID)

		// Get books this user has access to (through collections)
		books, err := s.GetBooksForUser(ctx, user.ID)
		if err != nil {
			log.Printf("Failed to get books for user %s: %v", user.ID, err)
			continue
		}

		// If no collection-accessible books, try using all books in the database
		// This handles edge cases where collections aren't set up yet
		if len(books) == 0 {
			fmt.Printf("  No collection-accessible books, trying all books...\n")
			allBooks, err := s.ListAllBooks(ctx)
			if err != nil {
				log.Printf("Failed to list all books: %v", err)
				continue
			}
			books = allBooks
		}

		if len(books) == 0 {
			fmt.Printf("  No books in database for this user, skipping\n")
			continue
		}

		fmt.Printf("  User has access to %d books\n", len(books))

		// Pick 3-5 random books for this user
		numBooks := min(3+rng.Intn(3), len(books))

		// Shuffle and pick books
		shuffled := make([]*domain.Book, len(books))
		copy(shuffled, books)
		rng.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		selectedBooks := shuffled[:numBooks]

		// Create listening events over the past 14 days
		now := time.Now()
		eventsCreated := 0

		for day := 13; day >= 0; day-- {
			// Always create events for today and yesterday to ensure an active streak
			// For other days, 80% chance of listening (randomness for realism)
			if day > 1 && rng.Float32() > 0.8 {
				continue
			}

			// Pick 1-3 listening sessions per day
			sessionsPerDay := 1 + rng.Intn(3)

			for range sessionsPerDay {
				// Pick a random book from selected
				book := selectedBooks[rng.Intn(len(selectedBooks))]

				// Random time during the day (6am - 11pm)
				hour := 6 + rng.Intn(17)
				minute := rng.Intn(60)
				sessionStart := time.Date(
					now.Year(), now.Month(), now.Day()-day,
					hour, minute, 0, 0, time.Local,
				)

				// Session duration: 5-45 minutes
				durationMinutes := 5 + rng.Intn(40)
				durationMs := int64(durationMinutes * 60 * 1000)

				// Random position in book (0 to 80% of duration)
				maxStart := book.TotalDuration * 80 / 100
				if maxStart <= 0 {
					maxStart = 3600000 // Default 1 hour if no duration
				}
				startPos := rng.Int63n(maxStart)
				endPos := startPos + durationMs

				event := &domain.ListeningEvent{
					ID:              id.MustGenerate("evt"),
					UserID:          user.ID,
					BookID:          book.ID,
					StartPositionMs: startPos,
					EndPositionMs:   endPos,
					StartedAt:       sessionStart,
					EndedAt:         sessionStart.Add(time.Duration(durationMinutes) * time.Minute),
					PlaybackSpeed:   1.0,
					DeviceID:        "seed-device",
					DeviceName:      "Seed Tool",
					DurationMs:      durationMs,
					CreatedAt:       time.Now(),
				}

				if err := createListeningEvent(ctx, s, event); err != nil {
					log.Printf("Failed to create event: %v", err)
					continue
				}

				eventsCreated++
			}
		}

		fmt.Printf("  Created %d listening events across %d books\n", eventsCreated, numBooks)

		// Also create/update progress for selected books
		for _, book := range selectedBooks {
			// Random progress: 10-90%
			progressPct := 10 + rng.Intn(80)
			positionMs := book.TotalDuration * int64(progressPct) / 100

			progress := &domain.PlaybackProgress{
				UserID:            user.ID,
				BookID:            book.ID,
				CurrentPositionMs: positionMs,
				Progress:          float64(progressPct) / 100.0,
				StartedAt:         now.AddDate(0, 0, -rng.Intn(14)),
				LastPlayedAt:      now,
				UpdatedAt:         now,
			}

			if err := s.UpsertProgress(ctx, progress); err != nil {
				log.Printf("Failed to update progress for %s: %v", book.Title, err)
			} else {
				fmt.Printf("  Updated progress for: %s (%d%%)\n", book.Title, progressPct)
			}
		}
	}

	fmt.Println("\nSeeding complete!")
}

// createListeningEvent creates a listening event using the store.
func createListeningEvent(ctx context.Context, s *store.Store, event *domain.ListeningEvent) error {
	return s.CreateListeningEvent(ctx, event)
}

// testUserNames are display names for generated test users.
var testUserNames = []string{
	"Alex Rivera",
	"Jordan Chen",
	"Sam Taylor",
	"Casey Morgan",
	"Riley Kim",
}

// createTestUsers creates test users and shares collections with them.
func createTestUsers(ctx context.Context, s *store.Store) {
	fmt.Println("\n=== Creating Test Users ===")

	// Get existing users to find an admin to share from
	existingUsers, err := s.ListUsers(ctx)
	if err != nil {
		log.Printf("Failed to list existing users: %v", err)
		return
	}

	// Find an admin user to share collections from
	var adminUser *domain.User
	for _, u := range existingUsers {
		if u.IsAdmin() {
			adminUser = u
			break
		}
	}

	if adminUser == nil {
		log.Println("No admin user found to share collections from")
		return
	}

	// Get all libraries
	libraries, err := s.ListLibraries(ctx)
	if err != nil {
		log.Printf("Failed to list libraries: %v", err)
		return
	}

	// Collect all collections from all libraries
	var allCollections []*domain.Collection
	for _, lib := range libraries {
		colls, err := s.ListAllCollectionsByLibrary(ctx, lib.ID)
		if err != nil {
			log.Printf("Failed to list collections for library %s: %v", lib.ID, err)
			continue
		}
		allCollections = append(allCollections, colls...)
	}

	if len(allCollections) == 0 {
		log.Println("No collections found to share with test users")
		return
	}

	fmt.Printf("Found %d collections to share\n", len(allCollections))

	// Hash a simple password for test users
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("testpass123"), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Failed to hash password: %v", err)
		return
	}

	now := time.Now()

	// Create test users
	for i, name := range testUserNames {
		userID := id.MustGenerate("usr")
		email := fmt.Sprintf("test%d@example.com", i+1)

		// Check if user with this email already exists
		if existing, _ := s.GetUserByEmail(ctx, email); existing != nil {
			fmt.Printf("  User %s already exists, skipping\n", email)
			continue
		}

		// Split name into first and last
		parts := []rune(name)
		midpoint := len(parts) / 2
		for i := midpoint; i < len(parts); i++ {
			if parts[i] == ' ' {
				midpoint = i
				break
			}
		}

		user := &domain.User{
			Syncable: domain.Syncable{
				ID:        userID,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Email:        email,
			PasswordHash: string(passwordHash),
			Role:         domain.RoleMember,
			Status:       domain.UserStatusActive,
			DisplayName:  name,
			FirstName:    string(parts[:midpoint]),
			LastName:     string(parts[midpoint+1:]),
		}

		if err := s.CreateUser(ctx, user); err != nil {
			log.Printf("  Failed to create user %s: %v", name, err)
			continue
		}

		fmt.Printf("  Created user: %s (%s)\n", name, email)

		// Share all collections with this user
		for _, coll := range allCollections {
			share := &domain.CollectionShare{
				Syncable: domain.Syncable{
					ID:        id.MustGenerate("shr"),
					CreatedAt: now,
					UpdatedAt: now,
				},
				CollectionID:     coll.ID,
				SharedWithUserID: userID,
				SharedByUserID:   adminUser.ID,
				Permission:       domain.PermissionRead,
			}

			if err := s.CreateShare(ctx, share); err != nil {
				log.Printf("    Failed to share collection %s: %v", coll.Name, err)
				continue
			}
		}

		fmt.Printf("    Shared %d collections with %s\n", len(allCollections), name)
	}

	fmt.Println("=== Test Users Created ===")
}
