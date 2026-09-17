package client

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// The census closes a gap no amount of reading this package can close.
//
// A request struct that carries a WRONG VALUE is visible in the source. A
// request struct that is MISSING A FIELD ENTIRELY is not: there is nothing
// to read. On a dialect A endpoint every key is transmitted on every call,
// so a field this client does not model is a field the server resets to its
// schema default on every apply — silently, with an HTTP 200. That is how
// watchPaths, buildSecrets, enableSubmodules, isStaticSpa and triggerType
// all came to be overwritten on every `terraform apply` without a single
// test going red.
//
// testdata/endpoint-fields.json is the server's own account of what each
// endpoint accepts, distilled from the OpenAPI document Dokploy serves at
// GET /api/trpc/settings.getOpenApiDocument (the document is real and
// complete for REQUEST bodies; its 200-response schemas are all empty
// objects, which is why this package is still hand-written — see doc.go).
//
// Regenerate it against a rig whenever the pinned Dokploy version moves:
//
//	eval "$(./acceptance/bootstrap.sh)"
//	curl -sS -H "x-api-key: $DOKPLOY_API_KEY" \
//	  "$DOKPLOY_ENDPOINT/api/trpc/settings.getOpenApiDocument" \
//	| python3 -c '
//	import json,sys
//	spec=json.load(sys.stdin)["result"]["data"]["json"]
//	out={}
//	for path,ops in spec["paths"].items():
//	    name=path.strip("/").split("/")[-1]
//	    for op in ops.values():
//	        s=op.get("requestBody",{}).get("content",{}).get("application/json",{}).get("schema",{})
//	        if s.get("properties"):
//	            out[name]={"fields":sorted(s["properties"]),"required":sorted(s.get("required",[]))}
//	json.dump(dict(sorted(out.items())),sys.stdout,indent=1)
//	' > internal/client/testdata/endpoint-fields.json
//
// Snapshot taken against Dokploy v0.30.6 on 2026-09-10. It is byte-identical
// to the v0.30.5 snapshot of 2026-09-04: the v0.30.5...v0.30.6 release changed
// no request body on any endpoint.

