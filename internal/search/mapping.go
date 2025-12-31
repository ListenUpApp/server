package search

import (
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/keyword"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/simple"
	"github.com/blevesearch/bleve/v2/analysis/lang/en"
	"github.com/blevesearch/bleve/v2/mapping"
)

// buildIndexMapping creates the Bleve index mapping for search documents.
//
// The mapping is designed with these priorities:
//  1. Fast full-text search on names/titles with English stemming
//  2. Boosted relevance for author/narrator matches
//  3. Exact keyword matching for type and genre filters
//  4. Numeric range queries for duration and year
//  5. Term vectors enabled on key fields for highlighting
func buildIndexMapping() mapping.IndexMapping {
	// Create the index mapping
	indexMapping := bleve.NewIndexMapping()

	// Use English analyzer as default for text fields
	indexMapping.DefaultAnalyzer = en.AnalyzerName

	// Create document mapping
	docMapping := bleve.NewDocumentMapping()

	// --- Text fields (full-text searchable) ---

	// Name field - primary search target, boosted
	nameFieldMapping := bleve.NewTextFieldMapping()
	nameFieldMapping.Analyzer = en.AnalyzerName
	nameFieldMapping.Store = true
	nameFieldMapping.IncludeTermVectors = true // For highlighting
	docMapping.AddFieldMappingsAt("name", nameFieldMapping)

	// Subtitle - searchable text
	subtitleFieldMapping := bleve.NewTextFieldMapping()
	subtitleFieldMapping.Analyzer = en.AnalyzerName
	subtitleFieldMapping.Store = true
	docMapping.AddFieldMappingsAt("subtitle", subtitleFieldMapping)

	// Description - searchable but not stored (too large)
	descFieldMapping := bleve.NewTextFieldMapping()
	descFieldMapping.Analyzer = en.AnalyzerName
	descFieldMapping.Store = false
	docMapping.AddFieldMappingsAt("description", descFieldMapping)

	// Author - searchable, important for book search
	authorFieldMapping := bleve.NewTextFieldMapping()
	authorFieldMapping.Analyzer = en.AnalyzerName
	authorFieldMapping.Store = true
	authorFieldMapping.IncludeTermVectors = true // For highlighting
	docMapping.AddFieldMappingsAt("author", authorFieldMapping)

	// Narrator - searchable
	narratorFieldMapping := bleve.NewTextFieldMapping()
	narratorFieldMapping.Analyzer = en.AnalyzerName
	narratorFieldMapping.Store = true
	narratorFieldMapping.IncludeTermVectors = true // For highlighting
	docMapping.AddFieldMappingsAt("narrator", narratorFieldMapping)

	// Series name - searchable
	seriesFieldMapping := bleve.NewTextFieldMapping()
	seriesFieldMapping.Analyzer = en.AnalyzerName
	seriesFieldMapping.Store = true
	seriesFieldMapping.IncludeTermVectors = true // For highlighting
	docMapping.AddFieldMappingsAt("series_name", seriesFieldMapping)

	// Biography - searchable for contributors
	bioFieldMapping := bleve.NewTextFieldMapping()
	bioFieldMapping.Analyzer = en.AnalyzerName
	bioFieldMapping.Store = false
	docMapping.AddFieldMappingsAt("biography", bioFieldMapping)

	// Publisher - searchable with simple analyzer (no stemming)
	publisherFieldMapping := bleve.NewTextFieldMapping()
	publisherFieldMapping.Analyzer = simple.Name
	publisherFieldMapping.Store = true
	docMapping.AddFieldMappingsAt("publisher", publisherFieldMapping)

	// --- Keyword fields (exact match, facetable) ---

	// Type - for filtering by document type
	typeFieldMapping := bleve.NewTextFieldMapping()
	typeFieldMapping.Analyzer = keyword.Name
	docMapping.AddFieldMappingsAt("type", typeFieldMapping)

	// ID - stored but not analyzed
	idFieldMapping := bleve.NewTextFieldMapping()
	idFieldMapping.Analyzer = keyword.Name
	docMapping.AddFieldMappingsAt("id", idFieldMapping)

	// Genre paths - for hierarchical faceting
	// Keyword analyzer for exact prefix matching
	genrePathsFieldMapping := bleve.NewTextFieldMapping()
	genrePathsFieldMapping.Analyzer = keyword.Name
	docMapping.AddFieldMappingsAt("genre_paths", genrePathsFieldMapping)

	// Genre slugs - for exact genre filtering and display
	genreSlugsFieldMapping := bleve.NewTextFieldMapping()
	genreSlugsFieldMapping.Analyzer = keyword.Name
	genreSlugsFieldMapping.Store = true // Store for retrieval in search results
	docMapping.AddFieldMappingsAt("genre_slugs", genreSlugsFieldMapping)

	// Tags - community-applied content descriptors
	// Keyword analyzer keeps compound slugs intact (e.g., "slow-burn")
	tagsFieldMapping := bleve.NewTextFieldMapping()
	tagsFieldMapping.Analyzer = keyword.Name
	tagsFieldMapping.Store = true
	tagsFieldMapping.IncludeTermVectors = true // For faceting
	docMapping.AddFieldMappingsAt("tags", tagsFieldMapping)

	// --- Numeric fields (range queries, sorting) ---

	// Duration - for range filtering
	durationFieldMapping := bleve.NewNumericFieldMapping()
	durationFieldMapping.Store = true
	docMapping.AddFieldMappingsAt("duration", durationFieldMapping)

	// Publish year - for range filtering
	yearFieldMapping := bleve.NewNumericFieldMapping()
	yearFieldMapping.Store = true
	docMapping.AddFieldMappingsAt("publish_year", yearFieldMapping)

	// Book count - for sorting contributors/series
	bookCountFieldMapping := bleve.NewNumericFieldMapping()
	bookCountFieldMapping.Store = true
	docMapping.AddFieldMappingsAt("book_count", bookCountFieldMapping)

	// Timestamps - for sorting by recency
	createdAtFieldMapping := bleve.NewNumericFieldMapping()
	createdAtFieldMapping.Store = true
	docMapping.AddFieldMappingsAt("created_at", createdAtFieldMapping)

	updatedAtFieldMapping := bleve.NewNumericFieldMapping()
	updatedAtFieldMapping.Store = true
	docMapping.AddFieldMappingsAt("updated_at", updatedAtFieldMapping)

	// Register the document mapping
	indexMapping.AddDocumentMapping("_default", docMapping)

	return indexMapping
}
