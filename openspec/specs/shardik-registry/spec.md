# Shardik Registry Specification

> *"Shardik the Bear guards the boundary between worlds -- the portal at his Beam's endpoint."*

## Purpose

Shardik is the registry client: OCI Distribution Spec v1.1.0 compliant communication with container registries. Includes Sigul (authentication), Thinny (mirror/proxy), and Horn of Eld (retry/circuit breaker).

---

## Requirements

---

### Requirement: OCI Distribution Spec v1.1.0 Pull Operations

Shardik MUST implement all required pull operations defined by the OCI Distribution Specification v1.1.0. These are the mandatory endpoints that every conformant registry exposes.

#### Scenario: Pull manifest by tag

GIVEN a registry hosts repository "library/nginx" with tag "latest"
WHEN Shardik sends `GET /v2/library/nginx/manifests/latest`
THEN the registry MUST respond with the manifest content
AND Shardik MUST return the manifest bytes and the digest from the `Docker-Content-Digest` response header

#### Scenario: Pull manifest by digest

GIVEN a registry hosts a manifest at digest "sha256:abc123..."
WHEN Shardik sends `GET /v2/library/nginx/manifests/sha256:abc123...`
THEN the registry MUST respond with the manifest content
AND Shardik MUST verify the response digest matches the requested digest

#### Scenario: Check manifest existence

GIVEN a registry hosts repository "library/nginx" with tag "latest"
WHEN Shardik sends `HEAD /v2/library/nginx/manifests/latest`
THEN the registry MUST respond with status 200 and the manifest digest in headers
AND Shardik MUST return the digest and content length without downloading the body

#### Scenario: Pull blob by digest

GIVEN a registry hosts a blob at digest "sha256:def456..."
WHEN Shardik sends `GET /v2/library/nginx/blobs/sha256:def456...`
THEN the registry MUST respond with the blob content
AND Shardik MUST return a reader that produces the blob bytes

#### Scenario: Check blob existence

GIVEN a registry hosts a blob at digest "sha256:def456..."
WHEN Shardik sends `HEAD /v2/library/nginx/blobs/sha256:def456...`
THEN the registry MUST respond with status 200
AND Shardik MUST return true with the content length

#### Scenario: Manifest not found returns error

GIVEN a registry does not host tag "nonexistent" in repository "library/nginx"
WHEN Shardik sends `GET /v2/library/nginx/manifests/nonexistent`
THEN the registry MUST respond with status 404
AND Shardik MUST return a not-found error with the requested reference

#### Scenario: Blob not found returns error

GIVEN a registry does not host blob "sha256:000..."
WHEN Shardik sends `HEAD /v2/library/nginx/blobs/sha256:000...`
THEN the registry MUST respond with status 404
AND Shardik MUST return false from the existence check

---

### Requirement: OCI Distribution Spec v1.1.0 Push Operations

Shardik MUST implement push operations for manifests and blobs as defined by the OCI Distribution Specification v1.1.0.

#### Scenario: Push manifest by tag

GIVEN valid credentials for a registry
WHEN Shardik sends `PUT /v2/myapp/manifests/v1.0` with manifest content
THEN the registry MUST accept the manifest
AND the registry MUST respond with status 201 and the manifest digest in the `Docker-Content-Digest` header

#### Scenario: Push manifest by digest

GIVEN valid credentials for a registry
WHEN Shardik sends `PUT /v2/myapp/manifests/sha256:abc123...` with manifest content
THEN the registry MUST accept the manifest
AND the digest in the URL MUST match the content digest

#### Scenario: Monolithic blob upload

GIVEN valid credentials for a registry
WHEN Shardik initiates a blob upload via `POST /v2/myapp/blobs/uploads/`
AND the registry responds with status 202 and a Location header
AND Shardik sends `PUT <location>?digest=sha256:abc...` with the complete blob content
THEN the registry MUST accept the blob
AND the registry MUST respond with status 201

#### Scenario: Chunked blob upload

GIVEN valid credentials for a registry
AND a blob that is uploaded in chunks
WHEN Shardik initiates a blob upload via `POST /v2/myapp/blobs/uploads/`
AND Shardik sends one or more `PATCH <location>` requests with Content-Range headers
AND Shardik finalizes with `PUT <location>?digest=sha256:abc...`
THEN the registry MUST accept the complete blob
AND the final digest MUST match the complete blob content