// endpointStructs maps a Dokploy write endpoint to the request struct this
// package sends to it. Every endpoint whose absent keys are load-bearing —
// all of dialect A, and dialect B wherever a field must be clearable —
// belongs here. Adding an endpoint to the client without adding it here is
// exactly the gap this table exists to close.
var endpointStructs = map[string]any{
	"application.saveGithubProvider":    SaveGithubProviderRequest{},
	"application.saveGitProvider":       SaveGitProviderRequest{},
	"application.saveDockerProvider":    SaveDockerProviderRequest{},
	"application.saveGitlabProvider":    SaveGitlabProviderRequest{},
	"application.saveBitbucketProvider": SaveBitbucketProviderRequest{},
	"application.saveGiteaProvider":     SaveGiteaProviderRequest{},
	"application.saveBuildType":         SaveBuildTypeRequest{},
	"application.saveEnvironment":       SaveApplicationEnvironmentRequest{},
	"mounts.create":                     CreateMountRequest{},
	"mounts.update":                     UpdateMountRequest{},
	"port.create":                       CreatePortRequest{},
	"port.update":                       UpdatePortRequest{},
	"redirects.create":                  CreateRedirectRequest{},
	"redirects.update":                  UpdateRedirectRequest{},
	"security.create":                   CreateSecurityRequest{},
	"security.update":                   UpdateSecurityRequest{},
	"destination.create":                CreateDestinationRequest{},
	"destination.update":                UpdateDestinationRequest{},
	"schedule.create":                   CreateScheduleRequest{},
	"schedule.update":                   UpdateScheduleRequest{},
	"volumeBackups.create":              CreateVolumeBackupRequest{},
	"volumeBackups.update":              UpdateVolumeBackupRequest{},
	"backup.create":                     CreateBackupRequest{},
	"backup.update":                     UpdateBackupRequest{},
	"compose.create":                    CreateComposeRequest{},
	"compose.update":                    UpdateComposeRequest{},
	"compose.saveEnvironment":           SaveComposeEnvironmentRequest{},
	"libsql.create":                     CreateLibsqlRequest{},
	"libsql.update":                     UpdateLibsqlRequest{},
	"libsql.saveExternalPorts":          saveLibsqlExternalPortsShape{},
	"libsql.saveEnvironment":            saveLibsqlEnvironmentShape{},
	"network.create":                    CreateNetworkRequest{},
	"vaultProvider.create":              CreateVaultProviderRequest{},
	"vaultProvider.update":              UpdateVaultProviderRequest{},
	"vaultProvider.testConnection":      TestVaultConnectionRequest{},
	"sshKey.create":                     CreateSSHKeyRequest{},
	"sshKey.update":                     UpdateSSHKeyRequest{},
	"server.create":                     CreateServerRequest{},
	"server.update":                     UpdateServerRequest{},
	"server.updateBuildsConcurrency":    UpdateBuildsConcurrencyRequest{},
	"tag.create":                        CreateTagRequest{},
	"tag.update":                        UpdateTagRequest{},
	"tag.bulkAssign":                    BulkAssignTagsRequest{},
	"compose.deploy":                    DeployComposeRequest{},
	"certificates.create":               CreateCertificateRequest{},
	"certificates.update":               UpdateCertificateRequest{},
	"ai.create":                         CreateAIRequest{},
	"ai.update":                         UpdateAIRequest{},
	"registry.create":                   CreateRegistryRequest{},
	"registry.update":                   UpdateRegistryRequest{},
	"gitlab.create":                     CreateGitlabRequest{},
	"gitlab.update":                     UpdateGitlabRequest{},
	"bitbucket.create":                  CreateBitbucketRequest{},
	"bitbucket.update":                  UpdateBitbucketRequest{},
	"gitea.create":                      CreateGiteaRequest{},
	"gitea.update":                      UpdateGiteaRequest{},
	"notification.createSlack":          CreateSlackNotificationRequest{},
	"notification.updateSlack":          UpdateSlackNotificationRequest{},
	"notification.createTelegram":       CreateTelegramNotificationRequest{},
	"notification.updateTelegram":       UpdateTelegramNotificationRequest{},
	"notification.createDiscord":        CreateDiscordNotificationRequest{},
	"notification.updateDiscord":        UpdateDiscordNotificationRequest{},
	"notification.createEmail":          CreateEmailNotificationRequest{},
	"notification.updateEmail":          UpdateEmailNotificationRequest{},
	"notification.createResend":         CreateResendNotificationRequest{},
	"notification.updateResend":         UpdateResendNotificationRequest{},
	"notification.createGotify":         CreateGotifyNotificationRequest{},
	"notification.updateGotify":         UpdateGotifyNotificationRequest{},
	"notification.createNtfy":           CreateNtfyNotificationRequest{},
	"notification.updateNtfy":           UpdateNtfyNotificationRequest{},
	"notification.createMattermost":     CreateMattermostNotificationRequest{},
	"notification.updateMattermost":     UpdateMattermostNotificationRequest{},
	"notification.createCustom":         CreateCustomNotificationRequest{},
	"notification.updateCustom":         UpdateCustomNotificationRequest{},
	"notification.createLark":           CreateLarkNotificationRequest{},
	"notification.updateLark":           UpdateLarkNotificationRequest{},
	"notification.createPushover":       CreatePushoverNotificationRequest{},
	"notification.updatePushover":       UpdatePushoverNotificationRequest{},
	"notification.createTeams":          CreateTeamsNotificationRequest{},
	"notification.updateTeams":          UpdateTeamsNotificationRequest{},
	"organization.create":               CreateOrganizationRequest{},
	"organization.update":               UpdateOrganizationRequest{},
	"user.createUserWithCredentials":    CreateUserRequest{},
	"user.assignPermissions":            AssignPermissionsRequest{},
	"user.createApiKey":                 CreateAPIKeyRequest{},

	// Dialect B endpoints (an absent key keeps the stored value). An
	// unmodelled field here is not reset on apply, only unmanageable - but
	// nothing flagged it either, so a Dokploy release could add a field and
	// the provider would never notice (#51, T1). Every field the server
	// accepts is now either on the struct or listed in censusExempt with
	// the reason it stays out.
	"project.create":     CreateProjectRequest{},
	"project.update":     UpdateProjectRequest{},
	"environment.create": CreateEnvironmentRequest{},
	"environment.update": UpdateEnvironmentRequest{},
	"domain.create":      CreateDomainRequest{},
	"domain.update":      UpdateDomainRequest{},
	"application.create": CreateApplicationRequest{},
	"application.update": UpdateApplicationRequest{},
	"postgres.create":    CreatePostgresRequest{},
	"postgres.update":    UpdatePostgresRequest{},
	"mysql.create":       CreateMysqlRequest{},
	"mysql.update":       UpdateMysqlRequest{},
	"mariadb.create":     CreateMariadbRequest{},
	"mariadb.update":     UpdateMariadbRequest{},
	"mongo.create":       CreateMongoRequest{},
	"mongo.update":       UpdateMongoRequest{},
	"redis.create":       CreateRedisRequest{},
	"redis.update":       UpdateRedisRequest{},
}

