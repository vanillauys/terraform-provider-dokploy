package client

// The three blocks below are the application.update columns that
// dokploy_application models since v1.4.0 (#52): preview deployments,
// rollback, and the build settings that live on application.update rather
// than on saveBuildType. Each is embedded (anonymously, so encoding/json
// flattens it) in Application and in UpdateApplicationRequest, the
// ServiceResources pattern.
//
// Probed live on v0.30.6 (2026-09-16, doc.go "v1.4.0 records"):
// application.one returns every field; application.update is dialect B for
// all of them (an absent key keeps the stored value). An explicit null is
// accepted on every column except previewCertificateType (an enum:
// letsencrypt | none | custom) and stores null, and previewHttps null
// reads back false. The resource always sends a concrete value for the
// bools, the numbers, the enum and previewPath, so the bare Go types below
// never have to express a null on the wire.

// ApplicationPreview is the preview-deployment block of application.one.
// The fresh-record values are isPreviewDeploymentsActive false,
// previewCertificateType "none", previewHttps false, previewLimit 3,
// previewPath "/", previewPort 3000, and
// previewRequireCollaboratorPermissions true; the rest read back null.
// previewLabels reads back null until set and [] after an explicit [].
type ApplicationPreview struct {
	IsPreviewDeploymentsActive            bool     `json:"isPreviewDeploymentsActive"`
	PreviewEnv                            *string  `json:"previewEnv"`
	PreviewBuildArgs                      *string  `json:"previewBuildArgs"`
	PreviewBuildSecrets                   *string  `json:"previewBuildSecrets"`
	PreviewCertificateType                string   `json:"previewCertificateType"`
	PreviewCustomCertResolver             *string  `json:"previewCustomCertResolver"`
	PreviewHTTPS                          bool     `json:"previewHttps"`
	PreviewLabels                         []string `json:"previewLabels"`
	PreviewLimit                          int64    `json:"previewLimit"`
	PreviewPath                           string   `json:"previewPath"`
	PreviewPort                           int64    `json:"previewPort"`
	PreviewRequireCollaboratorPermissions bool     `json:"previewRequireCollaboratorPermissions"`
	PreviewWildcard                       *string  `json:"previewWildcard"`
}

// ApplicationPreviewUpdate is the update-side twin of ApplicationPreview.
// PreviewLabels is a pointer so that a nil clears the column with an
// explicit null, the Args pattern; every other pointer is nil-to-null too.
type ApplicationPreviewUpdate struct {
	IsPreviewDeploymentsActive            bool      `json:"isPreviewDeploymentsActive"`
	PreviewEnv                            *string   `json:"previewEnv"`
	PreviewBuildArgs                      *string   `json:"previewBuildArgs"`
	PreviewBuildSecrets                   *string   `json:"previewBuildSecrets"`
	PreviewCertificateType                string    `json:"previewCertificateType"`
	PreviewCustomCertResolver             *string   `json:"previewCustomCertResolver"`
	PreviewHTTPS                          bool      `json:"previewHttps"`
	PreviewLabels                         *[]string `json:"previewLabels"`
	PreviewLimit                          int64     `json:"previewLimit"`
	PreviewPath                           string    `json:"previewPath"`
	PreviewPort                           int64     `json:"previewPort"`
	PreviewRequireCollaboratorPermissions bool      `json:"previewRequireCollaboratorPermissions"`
	PreviewWildcard                       *string   `json:"previewWildcard"`
}

// ApplicationRollback is the rollback block, the same shape on read and
// update. rollbackRegistryId must name an existing registry: an unknown id
// fails at the database layer with an HTTP 500 "Failed query".
type ApplicationRollback struct {
	RollbackActive     bool    `json:"rollbackActive"`
	RollbackRegistryID *string `json:"rollbackRegistryId"`
}

// ApplicationBuildSettings holds the build columns that application.update
// carries, the same shape on read and update. buildServerId must name a
// server the API key can reach: an unknown id is an HTTP 401 "You are not
// authorized to access this build server"; an unknown buildRegistryId is
// an HTTP 500 "Failed query", like rollbackRegistryId.
type ApplicationBuildSettings struct {
	BuildServerID   *string `json:"buildServerId"`
	BuildRegistryID *string `json:"buildRegistryId"`
	CleanCache      bool    `json:"cleanCache"`
	DropBuildPath   *string `json:"dropBuildPath"`
}