#### Scenario: Cross-repository blob mount

GIVEN blob "sha256:abc..." exists in repository "source/repo" on the registry
WHEN Shardik sends `POST /v2/target/repo/blobs/uploads/?mount=sha256:abc...&from=source/repo`
THEN the registry MUST respond with status 201 if the mount succeeds
AND the blob MUST be accessible in "target/repo" without re-uploading

#### Scenario: Cross-repository blob mount falls back to upload

GIVEN blob "sha256:abc..." does not exist in the "from" repository or mount is unsupported
WHEN Shardik sends a cross-repo mount request
AND the registry responds with status 202 (mount not performed, upload initiated)
THEN Shardik MUST fall back to a regular blob upload using the returned Location

#### Scenario: Delete manifest

GIVEN valid credentials for a registry
WHEN Shardik sends `DELETE /v2/myapp/manifests/sha256:abc123...`
THEN the registry MUST respond with status 202
AND the manifest MUST no longer be retrievable

#### Scenario: Delete blob

GIVEN valid credentials for a registry
WHEN Shardik sends `DELETE /v2/myapp/blobs/sha256:abc123...`
THEN the registry MUST respond with status 202
AND the blob MUST no longer be retrievable

---

### Requirement: Content Discovery Operations

Shardik MUST implement content discovery operations defined by the OCI Distribution Specification v1.1.0, including tag listing and the Referrers API.

#### Scenario: List repository tags

GIVEN a registry hosts repository "library/nginx" with tags ["latest", "1.25", "1.24"]
WHEN Shardik sends `GET /v2/library/nginx/tags/list`
THEN the response MUST include all three tags
AND the response MUST conform to the OCI tag list schema

#### Scenario: List tags with pagination via Link header

GIVEN a registry hosts a repository with more tags than fit in a single response
WHEN Shardik sends `GET /v2/<name>/tags/list?n=10`
AND the response includes a `Link` header for the next page
THEN Shardik MUST follow pagination links to retrieve all tags
AND the combined result MUST contain all tags

#### Scenario: Referrers API returns associated artifacts

GIVEN a manifest with digest "sha256:abc..." has referrers (signatures, SBOMs)
WHEN Shardik sends `GET /v2/myapp/referrers/sha256:abc...`
THEN the response MUST be an OCI Image Index containing the referrer manifests
AND each entry MUST include the `artifactType` field

#### Scenario: Referrers API with artifact type filter

GIVEN a manifest has referrers of multiple artifact types
WHEN Shardik sends `GET /v2/myapp/referrers/sha256:abc...?artifactType=application/spdx+json`
THEN the response MUST include only referrers matching the specified artifact type

#### Scenario: Referrers API not supported by registry

GIVEN a registry that does not support the Referrers API
WHEN Shardik sends `GET /v2/myapp/referrers/sha256:abc...`
AND the registry responds with status 404
THEN Shardik SHOULD fall back to the referrers tag schema (querying the tag `sha256-<digest>`)

---

### Requirement: Sigul (Authentication)

Shardik MUST implement the Sigul credential resolution chain to authenticate with registries. Credentials MUST be resolved in a strict priority order. Shardik MUST support the Docker V2 token authentication flow.

#### Scenario: Credential resolution priority order

GIVEN credentials are available at multiple levels
WHEN Shardik resolves credentials for a registry
THEN it MUST check CLI flags (`--username`/`--password`) first (priority 1)
AND if not found, it MUST check environment variable `$MAESTRO_REGISTRY_TOKEN` (priority 2)
AND if not found, it MUST check Maestro auth file `~/.config/maestro/auth.json` (priority 3)
AND if not found, it MUST check Docker config file `~/.docker/config.json` (priority 4)
AND if not found, it MUST check credential helpers (`credHelpers` / `credsStore` entries) (priority 5)
AND if no source provides credentials, it MUST fall back to anonymous access (priority 6)
AND the first source that provides credentials MUST be used
AND subsequent sources MUST NOT be consulted

#### Scenario: CLI flags take highest priority

GIVEN `--username=admin` and `--password=secret` are provided as CLI flags
AND different credentials exist in `auth.json`
WHEN Shardik authenticates to the registry
THEN it MUST use "admin"/"secret" from the CLI flags

#### Scenario: Environment variable overrides file-based credentials

