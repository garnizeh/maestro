# Rose Artifact Specification

## Purpose

Rose manages OCI artifacts: push/pull non-container content (Helm charts, WASM modules, policies, SBOMs) to/from registries using the OCI v1.1 artifact model. Includes Nineteen (Referrers API) for discovering artifacts attached to images.

The Rose is not the Tower itself, but an artifact that represents and is connected to the Tower -- a different manifestation in a different world.

---

## Requirements

### Requirement: Artifact Push

The system MUST support pushing arbitrary files to an OCI registry as artifacts. Each artifact MUST include a user-specified artifact type (media type) and MAY include annotations. The artifact MUST be stored as a valid OCI manifest in the registry.

#### Scenario: Push artifact with explicit type

GIVEN a local file exists
AND the user specifies an artifact type and a target registry reference
WHEN the user invokes artifact push
THEN the file MUST be uploaded to the registry as an OCI artifact
AND the manifest MUST contain the specified artifact type
AND the manifest MUST be retrievable by the target reference

#### Scenario: Push artifact to unreachable registry

GIVEN the target registry is unreachable
WHEN the user invokes artifact push
THEN the system MUST return an error indicating the registry could not be contacted

---

### Requirement: Artifact Pull

The system MUST support pulling artifacts from an OCI registry to a local file. The system MUST resolve the reference, download the artifact layers, and write them to the specified output path.

#### Scenario: Pull artifact to local file

GIVEN an artifact exists at the specified registry reference
WHEN the user invokes artifact pull with an output path
THEN the artifact content MUST be written to the specified local file
AND the file content MUST match what was originally pushed

---

### Requirement: Artifact Attach to Image

The system MUST support attaching an artifact to an existing image by setting the subject field in the artifact manifest to reference the image's digest. This enables discovery via the Referrers API.

#### Scenario: Attach artifact as referrer to image

GIVEN a pushed image exists at a registry reference
AND a local file to attach exists
WHEN the user invokes artifact attach with the image reference and file
THEN the artifact MUST be pushed with its subject field set to the image's manifest digest
AND the artifact MUST be discoverable via the Referrers API for that image

---

### Requirement: Nineteen (Referrers API Discovery)

The system MUST support discovering artifacts attached to an image via the OCI Referrers API. The system MUST list all artifacts that reference a given image digest, including their artifact type, digest, and size.

#### Scenario: List referrers for an image

GIVEN an image exists with one or more attached artifacts (signatures, SBOMs)
WHEN the user invokes artifact list for the image reference
THEN the output MUST list each attached artifact with its artifact type, digest, and size

#### Scenario: No referrers found

GIVEN an image exists with no attached artifacts
WHEN the user invokes artifact list for the image reference
THEN the output MUST indicate that no artifacts are attached

---

### Requirement: Supported Artifact Types

The system MUST support pushing and pulling artifacts of any media type. The system SHOULD recognize and display human-friendly labels for well-known types including: Helm charts, WASM modules, OPA policies, Sigstore signatures, SPDX SBOMs, CycloneDX SBOMs, and in-toto attestations.

#### Scenario: Well-known type displayed with friendly label

GIVEN an artifact with media type "application/spdx+json" is attached to an image
WHEN the user lists referrers for that image
THEN the output SHOULD display a human-readable label (e.g., "SBOM (SPDX)") alongside the raw media type

---

### Requirement: Artifact Inspect

The system MUST support inspecting an artifact's metadata without downloading its full content. Inspection MUST display the artifact type, size, digest, annotations, and subject reference (if present).

#### Scenario: Inspect artifact metadata

GIVEN an artifact exists at a registry reference
WHEN the user invokes artifact inspect
THEN the output MUST include the artifact type, total size, manifest digest, and any annotations
AND if the artifact has a subject field, the referenced image digest MUST be displayed
