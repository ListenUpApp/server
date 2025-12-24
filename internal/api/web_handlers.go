package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// registerWebRoutes sets up web-facing routes (join page, deep links, etc.)
func (s *Server) registerWebRoutes() {
	// Join page for invite links
	huma.Register(s.api, huma.Operation{
		OperationID: "getJoinPage",
		Method:      http.MethodGet,
		Path:        "/join/{code}",
		Summary:     "Join page",
		Description: "Returns HTML page for claiming an invite",
		Tags:        []string{"Web"},
	}, s.handleGetJoinPage)

	// Android App Links / iOS Universal Links
	s.router.Get("/.well-known/assetlinks.json", s.handleAssetLinks)
	s.router.Get("/.well-known/apple-app-site-association", s.handleAppleAppSiteAssociation)
}

// === DTOs ===

type GetJoinPageInput struct {
	Code string `path:"code" doc:"Invite code"`
}

type HTMLOutput struct {
	ContentType string `header:"Content-Type"`
	Body        string
}

// === Handlers ===

func (s *Server) handleGetJoinPage(ctx context.Context, input *GetJoinPageInput) (*HTMLOutput, error) {
	// Check if invite exists
	_, err := s.services.Invite.GetInviteDetails(ctx, input.Code)
	if err != nil {
		return &HTMLOutput{
			ContentType: "text/html; charset=utf-8",
			Body: `<!DOCTYPE html>
<html>
<head>
    <title>Invalid Invite</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; max-width: 400px; margin: 50px auto; padding: 20px; text-align: center; }
        h1 { color: #d32f2f; }
    </style>
</head>
<body>
    <h1>Invalid Invite</h1>
    <p>This invite link is invalid or has expired.</p>
</body>
</html>`,
		}, nil
	}

	// Return HTML that redirects to app or shows install prompt
	return &HTMLOutput{
		ContentType: "text/html; charset=utf-8",
		Body: `<!DOCTYPE html>
<html>
<head>
    <title>Join ListenUp</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <meta property="og:title" content="You're invited to ListenUp">
    <meta property="og:description" content="Click to join this audiobook library">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; max-width: 400px; margin: 50px auto; padding: 20px; text-align: center; }
        h1 { color: #1976d2; }
        .btn { display: inline-block; background: #1976d2; color: white; padding: 12px 24px; border-radius: 8px; text-decoration: none; margin: 10px; }
        .btn:hover { background: #1565c0; }
    </style>
    <script>
        // Try to open in app
        window.location.href = 'listenup://join/` + input.Code + `';
        setTimeout(function() {
            // If we're still here, app is not installed
            document.getElementById('install-prompt').style.display = 'block';
        }, 2000);
    </script>
</head>
<body>
    <h1>Join ListenUp</h1>
    <p>Opening in ListenUp app...</p>
    <div id="install-prompt" style="display:none">
        <p>Don't have the app yet?</p>
        <a href="https://play.google.com/store/apps/details?id=com.listenup" class="btn">Get on Google Play</a>
    </div>
</body>
</html>`,
	}, nil
}

func (s *Server) handleAssetLinks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`[{
  "relation": ["delegate_permission/common.handle_all_urls"],
  "target": {
    "namespace": "android_app",
    "package_name": "com.listenup",
    "sha256_cert_fingerprints": []
  }
}]`))
}

func (s *Server) handleAppleAppSiteAssociation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{
  "applinks": {
    "apps": [],
    "details": [{
      "appID": "TEAMID.com.listenup",
      "paths": ["/join/*"]
    }]
  }
}`))
}