GIVEN `$MAESTRO_REGISTRY_TOKEN` is set to "env-token-123"
AND credentials exist in `auth.json` and `~/.docker/config.json`
AND no CLI flags are provided
WHEN Shardik authenticates to the registry
THEN it MUST use the environment variable token

#### Scenario: Maestro auth.json used before Docker config

GIVEN credentials for "registry.example.com" exist in both `auth.json` and `~/.docker/config.json`
AND no CLI flags or environment variables are set
WHEN Shardik authenticates to "registry.example.com"
THEN it MUST use the credentials from `auth.json`

#### Scenario: Docker config.json fallback

GIVEN no Maestro-specific credentials exist
AND `~/.docker/config.json` contains credentials for "docker.io"
WHEN Shardik authenticates to "docker.io"
THEN it MUST use the credentials from Docker's config file

#### Scenario: Credential helper invocation

GIVEN `~/.docker/config.json` has `credHelpers` mapping "gcr.io" to "gcloud"
AND no higher-priority credential sources provide credentials
WHEN Shardik authenticates to "gcr.io"
THEN it MUST invoke the credential helper binary `docker-credential-gcloud`
AND it MUST pass the registry hostname on stdin
AND it MUST parse the JSON response for username and secret

#### Scenario: Anonymous access when no credentials available

GIVEN no credential sources provide credentials for the target registry
WHEN Shardik attempts to access the registry
THEN it MUST proceed with anonymous access (no Authorization header)
AND if the registry allows anonymous access, the request MUST succeed

#### Scenario: Docker V2 token authentication flow

GIVEN a registry returns `401 Unauthorized` with `WWW-Authenticate: Bearer realm="https://auth.example.com/token",service="registry",scope="repository:myapp:pull"`
WHEN Shardik receives this challenge
THEN it MUST extract the realm, service, and scope parameters
AND it MUST request a token from the realm endpoint with the resolved credentials
AND it MUST retry the original request with `Authorization: Bearer <token>`

#### Scenario: Token refresh on expiration

GIVEN a previously obtained bearer token has expired
WHEN Shardik sends a request and receives 401
THEN it MUST re-execute the token authentication flow
AND the request MUST be retried with the new token

#### Scenario: Authentication failure produces clear error

GIVEN invalid credentials are provided
WHEN Shardik attempts the token exchange
AND the auth endpoint returns 401 or 403
THEN the system MUST return an error indicating authentication failed
AND the error MUST include the registry hostname

---

### Requirement: Sigul Credential Storage

Shardik MUST store credentials securely in the Maestro auth file. The auth file MUST have restrictive file permissions to prevent unauthorized access.

#### Scenario: Auth file created with correct permissions

GIVEN no auth file exists
WHEN credentials are saved for the first time
THEN the auth file MUST be created at `~/.config/maestro/auth.json`
AND the file permissions MUST be set to 0600 (owner read/write only)

#### Scenario: Auth file permissions verified on read

GIVEN the auth file exists with permissions more permissive than 0600
WHEN Shardik reads credentials from the auth file
THEN the system SHOULD log a warning about insecure file permissions

#### Scenario: Credentials stored as base64-encoded pairs

GIVEN a user logs in with username "user" and password "pass" for "registry.example.com"
WHEN the credentials are saved
THEN the auth file MUST contain an entry keyed by registry hostname
AND the credentials MUST be stored in the `auths` section as a base64-encoded `username:password` pair

#### Scenario: Auth file directory created if missing

GIVEN the configuration directory `~/.config/maestro/` does not exist
WHEN credentials are saved
THEN the directory MUST be created with permissions 0700
AND the auth file MUST be created within it

---

### Requirement: Login Operation

Shardik MUST support an interactive login operation that authenticates against a registry and stores the credentials.

#### Scenario: Successful login stores credentials

GIVEN a registry at "registry.example.com" that accepts username "user" and password "pass"
WHEN a login operation is invoked with these credentials
THEN Shardik MUST validate the credentials by performing a `GET /v2/` check
AND the registry MUST respond with 200
AND the credentials MUST be saved to the auth file

#### Scenario: Login with incorrect credentials fails

GIVEN a registry at "registry.example.com"
WHEN a login operation is invoked with incorrect credentials
AND the registry returns 401 after the token flow
THEN the system MUST return an authentication error
AND no credentials MUST be saved to the auth file