// inEndpointStructs reports whether a request struct is registered above.
// blind_field_test.go uses it to require that every dialect A request is
// censused against the server's own field list.
func inEndpointStructs(typ reflect.Type) bool {
	for _, v := range endpointStructs {
		if reflect.TypeOf(v) == typ {
			return true
		}
	}
	return false
}

// censusExempt lists endpoint fields this client deliberately does not send,
// each with the reason. An entry here is a decision on the record rather
// than a silent omission — which is the whole point of the census. Never add
// one to quiet a failure you have not understood.
//
// Keep this list as short as you can: an exemption is a field the server
// accepts and this provider silently ignores. Wave-3 task 1 parked five
// application fields here as written-down debt and task 3 modelled all five,
// so the only entries left are ones where NOT sending the field is the
// correct behaviour rather than a gap.
var censusExempt = map[string]map[string]string{
	// mounts.update accepts every parent column plus serviceType, and
	// setting one does NOT clear the others: retargeting through this
	// endpoint leaves the record with two parents (see UpdateMountRequest
	// and doc.go for the live transcript). dokploy_mount marks its parent
	// attributes RequiresReplace instead, so the client never needs to
	// express a retarget and deliberately cannot.
	"mounts.update": {
		"serviceType":   "parent is RequiresReplace; mounts.update corrupts on retarget",
		"applicationId": "parent is RequiresReplace; mounts.update corrupts on retarget",
		"composeId":     "parent is RequiresReplace; mounts.update corrupts on retarget",
		"postgresId":    "parent is RequiresReplace; mounts.update corrupts on retarget",
		"mysqlId":       "parent is RequiresReplace; mounts.update corrupts on retarget",
		"mariadbId":     "parent is RequiresReplace; mounts.update corrupts on retarget",
		"mongoId":       "parent is RequiresReplace; mounts.update corrupts on retarget",
		"redisId":       "parent is RequiresReplace; mounts.update corrupts on retarget",
		"libsqlId":      "parent is RequiresReplace; mounts.update corrupts on retarget",
	},
	// destination.create/update accept serverId, but destination.one and
	// destination.all never return it (verified live, v0.29.13,
	// 2026-07-28). A write-only field cannot round-trip: state would hold a
	// value Read can never confirm, so either the attribute lies or every
	// plan shows a diff. Exposing it needs a read path first.
	"destination.create": {"serverId": "destination.one does not return it; a write-only field cannot round-trip"},
	"destination.update": {"serverId": "destination.one does not return it; a write-only field cannot round-trip"},
	// schedule.update accepts the parent columns but sets one without
	// clearing the others, so a retarget leaves two parents on the record.
	// schedule_type and service_id are RequiresReplace instead.
	"schedule.update": {
		"applicationId":  "parent is RequiresReplace; schedule.update corrupts on retarget",
		"composeId":      "parent is RequiresReplace; schedule.update corrupts on retarget",
		"serverId":       "parent is RequiresReplace; schedule.update corrupts on retarget",
		"appName":        "server-generated; not user configuration",
		"createdAt":      "server-generated; not user configuration",
		"organizationId": "implied by the API key's organization",
	},
	"schedule.create": {
		"appName":        "server-generated; not user configuration",
		"createdAt":      "server-generated; not user configuration",
		"organizationId": "implied by the API key's organization",
		"scheduleId":     "server-generated; the create endpoint assigns it",
	},
	// volumeBackups.update accepts serviceType and every parent column, and
	// retargeting through it leaves the record with two parents -- verified
	// live, see UpdateVolumeBackupRequest. RequiresReplace instead.
	"volumeBackups.update": {
		"serviceType":   "parent is RequiresReplace; volumeBackups.update corrupts on retarget",
		"applicationId": "parent is RequiresReplace; volumeBackups.update corrupts on retarget",
		"composeId":     "parent is RequiresReplace; volumeBackups.update corrupts on retarget",
		"postgresId":    "parent is RequiresReplace; volumeBackups.update corrupts on retarget",
		"mysqlId":       "parent is RequiresReplace; volumeBackups.update corrupts on retarget",
		"mariadbId":     "parent is RequiresReplace; volumeBackups.update corrupts on retarget",
		"mongoId":       "parent is RequiresReplace; volumeBackups.update corrupts on retarget",
		"redisId":       "parent is RequiresReplace; volumeBackups.update corrupts on retarget",
		"libsqlId":      "parent is RequiresReplace; volumeBackups.update corrupts on retarget",
		"appName":       "server-generated; not user configuration",
		"createdAt":     "server-generated; not user configuration",
	},
	"volumeBackups.create": {
		"appName":   "server-generated; not user configuration",
		"createdAt": "server-generated; not user configuration",
	},
	// metadata's schema is `anyOf: [{}, null]` -- genuinely untyped -- and it
	// has read back null on every record observed live. There is no shape to
	// model and no value to preserve, so it is sent as an explicit null.
	// Modelling it needs evidence this provider does not have.
	"backup.create": {
		"metadata": "schema is untyped (anyOf [{}, null]); reads back null on every observed record",
		"userId":   "implied by the API key; the server assigns it",
	},
	// compose.update is dialect B, not A, so an unmodelled field is merely
	// unmanageable here - it is NOT reset on every apply, verified live
	// (v0.29.13, 2026-07-29) by setting thirteen fields away from their
	// defaults and issuing an update carrying only composeId and name. The
	// gitlab, bitbucket and gitea columns are modelled since phase 2.
	"compose.update": {
		"isolatedDeployment":        "deprecated upstream since v0.30.0; service_networks replaces it",
		"isolatedDeploymentsVolume": "deprecated upstream since v0.30.0; service_networks replaces it",

		"appName":       "server-generated; RequiresReplace on the resource, never updated",
		"createdAt":     "server-generated; not user configuration",
		"environmentId": "RequiresReplace on the resource; compose.move is the supported retarget and is not modelled",
		"refreshToken":  "server-generated webhook token; rotating it is an imperative operation",
		"composeStatus": "server-mutable status; a deploy moves it, Terraform must not write it",
		"env":           "set through compose.saveEnvironment, which is the endpoint the Dokploy UI uses",
		"createEnvFile": "set through compose.saveEnvironment, which is the endpoint the Dokploy UI uses - same split as env",
	},
	// compose.create accepts sourceType since v0.30.0, but the resource sets
	// the source through the follow-up compose.update it already issues on
	// every create (Create's doc comment in resources/compose/resource.go).
	// Sending it twice would add a second writer for the same column.
	"compose.create": {
		"sourceType": "source is set by the follow-up compose.update the resource always issues on create",
	},
	// libsql.update is similar to compose.update: the endpoint accepts more
	// fields than this client models. The Swarm fields are not exposed in
	// Terraform (dokploy_application does not expose them either), and
	// externalAdminPort, externalGRPCPort, externalPort are managed through a
	// separate endpoint (libsql.saveExternalPorts, like compose.saveEnvironment
	// handles env).
	"libsql.update": {
		"endpointSpecSwarm":    "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"healthCheckSwarm":     "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"labelsSwarm":          "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"modeSwarm":            "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"networkSwarm":         "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"placementSwarm":       "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"restartPolicySwarm":   "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"rollbackConfigSwarm":  "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"updateConfigSwarm":    "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"stopGracePeriodSwarm": "Docker Swarm orchestration surface; dokploy_application does not expose it either",
		"appName":              "server-generated (suffixed for uniqueness on every create); Computed-only on the resource, so no config path exists to change it and libsql.update never needs to send it",
		"applicationStatus":    "server-mutable status; a deploy moves it, Terraform must not write it",
		"createdAt":            "server-generated; not user configuration",
		"env":                  "set through libsql.saveEnvironment, which is the endpoint the Dokploy UI uses",
		"environmentId":        "RequiresReplace on the resource; libsql.move is the supported retarget and is not modelled",
		"externalAdminPort":    "managed through libsql.saveExternalPorts, not in the primary update endpoint",
		"externalGRPCPort":     "managed through libsql.saveExternalPorts, not in the primary update endpoint",
		"externalPort":         "managed through libsql.saveExternalPorts, not in the primary update endpoint",
	},
	// Phase 2 records (probed 2026-09-05; see doc.go "Phase 2 records").
	"sshKey.update": {
		"lastUsedAt": "server-managed; the server rewrites it whenever a remote server uses the key",
	},
	"certificates.create": {
		"certificateId":   "server-generated; the create endpoint assigns it",
		"certificatePath": "server-generated; not user configuration",
	},
	"ai.update": {
		"createdAt": "server-generated; not user configuration",
	},
	// registry.create/update accept serverId, but registry.one and
	// registry.all never return it (probed live, v0.30.5, 2026-09-05). A
	// write-only field cannot round-trip, the same reasoning as
	// destination.serverId.
	"registry.create": {
		"serverId": "registry.one does not return it; a write-only field cannot round-trip",
	},
	// tag.update echoes the read-only columns of the record (#63, v1.6.0).
	"tag.update": {
		"createdAt":      "server-generated; not user configuration",
		"organizationId": "implied by the API key's organization",
	},
	"registry.update": {
		"serverId":       "registry.one does not return it; a write-only field cannot round-trip",
		"createdAt":      "server-generated; not user configuration",
		"organizationId": "implied by the API key's organization",
	},
	// The git provider records. The OAuth handshake fields (accessToken,
	// refreshToken, expiresAt, lastAuthenticatedAt, giteaUsername) are
	// written by the browser flow, never by configuration.
	"gitlab.create": {
		"gitProviderId": "server-generated; the create endpoint assigns it",
	},
	"bitbucket.create": {
		"bitbucketId":   "server-generated; the create endpoint assigns it",
		"gitProviderId": "server-generated; the create endpoint assigns it",
	},
	"bitbucket.update": {
		"organizationId": "implied by the API key's organization",
	},
	"gitea.create": {
		"gitProviderId":       "server-generated; the create endpoint assigns it",
		"giteaId":             "server-generated; the create endpoint assigns it",
		"accessToken":         "written by the OAuth handshake in the browser, never by configuration",
		"refreshToken":        "written by the OAuth handshake in the browser, never by configuration",
		"expiresAt":           "written by the OAuth handshake in the browser, never by configuration",
		"lastAuthenticatedAt": "written by the OAuth handshake in the browser, never by configuration",
		"giteaUsername":       "written by the OAuth handshake in the browser, never by configuration",
		"organizationName":    "gitea.one does not return it; a write-only field cannot round-trip",
	},
	"gitea.update": {
		"accessToken":         "written by the OAuth handshake in the browser, never by configuration",
		"refreshToken":        "written by the OAuth handshake in the browser, never by configuration",
		"expiresAt":           "written by the OAuth handshake in the browser, never by configuration",
		"lastAuthenticatedAt": "written by the OAuth handshake in the browser, never by configuration",
		"giteaUsername":       "written by the OAuth handshake in the browser, never by configuration",
		"organizationName":    "gitea.one does not return it; a write-only field cannot round-trip",
	},
	// Every notification.update<Type> accepts organizationId.
	"notification.updateSlack":      {"organizationId": "implied by the API key's organization"},
	"notification.updateTelegram":   {"organizationId": "implied by the API key's organization"},
	"notification.updateDiscord":    {"organizationId": "implied by the API key's organization"},
	"notification.updateEmail":      {"organizationId": "implied by the API key's organization"},
	"notification.updateResend":     {"organizationId": "implied by the API key's organization"},
	"notification.updateGotify":     {"organizationId": "implied by the API key's organization"},
	"notification.updateNtfy":       {"organizationId": "implied by the API key's organization"},
	"notification.updateMattermost": {"organizationId": "implied by the API key's organization"},
	"notification.updateCustom":     {"organizationId": "implied by the API key's organization"},
	"notification.updateLark":       {"organizationId": "implied by the API key's organization"},
	"notification.updatePushover":   {"organizationId": "implied by the API key's organization"},
	"notification.updateTeams":      {"organizationId": "implied by the API key's organization"},
	// Dialect B endpoints (#51, T1). The engine .update endpoints share one
	// list, built by databaseUpdateExemptions below.
	"postgres.update": databaseUpdateExemptions("postgres", "databaseName", "databaseUser"),
	"mysql.update":    databaseUpdateExemptions("mysql", "databaseName", "databaseUser"),
	"mariadb.update":  databaseUpdateExemptions("mariadb", "databaseName", "databaseUser"),
	"mongo.update":    databaseUpdateExemptions("mongo", "databaseUser"),
	"redis.update":    databaseUpdateExemptions("redis"),
	"project.update": {
		"createdAt":      "server-generated; not user configuration",
		"organizationId": "implied by the API key's organization",
	},
	"compose.deploy": {
		"title":       "title of the deployment log entry; not configuration",
		"description": "description of the deployment log entry; not configuration",
	},
	"environment.update": {
		"projectId": "RequiresReplace on the resource; moving an environment between projects is not modelled",
	},
	"domain.create": {
		"middlewares":         "Traefik middlewares; the Traefik surface is out of scope like the Traefik files (settings router)",
		"previewDeploymentId": "the parent of a preview deployment domain; preview deployment records are imperative (previewDeployment.* endpoints), not managed",
	},
	"domain.update": {
		"middlewares": "Traefik middlewares; the Traefik surface is out of scope like the Traefik files (settings router)",
	},
	// application.create accepts sourceType since v0.30.0; the resource sets
	// the source through the save*Provider call it always issues after
	// create, the same split as compose.create.
	"application.create": {
		"sourceType": "source is set by the application.save*Provider call the resource always issues on create",
	},
	// application.update accepts nearly every column. The resource writes
	// the source, build and env groups through the dialect A save* endpoints
	// (the ones the Dokploy UI uses), so they stay off this struct on
	// purpose; the rest are server-managed, RequiresReplace, or unmodelled
	// feature groups with an issue.
	"application.update": mergeExemptions(
		map[string]string{
			"appName":           "server-generated; RequiresReplace on the resource, never updated",
			"applicationStatus": "server-mutable status; a deploy moves it, Terraform must not write it",
			"createdAt":         "server-generated; not user configuration",
			"environmentId":     "RequiresReplace on the resource; application.move is the supported retarget and is not modelled",
			"refreshToken":      "server-generated webhook token; rotating it is an imperative operation",
			"enabled":           "start/stop state; a desired_state attribute needs its own design (gap plan D1)",
			"icon":              "display icon; not modelled on dokploy_application (dokploy_compose models it)",
		},
		sameReason("set through the application.save*Provider endpoints (dialect A), which the Dokploy UI uses",
			"sourceType", "triggerType", "watchPaths", "enableSubmodules",
			"repository", "owner", "branch", "buildPath", "githubId",
			"gitlabId", "gitlabProjectId", "gitlabRepository", "gitlabOwner", "gitlabBranch", "gitlabBuildPath", "gitlabPathNamespace",
			"bitbucketId", "bitbucketRepository", "bitbucketOwner", "bitbucketBranch", "bitbucketBuildPath", "bitbucketRepositorySlug",
			"giteaId", "giteaRepository", "giteaOwner", "giteaBranch", "giteaBuildPath",
			"customGitUrl", "customGitBranch", "customGitBuildPath", "customGitSSHKeyId",
			"dockerImage", "username", "password", "registryUrl",
		),
		sameReason("set through application.saveBuildType (dialect A), which the Dokploy UI uses",
			"buildType", "dockerfile", "dockerContextPath", "dockerBuildStage", "publishDirectory",
			"herokuVersion", "railpackVersion", "isStaticSpa",
		),
		sameReason("set through application.saveEnvironment (dialect A), which the Dokploy UI uses",
			"env", "buildArgs", "buildSecrets", "createEnvFile",
		),
		swarmExemptions(),
	),
	// better-auth's refill quota fields: not exposed in the Dokploy UI, and
	// the resource models the rate limit through rateLimitMax and the window.
	"user.createApiKey": {
		"remaining":      "better-auth refill quota; not exposed in the Dokploy UI and not modelled",
		"refillAmount":   "better-auth refill quota; not exposed in the Dokploy UI and not modelled",
		"refillInterval": "better-auth refill quota; not exposed in the Dokploy UI and not modelled",
	},
}

