package mockapi

// CloudflareErrorPage is a made-up HTML 503 page in the shape Cloudflare
// returns when the origin is down (gfsync-mock -fail-html and tests).
// Example values only.
const CloudflareErrorPage = `<!DOCTYPE html>
<html lang="en-US">
<head>
<title>example.com | 503: Service temporarily unavailable</title>
<meta charset="UTF-8" />
</head>
<body>
<div id="cf-error-details">
<h1>Service temporarily unavailable</h1>
<p>Error code 503</p>
<p>Cloudflare Ray ID: 8f0000000000a1b2</p>
</div>
</body>
</html>
`