#### Scenario: Login replaces existing credentials

GIVEN credentials for "registry.example.com" already exist in the auth file
WHEN a login operation is invoked with new credentials for the same registry
THEN the auth file MUST be updated with the new credentials
AND the old credentials MUST be replaced

---

### Requirement: Logout Operation

Shardik MUST support a logout operation that removes stored credentials for a specific registry.

#### Scenario: Successful logout removes credentials

GIVEN credentials for "registry.example.com" exist in the auth file
WHEN a logout operation is invoked for "registry.example.com"
THEN the credentials MUST be removed from the auth file
AND subsequent authentication attempts for that registry MUST fall through to lower-priority sources

#### Scenario: Logout for a registry with no stored credentials

GIVEN no credentials for "registry.example.com" exist in the auth file
WHEN a logout operation is invoked for "registry.example.com"
THEN the system MUST return an error or warning indicating no credentials were found

---

### Requirement: Thinny (Mirror/Proxy Resolution)

Shardik MUST support registry mirror configuration (Thinny). When mirrors are configured for a registry, Shardik MUST attempt mirrors in order before falling back to the primary registry.

#### Scenario: Mirror used when configured

GIVEN the configuration maps "docker.io" to mirrors `["https://mirror.internal.com", "https://registry-1.docker.io"]`
WHEN Shardik resolves the endpoint for "docker.io"
THEN it MUST attempt `https://mirror.internal.com` first

#### Scenario: Fallback to next mirror on failure

GIVEN the configuration maps "docker.io" to mirrors `["https://mirror1.example.com", "https://mirror2.example.com", "https://registry-1.docker.io"]`
AND "mirror1.example.com" is unreachable
WHEN Shardik attempts to pull a manifest
THEN it MUST attempt "mirror1.example.com" first
AND when that fails, it MUST attempt "mirror2.example.com"
AND if that also fails, it MUST attempt "registry-1.docker.io"

#### Scenario: Fallback to primary registry when all mirrors fail

GIVEN mirrors are configured but all are unreachable
AND the primary registry endpoint is available
WHEN Shardik attempts a registry operation
THEN it MUST fall back to the primary registry
AND the operation MUST succeed

#### Scenario: Skip TLS verification for configured mirrors

GIVEN a mirror is configured with `skip_verify = true`
WHEN Shardik connects to this mirror
THEN TLS certificate verification MUST be skipped for this specific mirror
AND TLS verification MUST still be enforced for all other endpoints

#### Scenario: No mirrors configured uses primary endpoint

GIVEN no mirror configuration exists for "ghcr.io"
WHEN Shardik resolves the endpoint for "ghcr.io"
THEN it MUST connect directly to the primary registry endpoint

#### Scenario: Mirror configuration per registry

GIVEN mirrors are configured for "docker.io" but not for "ghcr.io"
WHEN Shardik resolves endpoints
THEN "docker.io" requests MUST use the configured mirror chain
AND "ghcr.io" requests MUST go directly to the primary endpoint

---

### Requirement: Horn of Eld (Retry with Exponential Backoff)

Shardik MUST implement retry logic with exponential backoff for transient failures. The retry policy MUST use doubling delays with configurable limits.

#### Scenario: Retry on transient network error

GIVEN a registry request fails with a transient network error (connection reset, timeout)
WHEN the Horn of Eld retry policy is applied
THEN the request MUST be retried
AND the first retry MUST occur after approximately 100ms
AND the second retry MUST occur after approximately 200ms
AND the third retry MUST occur after approximately 400ms

#### Scenario: Exponential backoff doubles delay each attempt

GIVEN a request continues to fail on each retry
WHEN the retry policy computes delays
THEN each subsequent delay MUST be approximately double the previous delay
AND the sequence MUST follow 100ms, 200ms, 400ms, 800ms, 1600ms for five retries

#### Scenario: Maximum of 5 retry attempts

GIVEN a request fails on every attempt
WHEN the retry policy is exhausted
THEN exactly 5 retry attempts MUST be made (6 total attempts including the original)
AND after the 5th retry fails, the system MUST return the last error

#### Scenario: Maximum delay capped at 30 seconds

GIVEN the computed exponential backoff would exceed 30 seconds
WHEN the next retry delay is calculated
THEN the delay MUST be capped at 30 seconds

#### Scenario: Jitter applied to retry delays

