package github

// Release is the subset of a GitHub release the mothership consumes, decoded
// from the /repos/{owner}/{repo}/releases endpoints. Unknown fields in the
// API response are ignored, so a new field upstream does not break parsing.
type Release struct {
	// ID is GitHub's identifier for the release.
	ID int64 `json:"id"`

	// TagName is the git tag the release points at, e.g. "v1.9.0".
	TagName string `json:"tag_name"`

	// Name is the release title as published. It may differ from TagName and
	// may be empty for releases without a title.
	Name string `json:"name"`

	// Draft is true while the release is unpublished and visible only to
	// repository collaborators.
	Draft bool `json:"draft"`

	// Prerelease is true for releases GitHub marks as pre-release.
	Prerelease bool `json:"prerelease"`

	// Assets are the downloadable artifacts attached to the release.
	Assets []ReleaseAsset `json:"assets"`

	// CreatedAt and PublishedAt are RFC3339 timestamps.
	CreatedAt   string `json:"created_at"`
	PublishedAt string `json:"published_at"`

	// HTMLURL is the release's page on github.com.
	HTMLURL string `json:"html_url"`
}

// ReleaseAsset is one downloadable artifact attached to a release — for the
// Kaniko feed, the image-build tools the mothership tracks.
type ReleaseAsset struct {
	// ID is GitHub's identifier for the asset.
	ID int64 `json:"id"`

	// Name is the asset's filename, e.g. "releases_1.9.0.yaml".
	Name string `json:"name"`

	// BrowserDownloadURL is where the asset can be downloaded from.
	BrowserDownloadURL string `json:"browser_download_url"`

	// Size is the asset size in bytes.
	Size int64 `json:"size"`

	// ContentType is the MIME type GitHub recorded for the asset.
	ContentType string `json:"content_type"`
}