// swarmExemptions is the Docker Swarm orchestration surface every service
// update endpoint accepts (eleven JSON columns). None of it is modelled yet;
// the gap plan tracks it as a `swarm` block (item A9).
func swarmExemptions() map[string]string {
	return sameReason("Docker Swarm orchestration surface; not modelled yet (gap plan A9)",
		"endpointSpecSwarm", "healthCheckSwarm", "labelsSwarm", "modeSwarm", "networkSwarm",
		"placementSwarm", "restartPolicySwarm", "rollbackConfigSwarm", "stopGracePeriodSwarm",
		"ulimitsSwarm", "updateConfigSwarm",
	)
}

// databaseUpdateExemptions is the shared list for the five engine .update
// endpoints (#51): the server-managed columns, the RequiresReplace
// credentials named per engine, the two columns the resource writes through
// the engine's own save* endpoints, and the swarm surface.
func databaseUpdateExemptions(engine string, replaceCredentials ...string) map[string]string {
	out := mergeExemptions(
		map[string]string{
			"appName":           "server-generated (suffixed for uniqueness on every create); Computed-only on the resource, never updated",
			"applicationStatus": "server-mutable status; a deploy moves it, Terraform must not write it",
			"createdAt":         "server-generated; not user configuration",
			"environmentId":     "RequiresReplace on the resource; " + engine + ".move is the supported retarget and is not modelled",
			"env":               "set through " + engine + ".saveEnvironment, which is the endpoint the Dokploy UI uses",
			"externalPort":      "set through " + engine + ".saveExternalPort, which is the endpoint the Dokploy UI uses",
		},
		swarmExemptions(),
	)
	for _, c := range replaceCredentials {
		out[c] = "RequiresReplace on the resource: an in-place change would not migrate the running database"
	}
	return out
}