GIVEN a retry delay is computed
WHEN the actual wait time is determined
THEN random jitter SHOULD be applied to prevent thundering herd effects
AND the actual delay MUST be within a reasonable range of the computed delay

#### Scenario: Successful response stops retry

GIVEN a request fails on the first attempt
AND succeeds on the second attempt
WHEN the retry policy is applied
THEN the successful response MUST be returned immediately
AND no further retries MUST occur

#### Scenario: Non-retryable errors are not retried

GIVEN a registry returns 400 Bad Request or 403 Forbidden
WHEN the retry policy evaluates the response
THEN the error MUST be returned immediately without retry
AND the response status code MUST be preserved in the error

#### Scenario: 429 Too Many Requests triggers retry with Retry-After

GIVEN a registry returns 429 with `Retry-After: 5`
WHEN the retry policy evaluates the response
THEN Shardik MUST wait at least 5 seconds before the next attempt
AND the Retry-After value MUST take precedence over the computed backoff

#### Scenario: 5xx server errors are retried

GIVEN a registry returns 500, 502, 503, or 504
WHEN the retry policy evaluates the response
THEN the request MUST be retried according to the exponential backoff schedule

---

### Requirement: Horn of Eld (Circuit Breaker)

Shardik MUST implement a circuit breaker that prevents repeated requests to a failing registry. The circuit breaker MUST transition between closed, open, and half-open states.

#### Scenario: Circuit breaker opens after consecutive failures

GIVEN the circuit breaker is in the closed state
WHEN 3 consecutive requests to a registry fail
THEN the circuit breaker MUST transition to the open state
AND all subsequent requests MUST be rejected immediately without contacting the registry
AND the error MUST indicate the circuit breaker is open

#### Scenario: Circuit breaker enters half-open after timeout

GIVEN the circuit breaker is in the open state
WHEN 60 seconds have elapsed since the circuit opened
THEN the circuit breaker MUST transition to the half-open state
AND exactly one probe request MUST be allowed through

#### Scenario: Successful probe closes circuit breaker

GIVEN the circuit breaker is in the half-open state
WHEN the probe request succeeds
THEN the circuit breaker MUST transition to the closed state
AND all subsequent requests MUST be allowed through normally

#### Scenario: Failed probe reopens circuit breaker

GIVEN the circuit breaker is in the half-open state
WHEN the probe request fails
THEN the circuit breaker MUST transition back to the open state
AND the 60-second timeout MUST restart

#### Scenario: Success resets failure count

GIVEN the circuit breaker is in the closed state
AND 2 consecutive failures have occurred
WHEN a request succeeds
THEN the failure count MUST be reset to 0
AND the circuit breaker MUST remain in the closed state

#### Scenario: Circuit breaker is per-registry

GIVEN registry A has an open circuit breaker
AND registry B has a closed circuit breaker
WHEN a request is made to registry B
THEN the request MUST be allowed through
AND registry A's circuit breaker state MUST NOT affect registry B

---

### Requirement: Content Negotiation

Shardik MUST send appropriate Accept headers to indicate support for both OCI and Docker manifest formats. This ensures compatibility with registries that serve either format.

#### Scenario: Accept header includes OCI manifest types

WHEN Shardik sends a manifest request
THEN the Accept header MUST include `application/vnd.oci.image.manifest.v1+json`
AND the Accept header MUST include `application/vnd.oci.image.index.v1+json`

#### Scenario: Accept header includes Docker manifest types

WHEN Shardik sends a manifest request
THEN the Accept header MUST include `application/vnd.docker.distribution.manifest.v2+json`
AND the Accept header MUST include `application/vnd.docker.distribution.manifest.list.v2+json`

#### Scenario: Registry returns OCI format when preferred

GIVEN a registry supports both OCI and Docker manifest formats
WHEN Shardik sends a request with OCI types listed first in the Accept header
THEN the registry SHOULD return OCI-format manifests
AND Shardik MUST accept the returned format

#### Scenario: Registry returns Docker format

GIVEN a registry only serves Docker V2 manifests
WHEN Shardik sends a request with both OCI and Docker types in the Accept header
THEN the registry returns a Docker V2 manifest
AND Shardik MUST accept the Docker V2 manifest without error

---

### Requirement: Parallel Blob Downloads

Shardik MUST support downloading multiple blobs concurrently. The concurrency level MUST be configurable with a default of 4 parallel downloads.

