package api

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

func (s *Server) registerBookShareRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "getBookSharePage",
		Method:      http.MethodGet,
		Path:        "/share/book/{id}",
		Summary:     "Book share page",
		Description: "Returns HTML page with Open Graph meta tags for sharing a book",
		Tags:        []string{"Web"},
	}, s.handleGetBookSharePage)
}

// GetBookSharePageInput contains parameters for the book share page.
type GetBookSharePageInput struct {
	ID        string `path:"id" doc:"Book ID"`
	UserAgent string `header:"User-Agent"`
}

func (s *Server) handleGetBookSharePage(ctx context.Context, input *GetBookSharePageInput) (*HTMLOutput, error) {
	book, err := s.store.GetBookNoAccessCheck(ctx, input.ID)
	if err != nil {
		return &HTMLOutput{
			ContentType: "text/html; charset=utf-8",
			Body:        renderShareErrorPage("Book Not Found", "This book could not be found."),
		}, nil
	}

	enriched, err := s.store.EnrichBook(ctx, book)
	if err != nil {
		return &HTMLOutput{
			ContentType: "text/html; charset=utf-8",
			Body:        renderShareErrorPage("Error", "Could not load book details."),
		}, nil
	}

	// Get base URL from instance config
	instance, err := s.services.Instance.GetInstance(ctx)
	if err != nil {
		return &HTMLOutput{
			ContentType: "text/html; charset=utf-8",
			Body:        renderShareErrorPage("Error", "Could not load server config."),
		}, nil
	}

	baseURL := instance.RemoteURL
	if baseURL == "" {
		baseURL = instance.LocalURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	// Detect platform from user agent
	ua := strings.ToLower(input.UserAgent)
	isAndroid := strings.Contains(ua, "android")
	isIOS := strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad") || strings.Contains(ua, "ipod")

	title := html.EscapeString(enriched.Title)
	author := html.EscapeString(enriched.Author)
	description := html.EscapeString(truncate(enriched.Description, 200))

	coverURL := fmt.Sprintf("%s/api/v1/books/%s/cover", baseURL, input.ID)
	deepLink := fmt.Sprintf("listenup://book/%s", input.ID)
	playStoreURL := "https://play.google.com/store/apps/details?id=com.calypsan.listenup"
	appStoreURL := "https://apps.apple.com/app/listenup/id0000000000"

	storeURL := playStoreURL
	if isIOS {
		storeURL = appStoreURL
	}

	ogDescription := buildOGDescription(author, description)

	pageHTML := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>%s - ListenUp</title>
    <meta property="og:type" content="book">
    <meta property="og:title" content="%s">
    <meta property="og:description" content="%s">
    <meta property="og:image" content="%s">
    <meta property="og:image:alt" content="Cover art for %s">
    <meta name="twitter:card" content="summary_large_image">
    <meta name="twitter:title" content="%s">
    <meta name="twitter:description" content="%s">
    <meta name="twitter:image" content="%s">
    <style>
        *{margin:0;padding:0;box-sizing:border-box}
        body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#121212;color:#e0e0e0;min-height:100vh;display:flex;align-items:center;justify-content:center}
        .card{max-width:400px;width:90%%;text-align:center;padding:32px 24px}
        .cover{width:200px;height:200px;object-fit:cover;border-radius:12px;box-shadow:0 8px 32px rgba(0,0,0,.5);margin-bottom:24px}
        h1{font-size:1.5rem;font-weight:700;color:#fff;margin-bottom:8px}
        .author{font-size:1rem;color:#b0b0b0;margin-bottom:16px}
        .desc{font-size:.875rem;color:#909090;line-height:1.5;margin-bottom:32px}
        .btn{display:inline-block;background:#bb86fc;color:#000;font-size:1rem;font-weight:600;padding:14px 32px;border-radius:28px;text-decoration:none;transition:background .2s}
        .btn:hover{background:#ce9ffc}
        .store{display:block;margin-top:16px;font-size:.8rem;color:#808080}
        .store a{color:#bb86fc;text-decoration:none}
    </style>
</head>
<body>
    <div class="card">
        <img class="cover" src="%s" alt="Cover art">
        <h1>%s</h1>
        <p class="author">by %s</p>
        <p class="desc">%s</p>
        <a class="btn" id="openBtn" href="%s">Open in ListenUp</a>
        <p class="store">Don't have the app? <a href="%s">Get ListenUp</a></p>
    </div>
    <script>
    document.getElementById('openBtn').addEventListener('click',function(e){
        e.preventDefault();
        var dl='%s',su='%s',t=Date.now();
        window.location=dl;
        setTimeout(function(){if(Date.now()-t<2000)window.location=su},1000);
    });
    %s
    </script>
</body>
</html>`,
		title, title, ogDescription, coverURL, title,
		title, ogDescription, coverURL,
		coverURL, title, author, description, deepLink, storeURL,
		deepLink, storeURL,
		autoRedirectScript(isAndroid, isIOS, playStoreURL, appStoreURL),
	)

	return &HTMLOutput{
		ContentType: "text/html; charset=utf-8",
		Body:        pageHTML,
	}, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func buildOGDescription(author, description string) string {
	if author != "" && description != "" {
		return fmt.Sprintf("by %s — %s", author, description)
	}
	if author != "" {
		return fmt.Sprintf("by %s", author)
	}
	return description
}

func autoRedirectScript(isAndroid, isIOS bool, playStoreURL, appStoreURL string) string {
	if isAndroid {
		return fmt.Sprintf(`setTimeout(function(){window.location='%s'},3000);`, playStoreURL)
	}
	if isIOS {
		return fmt.Sprintf(`setTimeout(function(){window.location='%s'},3000);`, appStoreURL)
	}
	return ""
}

func renderShareErrorPage(title, message string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html><head><title>%s - ListenUp</title><meta name="viewport" content="width=device-width,initial-scale=1">
<style>body{font-family:-apple-system,BlinkMacSystemFont,sans-serif;background:#121212;color:#e0e0e0;display:flex;align-items:center;justify-content:center;min-height:100vh}.card{max-width:400px;text-align:center;padding:32px}h1{color:#cf6679;margin-bottom:16px}</style>
</head><body><div class="card"><h1>%s</h1><p>%s</p></div></body></html>`, title, title, message)
}