// sameReason builds an exemption map with one reason for every field.
func sameReason(reason string, fields ...string) map[string]string {
	out := make(map[string]string, len(fields))
	for _, f := range fields {
		out[f] = reason
	}
	return out
}

// mergeExemptions joins exemption maps; a field listed twice is a mistake.
func mergeExemptions(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			if _, dup := out[k]; dup {
				panic("censusExempt: field " + k + " listed twice")
			}
			out[k] = v
		}
	}
	return out
}

type endpointFields struct {
	Fields   []string `json:"fields"`
	Required []string `json:"required"`
}

func loadEndpointFields(t *testing.T) map[string]endpointFields {
	t.Helper()
	raw, err := os.ReadFile("testdata/endpoint-fields.json")
	if err != nil {
		t.Fatalf("read endpoint-fields snapshot: %v", err)
	}
	var snapshot map[string]endpointFields
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatalf("decode endpoint-fields snapshot: %v", err)
	}
	return snapshot
}

func TestEndpointFieldCensus(t *testing.T) {
	snapshot := loadEndpointFields(t)

	for endpoint, reqStruct := range endpointStructs {
		entry, ok := snapshot[endpoint]
		if !ok {
			t.Errorf("%s: absent from testdata/endpoint-fields.json — regenerate the snapshot", endpoint)
			continue
		}
		typ := reflect.TypeOf(reqStruct)
		have := make(map[string]bool, typ.NumField())
		for _, f := range jsonFields(typ) {
			if name := jsonName(f); name != "" {
				have[name] = true
			}
		}
		for _, field := range entry.Fields {
			if have[field] || censusExempt[endpoint][field] != "" {
				continue
			}
			t.Errorf(
				"%s: the server accepts %q but %s has no such field and no censusExempt entry.\n"+
					"On a dialect A endpoint every key is sent on every call, so an unmodelled "+
					"field is reset to its schema default on every apply, silently, with an HTTP 200. "+
					"On a dialect B endpoint it is unmanageable from Terraform and nothing else reports it. "+
					"Either model it as a schema attribute or record why not in censusExempt.",
				endpoint, field, typ.Name())
		}
	}
}