#### Scenario: Default parallel download limit is 4

GIVEN no custom parallelism configuration
WHEN multiple blobs are requested for download
THEN at most 4 blob downloads MUST be active concurrently

#### Scenario: Custom parallel download limit is respected

GIVEN the parallel download limit is configured to 2
WHEN 4 blobs are requested for download
THEN at no point MUST more than 2 concurrent downloads be active
AND all 4 blobs MUST eventually be downloaded

#### Scenario: Parallel download limit of 1 serializes downloads

GIVEN the parallel download limit is configured to 1
WHEN multiple blobs are requested
THEN downloads MUST proceed sequentially, one at a time

#### Scenario: One download failure does not cancel others

GIVEN 4 parallel blob downloads are in progress
WHEN one download fails
THEN the remaining 3 downloads MUST continue to completion
AND the caller MUST be informed of the single failure

---

### Requirement: Chunked Blob Uploads

Shardik MUST support chunked blob uploads for large blobs. Chunked uploads allow uploading a blob in multiple PATCH requests before finalizing with a PUT.

#### Scenario: Large blob uploaded in chunks

GIVEN a blob larger than the configured chunk size
WHEN Shardik initiates a chunked upload
THEN it MUST send `POST /v2/<name>/blobs/uploads/` to initiate
AND it MUST send one or more `PATCH <location>` requests with Content-Range headers
AND each chunk MUST have a Content-Range indicating its byte range
AND it MUST finalize with `PUT <location>?digest=<digest>`

#### Scenario: Chunk size is configurable

GIVEN a chunk size configured to 5 MB
WHEN a 12 MB blob is uploaded
THEN the blob MUST be uploaded in 3 chunks (5 MB + 5 MB + 2 MB)

#### Scenario: Monolithic upload for small blobs

GIVEN a blob smaller than the configured chunk threshold
WHEN Shardik uploads the blob
THEN it MUST use monolithic upload (single POST + PUT)
AND chunked upload MUST NOT be used

---

### Requirement: Cross-Repository Blob Mount

Shardik MUST support cross-repository blob mounting as defined by the OCI Distribution Spec. When pushing an image, if a required blob exists in another repository on the same registry, Shardik MUST attempt to mount it rather than re-upload.

#### Scenario: Mount succeeds for known source repository

GIVEN blob "sha256:abc..." exists in repository "base/image" on the registry
WHEN Shardik pushes to "myapp/image" and detects the blob is needed
THEN it MUST attempt `POST /v2/myapp/image/blobs/uploads/?mount=sha256:abc...&from=base/image`
AND if the registry responds with 201, the blob MUST be considered uploaded

#### Scenario: Mount fails and falls back to regular upload

GIVEN a cross-repo mount is attempted
AND the registry responds with 202 (mount failed, upload initiated)
THEN Shardik MUST proceed with a regular blob upload using the returned upload URL
AND the blob MUST be uploaded completely

---

### Requirement: Tag Listing

Shardik MUST support listing all tags for a given repository on a registry.

#### Scenario: List all tags for a repository

GIVEN a repository "library/nginx" has tags ["latest", "1.25", "1.24", "1.23"]
WHEN Shardik lists tags
THEN the result MUST contain all 4 tags

#### Scenario: List tags with pagination

GIVEN a repository has more tags than the registry returns in a single response
WHEN Shardik lists tags
THEN it MUST follow pagination links to retrieve all tags
AND the final result MUST be the complete, aggregated tag list

#### Scenario: List tags for empty repository

GIVEN a repository exists but has no tags
WHEN Shardik lists tags
THEN the result MUST be an empty list
AND no error MUST be returned

#### Scenario: List tags for non-existent repository

GIVEN a repository does not exist on the registry
WHEN Shardik lists tags
THEN the system MUST return an appropriate error (typically 404)

---

### Requirement: Referrers API

Shardik MUST support the OCI Referrers API (v1.1.0) for discovering artifacts associated with a manifest.

#### Scenario: List referrers for a manifest

GIVEN manifest "sha256:abc..." has associated referrers (e.g., a signature and an SBOM)
WHEN Shardik sends `GET /v2/myapp/referrers/sha256:abc...`
THEN the response MUST be an OCI Image Index
AND it MUST contain entries for each referrer manifest

#### Scenario: Filter referrers by artifact type

GIVEN manifest "sha256:abc..." has referrers of types "application/spdx+json" and "application/vnd.dev.sigstore.bundle.v0.3+json"
WHEN Shardik requests referrers with filter "application/spdx+json"
THEN the result MUST contain only the SBOM referrer

#### Scenario: No referrers returns empty index

GIVEN manifest "sha256:abc..." has no referrers
WHEN Shardik lists referrers
THEN the response MUST be an OCI Image Index with an empty manifests array

---

### Requirement: Timeout Configuration

Shardik MUST enforce configurable timeout limits for different types of operations to prevent indefinite hangs.

#### Scenario: Connection timeout

GIVEN the connection timeout is configured to 10 seconds
WHEN Shardik attempts to establish a TCP connection to a registry
AND the connection is not established within 10 seconds
THEN the operation MUST fail with a connection timeout error

#### Scenario: Blob transfer timeout

GIVEN the blob transfer timeout is configured to 5 minutes
WHEN Shardik is downloading or uploading a blob
AND no data is transferred for 5 minutes
THEN the operation MUST fail with a transfer timeout error

#### Scenario: Total operation timeout

GIVEN the total operation timeout is configured to 30 minutes
WHEN a complex operation (e.g., pulling a large image with many layers) exceeds 30 minutes total
THEN the operation MUST fail with a total timeout error
AND partially downloaded blobs MAY be retained for future resumption

#### Scenario: Default timeout values

GIVEN no custom timeout configuration is provided
WHEN Shardik is initialized
THEN the connection timeout MUST default to 10 seconds
AND the blob transfer timeout MUST default to 5 minutes
AND the total operation timeout MUST default to 30 minutes

#### Scenario: Custom timeout values override defaults

GIVEN the configuration sets connection timeout to 30 seconds and blob timeout to 10 minutes
WHEN Shardik is initialized
THEN the connection timeout MUST be 30 seconds
AND the blob transfer timeout MUST be 10 minutes

---

### Requirement: Error Handling for Registry Responses

Shardik MUST handle all standard HTTP error responses from registries and translate them into meaningful errors that include the registry hostname, the operation attempted, and the HTTP status code.

#### Scenario: 401 Unauthorized triggers authentication flow

GIVEN Shardik receives a 401 response with a WWW-Authenticate header
WHEN processing the response
THEN Shardik MUST initiate the Docker V2 token authentication flow
AND the original request MUST be retried with the obtained bearer token

#### Scenario: 401 after authentication retry produces error

GIVEN Shardik receives 401 even after completing the token authentication flow
WHEN processing the response
THEN the system MUST return an authentication error
AND the error MUST include the registry hostname and the scope that was requested

#### Scenario: 403 Forbidden produces authorization error

GIVEN Shardik receives a 403 response
WHEN processing the response
THEN the system MUST return an authorization error indicating the user lacks permission
AND the error MUST include the repository and operation that was denied

#### Scenario: 404 Not Found produces not-found error

GIVEN Shardik receives a 404 response for a manifest or blob request
WHEN processing the response
THEN the system MUST return a not-found error
AND the error MUST include the reference or digest that was not found

#### Scenario: 429 Too Many Requests handled with backoff

GIVEN Shardik receives a 429 response
WHEN processing the response
THEN Shardik MUST respect the `Retry-After` header if present
AND the request MUST be retried after the specified delay
AND if no `Retry-After` header is present, the standard exponential backoff MUST apply

#### Scenario: 500 Internal Server Error triggers retry

GIVEN Shardik receives a 500 response
WHEN processing the response
THEN the request MUST be retried according to the Horn of Eld retry policy

#### Scenario: 502 Bad Gateway triggers retry

GIVEN Shardik receives a 502 response
WHEN processing the response
THEN the request MUST be retried according to the Horn of Eld retry policy

#### Scenario: 503 Service Unavailable triggers retry

GIVEN Shardik receives a 503 response with or without Retry-After
WHEN processing the response
THEN the request MUST be retried
AND if `Retry-After` is present, the specified delay MUST take precedence over computed backoff

#### Scenario: 504 Gateway Timeout triggers retry

GIVEN Shardik receives a 504 response
WHEN processing the response
THEN the request MUST be retried according to the Horn of Eld retry policy

#### Scenario: Unknown 4xx errors are not retried