// TestCensusExemptionsAreLive keeps the exemption list honest: an entry for a
// field the struct has since grown, or for an endpoint/field the server no
// longer accepts, is stale and must go. Without this, exemptions accumulate
// into a second, invisible blind list.
func TestCensusExemptionsAreLive(t *testing.T) {
	snapshot := loadEndpointFields(t)

	for endpoint, fields := range censusExempt {
		entry, ok := snapshot[endpoint]
		if !ok {
			t.Errorf("censusExempt[%q]: endpoint is not in the snapshot", endpoint)
			continue
		}
		accepted := make(map[string]bool, len(entry.Fields))
		for _, f := range entry.Fields {
			accepted[f] = true
		}
		reqStruct, ok := endpointStructs[endpoint]
		if !ok {
			t.Errorf("censusExempt[%q]: endpoint has no entry in endpointStructs", endpoint)
			continue
		}
		typ := reflect.TypeOf(reqStruct)
		for field, reason := range fields {
			if reason == "" {
				t.Errorf("censusExempt[%q][%q]: exemptions must carry a reason", endpoint, field)
			}
			if !accepted[field] {
				t.Errorf("censusExempt[%q][%q]: the server no longer accepts this field — drop the exemption", endpoint, field)
			}
			if _, has := fieldByJSONName(typ, field); has {
				t.Errorf("censusExempt[%q][%q]: %s now carries this field — drop the exemption", endpoint, field, typ.Name())
			}
		}
	}
}