GIVEN Shardik receives a 400, 405, 409, or other non-retriable 4xx response
WHEN processing the response
THEN the error MUST be returned immediately without retry
AND the error MUST include the HTTP status code and any error body from the registry

#### Scenario: Registry error response body is included

GIVEN a registry returns an error response with a JSON body containing error code and message
WHEN Shardik processes the error
THEN the error MUST include the registry's error code and message
AND the error MUST be structured for programmatic handling

---

### Requirement: OCI Distribution API Version Check

Shardik MUST verify registry compatibility by checking the `/v2/` endpoint before performing operations.

#### Scenario: Successful version check

WHEN Shardik sends `GET /v2/` to a registry
AND the registry responds with 200
THEN the registry MUST be considered compatible
AND subsequent operations MUST proceed

#### Scenario: Version check receives 401

WHEN Shardik sends `GET /v2/` to a registry
AND the registry responds with 401 and a WWW-Authenticate header
THEN Shardik MUST initiate the authentication flow
AND if authentication succeeds, the registry MUST be considered compatible

#### Scenario: Version check fails with non-200/401 response

WHEN Shardik sends `GET /v2/` to a registry
AND the registry responds with 404 or another error
THEN the system MUST return an error indicating the registry does not support the OCI Distribution Spec

---

### Requirement: Image Reference Parsing

Shardik MUST correctly parse OCI image references into their component parts: registry host, repository path, tag, and digest.

#### Scenario: Parse fully qualified reference with tag

GIVEN the reference "registry.example.com/myorg/myapp:v1.0"
WHEN Shardik parses the reference
THEN the registry MUST be "registry.example.com"
AND the repository MUST be "myorg/myapp"
AND the tag MUST be "v1.0"

#### Scenario: Parse reference with digest

GIVEN the reference "registry.example.com/myorg/myapp@sha256:abc123..."
WHEN Shardik parses the reference
THEN the registry MUST be "registry.example.com"
AND the repository MUST be "myorg/myapp"
AND the digest MUST be "sha256:abc123..."

#### Scenario: Parse Docker Hub short name

GIVEN the reference "nginx:latest"
WHEN Shardik parses the reference
THEN the registry MUST default to "docker.io"
AND the repository MUST be "library/nginx"
AND the tag MUST be "latest"

#### Scenario: Parse Docker Hub short name without tag

GIVEN the reference "nginx"
WHEN Shardik parses the reference
THEN the registry MUST default to "docker.io"
AND the repository MUST be "library/nginx"
AND the tag MUST default to "latest"

#### Scenario: Parse reference with port number

GIVEN the reference "localhost:5000/myapp:v1"
WHEN Shardik parses the reference
THEN the registry MUST be "localhost:5000"
AND the repository MUST be "myapp"
AND the tag MUST be "v1"

#### Scenario: Parse Docker Hub user repository

GIVEN the reference "myuser/myapp:latest"
WHEN Shardik parses the reference
THEN the registry MUST default to "docker.io"
AND the repository MUST be "myuser/myapp"
AND the tag MUST be "latest"

#### Scenario: Invalid reference produces error

GIVEN an invalid reference string with illegal characters
WHEN Shardik attempts to parse the reference
THEN the system MUST return a parse error indicating the reference is malformed

---

### Requirement: TLS Configuration

Shardik MUST use TLS for all registry communication by default. The TLS configuration MUST be adjustable per registry endpoint.

#### Scenario: TLS enabled by default

GIVEN a registry at "registry.example.com"
WHEN Shardik connects to the registry
THEN the connection MUST use HTTPS
AND TLS certificate verification MUST be enforced

#### Scenario: Skip TLS verification for specific registry

GIVEN a registry is configured with `skip_verify = true`
WHEN Shardik connects to this registry
THEN TLS certificate verification MUST be skipped
AND a warning SHOULD be logged indicating insecure connection

#### Scenario: Plain HTTP for localhost registries

GIVEN a registry at "localhost:5000" with no TLS configuration
WHEN Shardik connects to this registry
THEN plain HTTP MAY be used as a fallback
AND this MUST only apply to localhost or 127.0.0.1 addresses

#### Scenario: Custom CA certificate

GIVEN a custom CA certificate path is configured for a registry
WHEN Shardik connects to that registry
THEN the custom CA MUST be used for TLS verification
AND the system's default CA bundle SHOULD also be trusted
