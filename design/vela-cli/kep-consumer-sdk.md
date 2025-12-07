# KEP: Go SDK for Application Consumers

## Summary

This KEP proposes a fluent, type-safe Go SDK for consuming KubeVela X-Definitions. Application developers use this SDK to programmatically create Applications, Components, Traits, Policies, and Workflow Steps without writing YAML.

## Motivation

### Goals

1. **Fluent builder pattern** for constructing KubeVela Applications in Go
2. **Type-safe** parameter validation at compile time
3. **IDE support** with autocomplete and inline documentation
4. **Automatic synchronization** with KubeVela core releases
5. **Custom X-Definition support** - generate SDK from platform-specific definitions

### Non-Goals

1. Authoring new X-Definitions (covered by defkit KEP)
2. Multi-language support in initial release (Go only; see [Future Language SDKs](#future-language-sdks))
3. Runtime CUE evaluation in SDK
4. Replacing YAML/CUE as valid input formats

### Relationship to Existing SDK Tooling

KubeVela currently provides `vela def gen-api` for generating Go code from definitions. This KEP introduces a new, enhanced SDK generation system. The migration path is:

| Version | `vela def gen-api` (existing) | `vela sdk generate` (new) |
|---------|-------------------------------|---------------------------|
| v1.13 and earlier | Stable, recommended | Not available |
| **v1.14** | Unchanged, continues working | **Introduced as alpha** |
| v1.x (between v1.14 and v2) | Unchanged, continues working | Matures based on feedback |
| **v2.0** | **Deprecated** (with migration notice) | **GA (Generally Available)** |
| v2.x+ | Removed after sufficient adoption | Stable, recommended |

**Migration Commitment:**

1. **No breakage in v1.x**: The existing `vela def gen-api` command will continue working without changes throughout the v1.x release series

2. **Alpha period for feedback**: The v1.14 alpha release allows users to try the new SDK generator and provide feedback before GA

3. **Sufficient migration window**: Deprecation in v2.0 provides a full major version cycle for users to migrate. The old command will not be removed until the new SDK has proven parity and adoption

4. **Migration tooling**: We will provide:
   - Documentation mapping old patterns to new patterns
   - Migration guide with examples
   - Codemods or automated refactoring where feasible

**Key differences between old and new:**

| Aspect | `vela def gen-api` | `vela sdk generate` |
|--------|-------------------|---------------------|
| OneOf/union support | Limited | Full discriminated union support |
| Lifecycle functions | Not supported | `Apply()`, `Validate()`, `Destroy()` |
| CLI integration | Generate + manual apply | Direct `vela apply myapp.go` |
| Validation | Runtime only | Compile-time + runtime |
| Generated tests | No | Yes |

## Proposal

### Target Developer Experience

The SDK uses a **lifecycle-based approach** with explicit `Apply()`, `Validate()`, and `Destroy()` functions. A minimal `main()` delegates to `sdk.RunLifecycle()`:

```go
package main

import (
    "github.com/oam-dev/kubevela/pkg/sdk"
    "github.com/oam-dev/kubevela/pkg/sdk/apis/component/webservice"
    "github.com/oam-dev/kubevela/pkg/sdk/apis/trait/scaler"
    "github.com/oam-dev/kubevela/pkg/sdk/apis/policy/topology"
    "github.com/oam-dev/kubevela/pkg/sdk/apis/workflow/deploy"
)

// Apply returns the Application to deploy
func Apply() *sdk.Application {
    return sdk.NewApplication("my-app").
        Namespace("production").
        Labels(map[string]string{"team": "platform"}).

        // Add a webservice component with traits
        AddComponent(
            webservice.New("frontend").
                Image("nginx:1.21").
                Port(80).
                Replicas(3).
                AddTrait(scaler.New().Min(2).Max(10)),
        ).

        // Add deployment policy
        AddPolicy(
            topology.New("prod-clusters").
                Clusters([]string{"cluster-1", "cluster-2"}),
        ).

        // Add workflow
        AddWorkflowStep(
            deploy.New("deploy-frontend").
                Policies([]string{"prod-clusters"}),
        )
}

// Validate performs custom validation beyond schema checks (optional)
func Validate(app *sdk.Application) error {
    if app.GetNamespace() == "production" {
        for _, c := range app.GetComponentsByType("webservice") {
            if c.GetReplicas() < 3 {
                return fmt.Errorf("component %q: production requires at least 3 replicas", c.ComponentName())
            }
        }
    }
    return nil
}

// Destroy returns the Application to delete (optional)
func Destroy() *sdk.Application {
    return sdk.NewApplication("my-app").Namespace("production")
}

func main() {
    // RunLifecycle automatically discovers and calls Apply(), Validate(), Destroy()
    sdk.RunLifecycle()
}
```

**Lifecycle Functions:**

| Function | Required | Purpose |
|----------|----------|---------|
| `Apply() *sdk.Application` | Yes | Returns the Application to create/update |
| `Validate(app *sdk.Application) error` | No | Custom business rules beyond schema validation |
| `Destroy() *sdk.Application` | No | Returns the Application to delete (for `vela delete`) |

**Philosophy: Minimize Side Effects**

We encourage treating `Apply()` as a pure function that produces the same output given the same inputs. This is a design philosophy, not a technical constraint - Go allows any code in `Apply()`, but minimizing side effects leads to better outcomes:

- Reproducible deployments across environments
- Predictable behavior in CI/CD pipelines
- Easier debugging and testing
- Safe to run multiple times

```go
// ✅ Good: Deterministic, explicit inputs
func Apply() *sdk.Application {
    replicas := getEnvInt("REPLICAS", 3)
    return sdk.NewApplication("my-app").
        AddComponent(webservice.New("web").Replicas(replicas))
}

// ❌ Avoid: Non-deterministic behavior
func Apply() *sdk.Application {
    return sdk.NewApplication("my-app").
        AddComponent(webservice.New("web").
            Replicas(rand.Intn(10)))  // Random - different output each run
}
```

**Deployment:**

```bash
# vela CLI runs the Go program and applies the output
$ vela apply myapp.go

# Delete using Destroy() lifecycle function
$ vela delete myapp.go

# Users can also run independently
$ go run myapp.go | kubectl apply -f -
```

**Multi-file Support:**

SDK code can span multiple files and packages - standard Go project structure is fully supported:

```bash
# Single file
$ vela apply myapp.go

# Directory with multiple files (runs as Go package)
$ vela apply ./myapp/

# Example project structure
myapp/
├── main.go          # func main() { sdk.RunLifecycle() }
├── app.go           # func Apply() *sdk.Application
├── validation.go    # func Validate(app) error
├── components/      # Reusable component configurations
│   ├── frontend.go
│   └── backend.go
└── go.mod
```

### Architecture

The SDK for core definitions lives **inside the KubeVela repository** and is auto-generated as part of the CI/CD pipeline. This ensures:

- SDK is always in sync with the shipped definitions
- Tests are auto-generated and run as part of CI
- SDK version matches the KubeVela release version
- No manual synchronization required

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    In-Repo SDK Generation (Core Definitions)                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  vela-templates/definitions/          pkg/sdk/                               │
│  ├── internal/                        ├── apis/                              │
│  │   ├── component/                   │   ├── component/                     │
│  │   │   ├── webservice.cue  ──────▶ │   │   ├── webservice/                │
│  │   │   ├── worker.cue              │   │   │   ├── webservice.go (gen)    │
│  │   │   └── task.cue                │   │   │   ├── types.go (gen)         │
│  │   ├── trait/                      │   │   │   ├── version.go (gen)       │
│  │   │   ├── scaler.cue              │   │   │   └── webservice_test.go (gen)│
│  │   │   └── ingress.cue             │   │   └── worker/                    │
│  │   ├── policy/                     │   ├── trait/                         │
│  │   └── workflowstep/               │   ├── policy/                        │
│  └── ...                             │   └── workflow/                      │
│                                      ├── client/                             │
│  ┌─────────────────────────┐         └── util/                               │
│  │  make generate-sdk      │                                                 │
│  │  (CI runs on every PR)  │                                                 │
│  └─────────────────────────┘                                                 │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│                  Custom SDK Generation (Platform Definitions)                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐                   │
│  │    Source    │    │   Enhanced   │    │  Go Code     │                   │
│  │              │    │      IR      │    │  Generator   │                   │
│  │ • Cluster    │───▶│ • Schema     │───▶│              │──▶ Custom SDK     │
│  │ • Local CUE  │    │ • CUE Meta   │    │ • Builders   │    (user repo)    │
│  │ • OCI        │    │ • Relations  │    │ • Types      │                   │
│  │              │    │ • Validation │    │ • Tests      │                   │
│  └──────────────┘    └──────────────┘    └──────────────┘                   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Repository Structure for Core SDK

The SDK is generated into the KubeVela main repository. The generation process:

1. **Copies scaffold files** - Core infrastructure (application builder, client, types) from `pkg/definition/gen_sdk/_scaffold/`
2. **Generates definition-specific code** - Fluent builders and types for each component/trait/policy/workflow-step

```
github.com/oam-dev/kubevela/
├── vela-templates/
│   └── definitions/
│       └── internal/
│           ├── component/
│           │   ├── webservice.cue      # Source definitions
│           │   ├── worker.cue
│           │   └── task.cue
│           ├── trait/
│           │   ├── scaler.cue
│           │   ├── ingress.cue
│           │   └── gateway.cue
│           ├── policy/
│           │   └── topology.cue
│           └── workflowstep/
│               ├── deploy.cue
│               └── notification.cue
│
├── pkg/
│   └── sdk/                            # AUTO-GENERATED SDK
│       ├── apis/
│       │   ├── common/
│       │   │   ├── application.go      # Application builder (from scaffold)
│       │   │   └── catalog.go          # Definition catalog (from scaffold)
│       │   ├── types.go                # Core interfaces (from scaffold)
│       │   │
│       │   ├── component/              # Generated per component type
│       │   │   ├── webservice/
│       │   │   │   ├── webservice.go           # Fluent builder (generated)
│       │   │   │   ├── types.go                # Property types (generated)
│       │   │   │   ├── version.go              # Version metadata (generated)
│       │   │   │   └── webservice_test.go      # Tests (generated)
│       │   │   ├── worker/
│       │   │   └── task/
│       │   │
│       │   ├── trait/
│       │   │   ├── scaler/
│       │   │   │   ├── scaler.go
│       │   │   │   ├── types.go
│       │   │   │   └── scaler_test.go
│       │   │   ├── ingress/
│       │   │   └── gateway/
│       │   │
│       │   ├── policy/
│       │   │   └── topology/
│       │   │
│       │   └── workflow/
│       │       ├── deploy/
│       │       └── notification/
│       │
│       ├── client/
│       │   └── client.go               # K8s client wrapper (from scaffold)
│       │
│       └── util/
│           └── ptr.go                  # Utilities (from scaffold)
│
├── hack/
│   └── generate-sdk.sh                 # SDK generation script
│
└── .github/
    └── workflows/
        └── sdk-generate.yaml           # CI workflow for SDK generation
```

### Scaffold Files

The scaffold files in `pkg/definition/gen_sdk/_scaffold/` provide the core SDK infrastructure that is **copied** (not generated) during SDK generation. These files are the foundation that generated definition-specific code builds upon.

| File | Purpose |
|------|---------|
| `apis/types.go` | Core interfaces (`TypedApplication`, `Component`, `Trait`, `WorkflowStep`, `Policy`) and base structs (`ComponentBase`, `TraitBase`, etc.) that generated builders implement |
| `apis/common/application.go` | `ApplicationBuilder` - the main fluent builder for constructing Applications. Provides `Name()`, `Namespace()`, `SetComponents()`, `Build()`, `ToYAML()`, `Validate()`, `FromK8sObject()` |
| `apis/common/catalog.go` | Builder registry - maps definition types to constructors. Generated code registers itself here (e.g., `RegisterComponent("webservice", ...)`) |
| `client/client.go` | Thin wrapper around `controller-runtime/pkg/client` for K8s CRUD operations (`Get`, `Create`, `Update`, `Delete`, `Patch`). Converts SDK builders to K8s CRs via `app.Build()` |

**How scaffold and generated code work together:**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Scaffold + Generated Code Relationship                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  SCAFFOLD (copied)                      GENERATED (per definition)           │
│  ─────────────────                      ──────────────────────────           │
│                                                                              │
│  types.go                               webservice/webservice.go             │
│  ├── type Component interface ◄─────── ├── type WebserviceSpec struct       │
│  │       Build()                        │       implements Component         │
│  │       Validate()                     │                                    │
│  │                                      │                                    │
│  application.go                         │   webservice.New("name")           │
│  ├── SetComponents(c ...Component)◄────┼── returns Component interface      │
│  │                                      │                                    │
│  catalog.go                             │   func init() {                    │
│  ├── ComponentsBuilders map ◄──────────┼──   RegisterComponent("webservice",│
│  ├── RegisterComponent()                │       fromK8sComponent)            │
│  │                                      │   }                                │
│  client/client.go                       │                                    │
│  ├── Create(app TypedApplication)       │                                    │
│  │     calls app.Build() ──────────────►│   Build() returns K8s CR           │
│  │                                      │                                    │
└─────────────────────────────────────────────────────────────────────────────┘
```

The scaffold provides:
1. **Interfaces** - Contracts that all generated builders must implement
2. **Application orchestration** - How components, traits, policies, and workflow steps are assembled
3. **Registry pattern** - How generated code registers itself for discovery (used by `FromK8sObject()`)
4. **K8s client** - How SDK applications are applied to clusters

### Future Language SDKs

The **Go SDK** is in-repo because:

- Go is the primary language for Kubernetes ecosystem tooling
- The SDK shares code with the KubeVela controller (types, validation)
- Tight coupling ensures API consistency with each release

**Future language SDKs** (Python, TypeScript, Java, etc.) will live in **separate repositories**:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Multi-Language SDK Strategy                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  github.com/oam-dev/kubevela/                                                │
│  └── pkg/sdk/                    ◄── Go SDK (in-repo, auto-generated)       │
│                                                                              │
│  github.com/oam-dev/kubevela-sdk-python/    ◄── Python SDK (separate repo)  │
│  github.com/oam-dev/kubevela-sdk-typescript/◄── TypeScript SDK (separate)   │
│  github.com/oam-dev/kubevela-sdk-java/      ◄── Java SDK (separate repo)    │
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                    Shared: Enhanced IR (JSON Schema)                  │   │
│  │                                                                       │   │
│  │  The same IR extracted from CUE definitions is used by all language  │   │
│  │  generators. Each language SDK repo consumes published IR artifacts. │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Why separate repos for other languages:**

| Concern       | Go SDK (In-Repo)             | Other Language SDKs (Separate Repos)          |
| ------------- | ---------------------------- | --------------------------------------------- |
| Release cycle | Tied to KubeVela releases    | Can release independently                     |
| CI/CD         | Uses KubeVela's Go CI        | Language-specific CI (pytest, npm test, etc.) |
| Dependencies  | Shares types with controller | No shared code dependencies                   |
| Maintainers   | Core KubeVela team           | Can have language-specific maintainers        |
| Versioning    | Same as KubeVela version     | Semantic versioning aligned with KubeVela     |

**Generation workflow for other languages:**

```yaml
# In kubevela-sdk-python/.github/workflows/generate.yaml
name: Generate Python SDK
on:
  workflow_dispatch:
    inputs:
      kubevela_version:
        description: "KubeVela version to generate SDK for"
        required: true

jobs:
  generate:
    steps:
      - name: Download IR artifacts from KubeVela release
        run: |
          curl -L -o ir.json \
            "https://github.com/oam-dev/kubevela/releases/download/${{ inputs.kubevela_version }}/sdk-ir.json"

      - name: Generate Python SDK from IR
        run: |
          vela sdk generate --from-ir ir.json --lang python --output ./kubevela_sdk

      - name: Run Python tests
        run: pytest tests/

      - name: Publish to PyPI
        run: python -m twine upload dist/*
```

**IR artifact publishing (added to KubeVela release process):**

```yaml
# In kubevela/.github/workflows/release.yaml (addition)
- name: Generate and publish SDK IR artifact
  run: |
    # Generate IR from definitions
    vela sdk ir-export --from-file ./vela-templates/definitions/internal/... \
                       --output ./dist/sdk-ir.json

    # Attach to GitHub release
    gh release upload ${{ github.ref_name }} ./dist/sdk-ir.json
```

This approach allows:

1. **Language-specific best practices** - Each SDK follows idiomatic patterns for its language
2. **Independent release cycles** - Bug fixes can ship without waiting for KubeVela releases
3. **Community ownership** - Language experts can maintain their SDK
4. **Shared source of truth** - All SDKs generate from the same IR, ensuring consistency

### Lifecycle-Based Deployment

The SDK uses a **lifecycle-based approach** where users define hook functions that `vela` CLI (or `kubectl vela` plugin) invokes. This design:

- **Eliminates `main()` boilerplate** for standard workflows
- **Decouples SDK from K8s client versions** - the CLI owns the client, not the SDK
- **Enables direct deployment** - `vela apply myapp.go` with no intermediate steps

#### Lifecycle Functions

```go
package sdk

// Lifecycle defines the hooks that vela CLI looks for in user code
// Users implement these as package-level functions

// Apply returns the application to deploy (required)
// Called by: vela apply, kubectl vela apply
type ApplyFunc func() *Application

// Validate performs custom validation after automatic SDK validation (optional)
// Called by: vela apply, vela validate
// The app parameter is the result of Apply()
type ValidateFunc func(app *Application) error

// Destroy returns the application to delete (optional)
// Called by: vela delete
// If not provided, Apply() is used to identify what to delete
type DestroyFunc func() *Application
```

#### User Code (No `main()` Required)

```go
package myapp

import (
    "fmt"

    "github.com/oam-dev/kubevela/pkg/sdk"
    "github.com/oam-dev/kubevela/pkg/sdk/apis/component/webservice"
)

// Apply is called by `vela apply myapp.go`
func Apply() *sdk.Application {
    return sdk.NewApplication("my-app").
        Namespace("production").
        AddComponent(
            webservice.New("frontend").
                Image("nginx:1.21").
                Port(80).
                Replicas(3),
        )
}

// Validate is optional - for custom business rules beyond schema validation
func Validate(app *sdk.Application) error {
    if app.GetNamespace() == "production" {
        for _, c := range app.GetComponentsByType("webservice") {
            if c.GetReplicas() < 3 {
                return fmt.Errorf("component %q: production requires at least 3 replicas", c.ComponentName())
            }
        }
    }
    return nil
}
```

#### CLI Invocation

```bash
# Deploy directly - no main(), no YAML intermediate step
vela apply myapp.go

# Or via kubectl plugin
kubectl vela apply myapp.go

# Validate without deploying
vela validate myapp.go

# Delete the application
vela delete myapp.go

# Export YAML (for GitOps, review, or debugging)
vela export myapp.go > app.yaml

# Dry-run to see what would be applied
vela apply myapp.go --dry-run
```

#### Language Detection

The CLI determines the input language using:

1. **Explicit `--language` flag** (highest priority)
2. **File extension inference** (fallback)
3. **Error if ambiguous or unsupported**

```bash
# Explicit language specification
vela apply myapp.go --language=go
vela apply myapp.py --language=python
vela apply app.cue --language=cue        # CUE is the default for .cue files

# Extension-based inference (default behavior)
vela apply myapp.go                       # Infers Go from .go extension
vela apply myapp.py                       # Infers Python from .py extension
vela apply app.yaml                       # Infers YAML/manifest
vela apply app.cue                        # Infers CUE (default)

# Override inference when extension is misleading
vela apply app.txt --language=cue         # Treat .txt as CUE
```

**Supported languages and extensions (source files only):**

| Language | Extension | Runtime Required | Status |
|----------|-----------|------------------|--------|
| CUE | `.cue` | None (built into vela) | Current default |
| YAML | `.yaml`, `.yml` | None | Passthrough |
| Go | `.go` | Go toolchain | Phase 1 |
| Python | `.py` | Python 3.x | Future |
| TypeScript | `.ts` | Node.js + ts-node | Future |

> **Note:** Binary/compiled artifacts (Go binaries, JARs, etc.) are not supported in the initial release. Users must provide source files. Binary support may be added in a future phase based on community feedback.

**Runtime requirements:**

```bash
# Go source files require Go toolchain
$ vela apply myapp.go
Error: Go toolchain not found
  Install Go from https://go.dev/dl/
  Required: go 1.21 or later

# Python source files require Python runtime (future)
$ vela apply myapp.py
Error: Python runtime not found
  Install Python from https://python.org/
  Required: python 3.9 or later
```

**Error handling:**

```bash
# Unknown extension without --language flag
$ vela apply myapp.xyz
Error: unable to determine language for "myapp.xyz"
  Supported extensions: .cue, .yaml, .yml, .go
  Use --language=<lang> to specify explicitly

# Explicit language doesn't match content
$ vela apply myapp.go --language=cue
Error: failed to parse "myapp.go" as CUE
  File appears to be Go source code
  Did you mean: vela apply myapp.go --language=go

# Go file missing Apply() function
$ vela apply broken.go
Error: "broken.go" does not export an Apply() function
  Go SDK files must export: func Apply() *sdk.Application

# Syntax error in Go file
$ vela apply invalid.go
Error: failed to compile "invalid.go"
  invalid.go:15:5: undefined: webservice
  Hint: missing import "github.com/oam-dev/kubevela/pkg/sdk/apis/component/webservice"?
```

**Language detection flow:**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  vela apply <file> [--language=<lang>]                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. Check --language flag                                                    │
│     ├── If set → use specified language                                     │
│     └── If not set → continue to step 2                                     │
│                                                                              │
│  2. Infer from file extension (source files only)                           │
│     ├── .cue        → CUE                                                   │
│     ├── .yaml/.yml  → YAML manifest                                         │
│     ├── .go         → Go SDK (requires Go toolchain)                        │
│     ├── .py         → Python SDK (requires Python, future)                  │
│     ├── .ts         → TypeScript SDK (requires Node, future)                │
│     └── unknown     → Error: "use --language to specify"                    │
│                                                                              │
│  3. Check runtime availability                                               │
│     ├── Go  → check `go version`                                            │
│     ├── Python → check `python3 --version`                                  │
│     ├── TypeScript → check `node --version` + `npx ts-node --version`       │
│     └── Missing runtime → Error with install instructions                   │
│                                                                              │
│  4. Compile/interpret source file                                            │
│     ├── Success → continue to validation                                    │
│     └── Failure → Error with helpful message                                │
│           - Show syntax/compilation errors                                   │
│           - For Go: check for Apply() function                              │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### Go Execution Mechanism

The `vela` CLI executes Go SDK source files using `go run`:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  vela apply myapp.go                                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. Verify Go toolchain is available                                         │
│                                                                              │
│  2. Execute: go run myapp.go                                                 │
│                                                                              │
│  3. sdk.RunLifecycle() inside user's main():                                │
│     a. Calls Apply() to get the Application                                 │
│     b. Runs schema validation on all components/traits/policies             │
│     c. Calls Validate(app) if defined for custom business rules             │
│     d. Outputs Application YAML to stdout                                   │
│     e. Writes errors to stderr, exits non-zero on failure                   │
│                                                                              │
│  4. CLI captures stdout and applies to cluster                              │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

This approach:
- Uses the standard Go toolchain - no custom interpreters or plugins
- Matches patterns used by Pulumi, CDK, and Terraform providers
- Allows users to run `go run myapp.go | kubectl apply -f -` independently
- Works with any IDE, debugger, and standard Go testing

**Requirements:**
- Go toolchain (1.21+) installed and in PATH
- User code must be `package main` with `func main()` calling `sdk.RunLifecycle()`
- Must define `func Apply() *sdk.Application`
- `go.mod` must exist (CLI can initialize if missing)

#### Execution Model: Compile-Time, Not Runtime

**Important:** Go SDK code is **compiled to an Application CR (YAML) ahead of time** by the `vela` CLI. The Go code is **not** sent to or evaluated by the KubeVela controller or Kubernetes.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Go SDK Execution Model                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   Developer Machine (vela CLI)              │  Kubernetes Cluster           │
│   ─────────────────────────────             │  ───────────────────          │
│                                             │                                │
│   myapp.go                                  │                                │
│       │                                     │                                │
│       ▼                                     │                                │
│   ┌─────────────┐                           │                                │
│   │ vela apply  │  (compiles Go,            │                                │
│   │             │   executes Apply())       │                                │
│   └─────────────┘                           │                                │
│       │                                     │                                │
│       ▼                                     │                                │
│   Application CR (YAML)  ───────────────────┼──►  KubeVela Controller       │
│                                             │           │                    │
│   Go code NEVER reaches                     │           ▼                    │
│   the cluster                               │     CUE Template Evaluation    │
│                                             │           │                    │
│                                             │           ▼                    │
│                                             │     Kubernetes Resources       │
│                                             │     (Deployment, Service...)   │
│                                             │                                │
└─────────────────────────────────────────────┴────────────────────────────────┘
```

**What this means:**

| Aspect | Behavior |
|--------|----------|
| Go code execution | Runs **locally** on developer machine via `vela` CLI |
| What's sent to cluster | Only the **Application CR** (YAML/JSON) - no Go code |
| Controller's role | Evaluates **CUE templates** from X-Definitions, not Go |
| Runtime evaluation | CUE templates are evaluated by controller; Go is already done |

**Comparison with CUE workflow:**

```
Traditional CUE:     app.cue  ──► vela CLI ──► Application CR ──► Controller ──► K8s Resources
                                    │
                                    └── CUE evaluated here OR by controller (depends on command)

Go SDK:              myapp.go ──► vela CLI ──► Application CR ──► Controller ──► K8s Resources
                                    │
                                    └── Go compiled & executed HERE (never reaches cluster)
```

#### Execution Lifecycle

When `vela apply myapp.go` is executed:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  vela apply myapp.go                                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  LOCAL EXECUTION (on developer machine / CI)                                 │
│  ════════════════════════════════════════════                                │
│                                                                              │
│  1. COMPILE & EXECUTE GO CODE                                                │
│     ─────────────────────────                                                │
│     - Compile myapp.go (Go plugin or yaegi interpreter)                     │
│     - Call Apply() → *sdk.Application (in-memory data structure)            │
│     - Go code runs HERE, not on cluster                                     │
│                                                                              │
│  2. AUTOMATIC VALIDATION (SDK validates all components/traits/policies)     │
│     ────────────────────────────────────────────────────────────────────    │
│     Application-level:                                                       │
│       ✓ Name is set                                                         │
│       ✓ Namespace is set                                                    │
│       ✓ At least one component exists                                       │
│                                                                              │
│     Per-component:                                                           │
│       ✓ Component name is unique                                            │
│       ✓ Required parameters are set (image, etc.)                           │
│       ✓ Parameter types are correct                                         │
│       ✓ Constraints satisfied (min/max/pattern)                             │
│       ✓ OneOf variants are selected                                         │
│                                                                              │
│     Per-trait:                                                               │
│       ✓ Trait is compatible with component type                             │
│       ✓ Trait parameters are valid                                          │
│                                                                              │
│     Policies and workflow steps:                                             │
│       ✓ Parameters valid, references exist                                  │
│                                                                              │
│     ✗ If validation fails → exit with error, no apply                       │
│                                                                              │
│  3. CUSTOM VALIDATION (if Validate(app) function exists)                    │
│     ──────────────────────────────────────────────────                      │
│     - Call Validate(app) with the application                               │
│     - User's custom business rules (e.g., "prod needs 3+ replicas")         │
│                                                                              │
│     ✗ If custom validation fails → exit with error, no apply                │
│                                                                              │
│  4. BUILD APPLICATION CR                                                     │
│     ────────────────────                                                     │
│     - app.Build() → v1beta1.Application CR (Kubernetes object)              │
│     - This is pure data (YAML/JSON) - no Go code                            │
│                                                                              │
│  ════════════════════════════════════════════                                │
│  CLUSTER INTERACTION                                                         │
│  ════════════════════════════════════════════                                │
│                                                                              │
│  5. APPLY TO CLUSTER                                                         │
│     ────────────────                                                         │
│     - vela CLI sends Application CR to Kubernetes API                       │
│     - Only YAML/JSON is sent - Go code stays local                          │
│     - Controller takes over from here (CUE template evaluation)             │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### CLI Output

```bash
$ vela apply myapp.go
Loading myapp.go...
Validating...
  ✓ Application "my-app"
  ✓ Component "frontend" (webservice)
    ✓ image: nginx:1.21
    ✓ port: 80
    ✓ replicas: 3
  ✓ Custom validation passed
Applying to namespace "production"...
  ✓ Application "my-app" created

$ vela apply broken.go
Loading broken.go...
Validating...
  ✗ Component "frontend" (webservice): image is required
Error: validation failed

$ vela validate myapp.go
Loading myapp.go...
Validating...
  ✓ Application "my-app"
  ✓ Component "frontend" (webservice)
  ✓ Custom validation passed
Valid.
```

#### Power User: Custom `main()` with Client Control

For users who need full control over the K8s client (custom authentication, specific K8s version, testing, etc.):

```go
package main

import (
    "context"
    "log"

    "mycompany/myapp"

    "k8s.io/client-go/tools/clientcmd"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

func main() {
    // User's code - they manage K8s client and version compatibility
    app := myapp.Apply()

    // Custom validation if needed
    if err := myapp.Validate(app); err != nil {
        log.Fatal(err)
    }

    // User controls the K8s client
    config, _ := clientcmd.BuildConfigFromFlags("", kubeconfig)
    k8sClient, _ := client.New(config, client.Options{})

    // Build and apply
    cr, err := app.Build()
    if err != nil {
        log.Fatal(err)
    }
    if err := k8sClient.Create(context.Background(), &cr); err != nil {
        log.Fatal(err)
    }
}
```

This approach gives power users full control while keeping the common case (`vela apply myapp.go`) simple.

#### Design Benefits

| Concern | Solution |
|---------|----------|
| K8s version compatibility | `vela` CLI owns the K8s client, not the SDK |
| Simple UX | `vela apply myapp.go` - one command, no intermediate steps |
| GitOps support | `vela export myapp.go > app.yaml` for manifest-based workflows |
| Testability | `Apply()` is a pure function returning data - easy to unit test |
| Power users | Can write `main()` and manage client themselves |
| Multi-language | Same lifecycle pattern works for Python, TypeScript SDKs |

### Intermediate Representation (IR)

The IR captures schema information beyond what OpenAPI provides. Standard OpenAPI generation loses CUE-specific constructs like conditional requirements, closed structs, and discriminated unions. Our enhanced IR preserves this metadata to generate more accurate Go code with proper validation.

```go
// pkg/definition/gen_sdk/ir/definition.go

type DefinitionIR struct {
    Name        string            `json:"name"`
    Type        DefinitionType    `json:"type"` // component, trait, policy, workflow-step
    Description string            `json:"description"`
    Version     string            `json:"version"`
    Labels      map[string]string `json:"labels"`
    Annotations map[string]string `json:"annotations"`
    Properties  *PropertySchema   `json:"properties"`

    // Relationships (for traits)
    AppliesToWorkloads []string  `json:"appliesToWorkloads,omitempty"`
    ConflictsWith      []string  `json:"conflictsWith,omitempty"`

    // Version tracking for compatibility checks
    DefinitionHash string           `json:"definitionHash"`
    Source         DefinitionSource `json:"source"`
}

type PropertySchema struct {
    Type        string                     `json:"type"`
    Description string                     `json:"description"`
    Required    bool                       `json:"required"`
    Default     interface{}                `json:"default,omitempty"`
    Properties  map[string]*PropertySchema `json:"properties,omitempty"`
    Items       *PropertySchema            `json:"items,omitempty"`
    Enum        []interface{}              `json:"enum,omitempty"`

    // Validation constraints
    Min       *int   `json:"min,omitempty"`
    Max       *int   `json:"max,omitempty"`
    MinLength *int   `json:"minLength,omitempty"`
    MaxLength *int   `json:"maxLength,omitempty"`
    Pattern   string `json:"pattern,omitempty"`

    // CUE-specific metadata (preserved from source)
    // These enable accurate validation that OpenAPI alone cannot express
    ClosedStruct          bool                       `json:"closedStruct,omitempty"`
    ConditionallyRequired []ConditionalRequirement   `json:"conditionallyRequired,omitempty"`
    OneOfVariants         []OneOfVariant             `json:"oneOfVariants,omitempty"`
    Discriminator         *DiscriminatorInfo         `json:"discriminator,omitempty"`
}

// ConditionalRequirement captures CUE patterns like:
// if parameter.type == "pvc" { claimName: string }
type ConditionalRequirement struct {
    Condition string   `json:"condition"` // e.g., "type == 'pvc'"
    Required  []string `json:"required"`  // fields required when condition is true
}

// OneOfVariant represents a variant in a discriminated union
type OneOfVariant struct {
    Name       string          `json:"name"`
    Schema     *PropertySchema `json:"schema"`
    Identifier string          `json:"identifier,omitempty"` // discriminator value
}

// DiscriminatorInfo captures which field determines the variant
type DiscriminatorInfo struct {
    PropertyName string            `json:"propertyName"`
    Mapping      map[string]string `json:"mapping"` // value -> variant name
}
```

### Generated Code Structure

The SDK lives within the KubeVela main repository at `pkg/sdk/`:

```
github.com/oam-dev/kubevela/pkg/sdk/
├── apis/
│   ├── application.go         # Application builder (hand-written)
│   ├── types.go               # Core interfaces (hand-written)
│   ├── component/
│   │   ├── webservice/        # Generated per component type
│   │   │   ├── webservice.go  # Fluent builder (generated)
│   │   │   ├── types.go       # Parameter types (generated)
│   │   │   ├── version.go     # Version metadata (generated)
│   │   │   └── webservice_test.go # Tests (generated)
│   │   ├── worker/
│   │   └── task/
│   ├── trait/
│   │   ├── scaler/
│   │   ├── ingress/
│   │   └── gateway/
│   ├── policy/
│   │   └── topology/
│   └── workflow/
│       ├── deploy/
│       └── notification/
├── client/
│   └── client.go              # K8s client wrapper (hand-written)
├── util/
│   └── ptr.go                 # Helper utilities (hand-written)
├── test/
│   ├── integration/           # Integration tests (generated)
│   └── e2e/                   # E2E tests (generated)
└── examples/                  # Usage examples
```

**Note:** Files marked as "(generated)" are auto-generated by `make generate-sdk` and should not be edited manually. Files marked as "(hand-written)" are maintained by developers.

### Fluent Builder Implementation

```go
// pkg/apis/component/webservice/webservice.go

package webservice

import "github.com/oam-dev/kubevela/pkg/sdk/apis"

const WebserviceType = "webservice"

type WebserviceSpec struct {
    Base       apis.ComponentBase
    Properties WebserviceProperties
}

type WebserviceProperties struct {
    Image    string  `json:"image"`
    Port     *int    `json:"port,omitempty"`
    Replicas *int    `json:"replicas,omitempty"`
    Cpu      *string `json:"cpu,omitempty"`
    Memory   *string `json:"memory,omitempty"`
    Cmd      []string `json:"cmd,omitempty"`
    Env      []EnvVar `json:"env,omitempty"`
}

// New creates a new webservice component builder
func New(name string) *WebserviceSpec {
    return &WebserviceSpec{
        Base: apis.ComponentBase{
            Name: name,
            Type: WebserviceType,
        },
    }
}

// Required parameter - no pointer, returns *WebserviceSpec for chaining
func (w *WebserviceSpec) Image(image string) *WebserviceSpec {
    w.Properties.Image = image
    return w
}

// Optional parameter with default - pointer type
func (w *WebserviceSpec) Port(port int) *WebserviceSpec {
    w.Properties.Port = &port
    return w
}

func (w *WebserviceSpec) Replicas(replicas int) *WebserviceSpec {
    w.Properties.Replicas = &replicas
    return w
}

func (w *WebserviceSpec) Cpu(cpu string) *WebserviceSpec {
    w.Properties.Cpu = &cpu
    return w
}

func (w *WebserviceSpec) Memory(memory string) *WebserviceSpec {
    w.Properties.Memory = &memory
    return w
}

// AddTrait adds a trait with compatibility validation
func (w *WebserviceSpec) AddTrait(trait apis.Trait) *WebserviceSpec {
    w.Base.Traits = append(w.Base.Traits, trait)
    return w
}

// Build converts to ApplicationComponent
func (w *WebserviceSpec) Build() common.ApplicationComponent {
    return common.ApplicationComponent{
        Name:       w.Base.Name,
        Type:       w.Base.Type,
        Properties: util.Object2RawExtension(w.Properties),
        Traits:     w.buildTraits(),
        DependsOn:  w.Base.DependsOn,
    }
}

// Validate checks required fields and trait compatibility
func (w *WebserviceSpec) Validate() error {
    if w.Properties.Image == "" {
        return fmt.Errorf("webservice %q: image is required", w.Base.Name)
    }
    // Validate trait compatibility
    for i, trait := range w.Base.Traits {
        if err := trait.Validate(); err != nil {
            return fmt.Errorf("trait[%d] %s: %w", i, trait.DefType(), err)
        }
        if checker, ok := trait.(apis.TraitCompatibilityChecker); ok {
            if !checker.IsCompatibleWith(WebserviceType) {
                return fmt.Errorf("trait %s is not compatible with component type %s",
                    trait.DefType(), WebserviceType)
            }
        }
    }
    return nil
}
```

### Trait Compatibility Validation

Traits in CUE definitions specify which component types they can be applied to via `appliesToWorkloads`. The SDK preserves this information and validates at runtime to catch invalid combinations early.

```go
// pkg/apis/trait/scaler/scaler.go

package scaler

// Generated compatibility metadata from CUE definition
var compatibleWorkloads = []string{"webservice", "worker", "statefulset"}

// IsCompatibleWith checks if this trait can be applied to the given component type
func (s *ScalerSpec) IsCompatibleWith(componentType string) bool {
    for _, w := range compatibleWorkloads {
        if w == componentType {
            return true
        }
    }
    return false
}

// CompatibleWorkloads returns the list of component types this trait supports
func (s *ScalerSpec) CompatibleWorkloads() []string {
    return compatibleWorkloads
}
```

### Discriminated Union (OneOf) Handling

CUE definitions often use discriminated unions with `close({...}) | close({...})` patterns. The SDK generates type-safe union types with:

1. **Variant enum type** - Typed constants for variant names (not raw strings)
2. **Variant constructor functions** - Package-level functions to construct each variant
3. **Typed accessors** - Methods to safely extract the underlying variant
4. **JSON marshaling** - Custom serialization matching CUE's output format

**Generated code for specific union types:**

```go
// Example: Generated from notification.cue lark.url field
// CUE: url: close({ value: string }) | close({ secretRef: { name: string, key: string } })

// --- Variant Enum (type-safe variant identification) ---

type LarkUrlVariant string

const (
    LarkUrlVariantUnset     LarkUrlVariant = ""
    LarkUrlVariantValue     LarkUrlVariant = "value"
    LarkUrlVariantSecretRef LarkUrlVariant = "secretRef"
)

// AllLarkUrlVariants returns all valid variants (useful for validation/docs)
func AllLarkUrlVariants() []LarkUrlVariant {
    return []LarkUrlVariant{LarkUrlVariantValue, LarkUrlVariantSecretRef}
}

// --- Variant types (internal, not directly constructed by users) ---

type larkUrlValue struct {
    Value string `json:"value"`
}

type larkUrlSecretRef struct {
    Name string `json:"name"`
    Key  string `json:"key"`
}

// --- Union Type ---

// LarkUrl is a discriminated union type
type LarkUrl struct {
    variant LarkUrlVariant
    value   any
}

// --- Variant Constructors (package-level functions) ---

// LarkUrlValue creates a LarkUrl with a direct value
func LarkUrlValue(v string) LarkUrl {
    return LarkUrl{variant: LarkUrlVariantValue, value: larkUrlValue{Value: v}}
}

// LarkUrlSecretRef creates a LarkUrl with a secret reference
func LarkUrlSecretRef(name, key string) LarkUrl {
    return LarkUrl{variant: LarkUrlVariantSecretRef, value: larkUrlSecretRef{Name: name, Key: key}}
}

// --- Typed Accessors (avoid raw `any` in user code) ---

// Variant returns which variant is set (typed enum, not string)
func (u LarkUrl) Variant() LarkUrlVariant { return u.variant }

// IsSet returns true if a variant has been selected
func (u LarkUrl) IsSet() bool { return u.variant != LarkUrlVariantUnset }

// IsValue returns true if this is the "value" variant
func (u LarkUrl) IsValue() bool { return u.variant == LarkUrlVariantValue }

// IsSecretRef returns true if this is the "secretRef" variant
func (u LarkUrl) IsSecretRef() bool { return u.variant == LarkUrlVariantSecretRef }

// AsValue returns the value if this is a "value" variant
func (u LarkUrl) AsValue() (string, bool) {
    if u.variant != LarkUrlVariantValue {
        return "", false
    }
    v, ok := u.value.(larkUrlValue)
    return v.Value, ok
}

// AsSecretRef returns the secret reference if this is a "secretRef" variant
func (u LarkUrl) AsSecretRef() (name string, key string, ok bool) {
    if u.variant != LarkUrlVariantSecretRef {
        return "", "", false
    }
    v, ok := u.value.(larkUrlSecretRef)
    return v.Name, v.Key, ok
}

// --- Validation ---

func (u LarkUrl) Validate() error {
    switch u.variant {
    case LarkUrlVariantUnset:
        return errors.New("LarkUrl: no variant selected")
    case LarkUrlVariantValue, LarkUrlVariantSecretRef:
        return nil
    default:
        return fmt.Errorf("LarkUrl: unknown variant %q", u.variant)
    }
}

// --- JSON Marshaling (matches CUE output format) ---

func (u LarkUrl) MarshalJSON() ([]byte, error) {
    switch u.variant {
    case LarkUrlVariantValue:
        v, _ := u.value.(larkUrlValue)
        return json.Marshal(map[string]any{"value": v.Value})
    case LarkUrlVariantSecretRef:
        s, _ := u.value.(larkUrlSecretRef)
        return json.Marshal(map[string]any{
            "secretRef": map[string]string{"name": s.Name, "key": s.Key},
        })
    default:
        return nil, fmt.Errorf("LarkUrl: unknown variant %q", u.variant)
    }
}

func (u *LarkUrl) UnmarshalJSON(data []byte) error {
    var raw map[string]json.RawMessage
    if err := json.Unmarshal(data, &raw); err != nil {
        return err
    }

    if v, ok := raw["value"]; ok {
        var val string
        if err := json.Unmarshal(v, &val); err != nil {
            return err
        }
        *u = LarkUrlValue(val)
        return nil
    }

    if s, ok := raw["secretRef"]; ok {
        var ref struct {
            Name string `json:"name"`
            Key  string `json:"key"`
        }
        if err := json.Unmarshal(s, &ref); err != nil {
            return err
        }
        *u = LarkUrlSecretRef(ref.Name, ref.Key)
        return nil
    }

    return errors.New("LarkUrl: no recognized variant in JSON")
}
```

**Usage:**

```go
// Construct using variant functions (clean, type-safe)
notif := notification.New("alert").
    SetLark(notification.LarkWith{
        Url: notification.LarkUrlValue("https://open.larksuite.com/..."),
    })

// Or with secret reference
notif := notification.New("alert").
    SetLark(notification.LarkWith{
        Url: notification.LarkUrlSecretRef("lark-secret", "webhook-url"),
    })

// Reading back (type-safe accessors)
if val, ok := larkConfig.Url.AsValue(); ok {
    fmt.Println("Direct URL:", val)
}
if name, key, ok := larkConfig.Url.AsSecretRef(); ok {
    fmt.Printf("Secret: %s/%s\n", name, key)
}

// Switch on variant (compile-time exhaustiveness with linter)
switch larkConfig.Url.Variant() {
case notification.LarkUrlVariantValue:
    val, _ := larkConfig.Url.AsValue()
    fmt.Println("Direct:", val)
case notification.LarkUrlVariantSecretRef:
    name, key, _ := larkConfig.Url.AsSecretRef()
    fmt.Printf("Secret: %s/%s\n", name, key)
case notification.LarkUrlVariantUnset:
    fmt.Println("Not configured")
}

// Boolean checks for simple conditionals
if larkConfig.Url.IsSecretRef() {
    // handle secret lookup
}
```

**Design Rationale:**

| Concern | Solution |
|---------|----------|
| Variant identification | Typed enum (`LarkUrlVariant`) instead of raw strings; enables exhaustive switch checking |
| Type safety with `any` | Internal variant types are unexported; users interact via typed constructors and accessors |
| API ergonomics | Package-level constructor functions + `IsX()` boolean helpers for simple checks |
| JSON serialization | Custom `MarshalJSON`/`UnmarshalJSON` matching CUE's discriminated union format |
| IDE discoverability | Enum constants, constructor functions, and accessor methods all appear in autocomplete |

### Validation Lifecycle

Validation occurs **automatically** when using `vela apply` (see [Lifecycle-Based Deployment](#lifecycle-based-deployment)). For power users writing custom `main()`, the `Validate()` method can be called at various points:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Validation Entry Points                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  CLI-DRIVEN (recommended - validation is automatic)                          │
│  ─────────────────────────────────────────────────                           │
│  $ vela apply myapp.go      ◄── Automatic: SDK validation + custom Validate()│
│  $ vela validate myapp.go   ◄── Validation only, no apply                   │
│                                                                              │
│  PROGRAMMATIC (for custom main() users)                                      │
│  ──────────────────────────────────────                                      │
│  app.Validate()             ◄── Explicit call, returns error                │
│  app.Build()                ◄── Calls Validate() internally                 │
│  app.ToYAML()               ◄── Calls Validate() internally                 │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Validation is recursive:**

```go
func (a *ApplicationBuilder) Validate() error {
    if a.name == "" {
        return errors.New("application name is required")
    }
    if a.namespace == "" {
        return errors.New("namespace is required")
    }

    // Validate all components (recursive)
    for _, c := range a.components {
        if err := c.Validate(); err != nil {
            return fmt.Errorf("component %q: %w", c.ComponentName(), err)
        }
    }

    // Validate all traits within components
    for _, c := range a.components {
        for _, t := range c.GetAllTraits() {
            if err := t.Validate(); err != nil {
                return fmt.Errorf("component %q trait %q: %w", c.ComponentName(), t.DefType(), err)
            }
        }
    }

    // Validate policies, workflow steps, OneOf fields...
    // Each calls Validate() on nested structures
    return nil
}
```

**When validation runs:**

| Entry Point | SDK Validation | Custom Validate() | Behavior on Error |
|-------------|----------------|-------------------|-------------------|
| `vela apply myapp.go` | Automatic | Automatic (if defined) | Stops, no apply |
| `vela validate myapp.go` | Automatic | Automatic (if defined) | Reports error |
| `app.Validate()` | Yes | No (call separately) | Returns error |
| `app.Build()` | Yes | No | Returns `(Application, error)` |
| `app.ToYAML()` | Yes | No | Returns `(string, error)` |

**Custom validation (for business rules):**

User-defined `Validate(app)` function runs **after** automatic SDK validation passes:

```go
// In myapp.go - this is called by vela CLI after SDK validation
func Validate(app *sdk.Application) error {
    // Custom business rules beyond schema validation
    if app.GetNamespace() == "production" {
        for _, c := range app.GetComponentsByType("webservice") {
            if c.GetReplicas() < 3 {
                return fmt.Errorf("component %q: production requires at least 3 replicas", c.ComponentName())
            }
        }
    }
    return nil
}
```

### Cross-Field Validation for Conditional Requirements

CUE definitions often have fields that are required only when another field has a specific value. The SDK generates validation logic for these patterns.

```go
// Example: Generated from dingtalk message definition
// CUE: msgtype: "text" | "link" | "markdown"
//      if msgtype == "text" { text: { content: string } }

type DingdingMessage struct {
    Msgtype    string      `json:"msgtype"`
    Text       *TextMsg    `json:"text,omitempty"`
    Link       *LinkMsg    `json:"link,omitempty"`
    Markdown   *MarkdownMsg `json:"markdown,omitempty"`
}

func (m *DingdingMessage) Validate() error {
    switch m.Msgtype {
    case "text":
        if m.Text == nil {
            return errors.New("text is required when msgtype is 'text'")
        }
        if m.Link != nil || m.Markdown != nil {
            return errors.New("only text should be set when msgtype is 'text'")
        }
    case "link":
        if m.Link == nil {
            return errors.New("link is required when msgtype is 'link'")
        }
    case "markdown":
        if m.Markdown == nil {
            return errors.New("markdown is required when msgtype is 'markdown'")
        }
    default:
        return fmt.Errorf("invalid msgtype: %s", m.Msgtype)
    }
    return nil
}
```

### Context Variables: SDK Scope vs Controller Scope

CUE definitions use `context.*` variables that are injected by the KubeVela controller at runtime. These variables cannot be controlled from the SDK because their values depend on the actual runtime environment.

| Context Variable               | Description                   | SDK Control            |
| ------------------------------ | ----------------------------- | ---------------------- |
| `context.name`                 | Component name                | Set via Application CR |
| `context.namespace`            | Target namespace              | Set via Application CR |
| `context.appName`              | Application name              | Set via Application CR |
| `context.clusterVersion.minor` | Target cluster K8s version    | Runtime only           |
| `context.outputs.*`            | Previously rendered resources | Runtime only           |
| `context.appRevision`          | Application revision string   | Runtime only           |

**What this means for SDK users:**

CUE template conditionals based on `parameter.*` can be "triggered" by setting parameter values in the SDK. However, conditionals based on `context.*` (like K8s version checks) execute in the controller based on the actual target environment.

```go
// Example: This CUE conditional is NOT controllable from SDK
// if context.clusterVersion.minor < 25 { apiVersion: "batch/v1beta1" }
// The controller determines the K8s version at runtime

// Example: This CUE conditional IS controllable from SDK
// if parameter.cpu != _|_ { resources: limits: cpu: parameter.cpu }
// Setting Cpu("500m") in SDK triggers this conditional
comp := webservice.New("app").
    Image("nginx").
    Cpu("500m")  // This triggers the conditional in CUE template
```

### SDK Generation Commands

```bash
# Generate SDK from cluster definitions (default namespace)
vela sdk generate --from-cluster --output ./sdk

# Generate from specific namespace
vela sdk generate --from-cluster --namespace platform-defs --output ./sdk

# Generate from local CUE files
vela sdk generate --from-file ./definitions/*.cue --output ./sdk

# Generate from OCI registry
vela sdk generate --from-oci oci://registry.example.com/platform/defs:v1.0 --output ./sdk

# Generate from KubeVela release (for core definitions)
vela sdk generate --from-release v1.9.0 --output ./sdk

# Include only specific definition types
vela sdk generate --from-cluster --types component,trait --output ./sdk

# Generate with custom package name
vela sdk generate --from-cluster --package myplatform/sdk --output ./sdk
```

## In-Repo SDK Generation and CI/CD

The core SDK is generated automatically as part of the KubeVela build process. This ensures the SDK is always synchronized with the definitions shipped in each release.

### Makefile Targets

```makefile
# Makefile (in kubevela repo root)

.PHONY: generate-sdk test-sdk verify-sdk

# Generate SDK from vela-templates/definitions
generate-sdk:
 @echo "Generating SDK from core definitions..."
 go run ./cmd/vela/main.go sdk generate \
  --from-file ./vela-templates/definitions/internal/... \
  --output ./pkg/sdk/apis \
  --package github.com/oam-dev/kubevela/pkg/sdk/apis \
  --generate-tests
 @echo "Running go fmt on generated code..."
 go fmt ./pkg/sdk/...
 @echo "SDK generation complete."

# Run generated SDK tests
test-sdk:
 @echo "Running SDK tests..."
 go test -v ./pkg/sdk/...

# Verify SDK is up-to-date (fails if generation would change files)
verify-sdk: generate-sdk
 @echo "Verifying SDK is up-to-date..."
 @if [ -n "$$(git status --porcelain ./pkg/sdk/)" ]; then \
  echo "ERROR: SDK is out of sync with definitions."; \
  echo "Run 'make generate-sdk' and commit the changes."; \
  git diff ./pkg/sdk/; \
  exit 1; \
 fi
 @echo "SDK is up-to-date."

# Full SDK workflow
sdk: generate-sdk test-sdk
```

### GitHub Actions Workflow

```yaml
# .github/workflows/sdk.yaml
name: SDK Generation and Tests

on:
  push:
    branches: [master, release-*]
    paths:
      - "vela-templates/definitions/**"
      - "pkg/sdk/**"
      - "pkg/definition/gen_sdk/**"
  pull_request:
    paths:
      - "vela-templates/definitions/**"
      - "pkg/sdk/**"
      - "pkg/definition/gen_sdk/**"

jobs:
  generate-and-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: "1.21"

      - name: Generate SDK
        run: make generate-sdk

      - name: Verify SDK is up-to-date
        run: |
          if [ -n "$(git status --porcelain ./pkg/sdk/)" ]; then
            echo "::error::SDK is out of sync with definitions!"
            echo "Please run 'make generate-sdk' locally and commit the changes."
            git diff ./pkg/sdk/
            exit 1
          fi

      - name: Run SDK unit tests
        run: make test-sdk

      - name: Run SDK integration tests
        run: |
          # Setup envtest
          make envtest-setup
          go test -v -tags=integration ./pkg/sdk/test/integration/...

  # Runs on release branches to ensure SDK works with released definitions
  release-validation:
    if: startsWith(github.ref, 'refs/heads/release-')
    runs-on: ubuntu-latest
    needs: generate-and-test
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: "1.21"

      - name: Build SDK examples
        run: |
          cd pkg/sdk/examples
          go build ./...

      - name: Run E2E SDK tests
        run: |
          # Setup kind cluster with KubeVela
          make kind-setup
          make vela-install
          # Run E2E tests
          go test -v -tags=e2e ./pkg/sdk/test/e2e/...
```

### Auto-Generated Tests

The SDK generator produces comprehensive tests for each definition. These tests are committed to the repository and run in CI.

#### Generated Test Structure

```go
// pkg/sdk/apis/component/webservice/webservice_test.go
// Code generated by vela sdk generate. DO NOT EDIT.

package webservice_test

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/oam-dev/kubevela/pkg/sdk/apis/component/webservice"
)

func TestWebservice(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Webservice Component SDK Suite")
}

var _ = Describe("Webservice Component", func() {

    Describe("Builder", func() {
        It("should create a valid component with required fields", func() {
            comp := webservice.New("my-app").
                Image("nginx:1.21")

            Expect(comp.ComponentName()).To(Equal("my-app"))
            Expect(comp.DefType()).To(Equal("webservice"))

            err := comp.Validate()
            Expect(err).NotTo(HaveOccurred())
        })

        It("should fail validation when required field 'image' is missing", func() {
            comp := webservice.New("my-app")
            // Image not set

            err := comp.Validate()
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("image"))
        })
    })

    Describe("Optional Fields", func() {
        It("should accept optional port", func() {
            comp := webservice.New("my-app").
                Image("nginx:1.21").
                Port(8080)

            built := comp.Build()
            Expect(string(built.Properties.Raw)).To(ContainSubstring(`"port":8080`))
        })

        It("should use default port when not specified", func() {
            comp := webservice.New("my-app").
                Image("nginx:1.21")

            built := comp.Build()
            // Default port (80) should be in properties or omitted
            // depending on definition behavior
        })

        It("should accept optional cpu and memory", func() {
            comp := webservice.New("my-app").
                Image("nginx:1.21").
                Cpu("500m").
                Memory("512Mi")

            built := comp.Build()
            Expect(string(built.Properties.Raw)).To(ContainSubstring(`"cpu":"500m"`))
            Expect(string(built.Properties.Raw)).To(ContainSubstring(`"memory":"512Mi"`))
        })
    })

    Describe("Validation Constraints", func() {
        // Generated from parameter constraints in CUE

        It("should validate replicas minimum", func() {
            comp := webservice.New("my-app").
                Image("nginx:1.21").
                Replicas(0) // Below minimum of 1

            err := comp.Validate()
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("replicas"))
        })

        It("should validate replicas maximum", func() {
            comp := webservice.New("my-app").
                Image("nginx:1.21").
                Replicas(101) // Above maximum of 100

            err := comp.Validate()
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("replicas"))
        })

        It("should accept valid replicas", func() {
            comp := webservice.New("my-app").
                Image("nginx:1.21").
                Replicas(50)

            err := comp.Validate()
            Expect(err).NotTo(HaveOccurred())
        })
    })

    Describe("Build Output", func() {
        It("should produce valid ApplicationComponent", func() {
            comp := webservice.New("my-app").
                Image("nginx:1.21").
                Port(8080).
                Replicas(3)

            built := comp.Build()

            Expect(built.Name).To(Equal("my-app"))
            Expect(built.Type).To(Equal("webservice"))
            Expect(built.Properties).NotTo(BeNil())
        })
    })

    Describe("Version Compatibility", func() {
        It("should have version metadata", func() {
            Expect(webservice.DefinitionVersion).NotTo(BeEmpty())
            Expect(webservice.DefinitionHash).NotTo(BeEmpty())
            Expect(webservice.MinControllerVersion).NotTo(BeEmpty())
        })
    })
})
```

#### Test Generation Template

The generator uses templates to create tests:

```go
// pkg/definition/gen_sdk/templates/component_test.go.tmpl

package {{.PackageName}}_test

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "{{.ImportPath}}"
)

func Test{{.TypeName}}(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "{{.TypeName}} Component SDK Suite")
}

var _ = Describe("{{.TypeName}} Component", func() {

    Describe("Builder", func() {
        It("should create a valid component with required fields", func() {
            comp := {{.PackageName}}.New("test").
                {{range .RequiredFields}}{{.SetterName}}({{.ExampleValue}}).
                {{end}}

            Expect(comp.ComponentName()).To(Equal("test"))
            err := comp.Validate()
            Expect(err).NotTo(HaveOccurred())
        })

        {{range .RequiredFields}}
        It("should fail validation when required field '{{.Name}}' is missing", func() {
            comp := {{$.PackageName}}.New("test")
            {{range $.RequiredFields}}{{if ne .Name $.Name}}.{{.SetterName}}({{.ExampleValue}}){{end}}{{end}}

            err := comp.Validate()
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("{{.Name}}"))
        })
        {{end}}
    })

    {{if .OptionalFields}}
    Describe("Optional Fields", func() {
        {{range .OptionalFields}}
        It("should accept optional {{.Name}}", func() {
            comp := {{$.PackageName}}.New("test").
                {{range $.RequiredFields}}{{.SetterName}}({{.ExampleValue}}).
                {{end}}{{.SetterName}}({{.ExampleValue}})

            built := comp.Build()
            Expect(string(built.Properties.Raw)).To(ContainSubstring(`"{{.JSONName}}"`))
        })
        {{end}}
    })
    {{end}}

    {{if .ValidationConstraints}}
    Describe("Validation Constraints", func() {
        {{range .ValidationConstraints}}
        It("should validate {{.FieldName}} {{.ConstraintType}}", func() {
            comp := {{$.PackageName}}.New("test").
                {{range $.RequiredFields}}{{.SetterName}}({{.ExampleValue}}).
                {{end}}{{.SetterName}}({{.InvalidValue}})

            err := comp.Validate()
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("{{.FieldName}}"))
        })
        {{end}}
    })
    {{end}}
})
```

### Integration Test Generation

For each definition, the generator also creates integration tests that verify the SDK works correctly with envtest:

```go
// pkg/sdk/test/integration/webservice_integration_test.go
// Code generated by vela sdk generate. DO NOT EDIT.
//go:build integration

package integration_test

import (
    "context"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/oam-dev/kubevela/pkg/sdk"
    "github.com/oam-dev/kubevela/pkg/sdk/apis/component/webservice"
)

var _ = Describe("Webservice Integration", func() {

    It("should create Application CR with webservice component", func() {
        ctx := context.Background()

        app := sdk.NewApplication("test-webservice").
            Namespace(testNamespace).
            AddComponent(
                webservice.New("test").
                    Image("nginx:1.21").
                    Port(80),
            )

        err := velaClient.Apply(ctx, app)
        Expect(err).NotTo(HaveOccurred())

        // Verify Application was created
        retrieved, err := velaClient.Get(ctx, "test-webservice", testNamespace)
        Expect(err).NotTo(HaveOccurred())
        Expect(retrieved).NotTo(BeNil())

        // Cleanup
        err = velaClient.Delete(ctx, app)
        Expect(err).NotTo(HaveOccurred())
    })
})
```

### Release Process Integration

The SDK generation is integrated into the release process:

```yaml
# .github/workflows/release.yaml (excerpt)

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      # ... other release steps ...

      - name: Generate and verify SDK
        run: |
          make generate-sdk
          make verify-sdk
          make test-sdk

      - name: Build release artifacts
        run: |
          # SDK is already in pkg/sdk/, included in main module
          make build

      - name: Tag release
        run: |
          git tag v${{ github.event.inputs.version }}
          git push origin v${{ github.event.inputs.version }}
```

### Importing the Core SDK

Users import the core SDK directly from the KubeVela module:

```go
import (
    // Core SDK - versioned with KubeVela releases
    "github.com/oam-dev/kubevela/pkg/sdk"
    "github.com/oam-dev/kubevela/pkg/sdk/apis/component/webservice"
    "github.com/oam-dev/kubevela/pkg/sdk/apis/trait/scaler"
    "github.com/oam-dev/kubevela/pkg/sdk/apis/policy/topology"
    "github.com/oam-dev/kubevela/pkg/sdk/apis/workflow/deploy"
)

// go.mod
module myapp

require (
    github.com/oam-dev/kubevela v1.9.0  // SDK included
)
```

### SDK Versioning with KubeVela Releases

| KubeVela Version | SDK Version    | Compatibility                        |
| ---------------- | -------------- | ------------------------------------ |
| v1.9.0           | v1.9.0 (same)  | Requires KubeVela controller v1.9.0+ |
| v1.9.1           | v1.9.1 (same)  | Patch release, backwards compatible  |
| v1.10.0          | v1.10.0 (same) | May have new definitions/fields      |

**Version guarantees:**

- SDK version always matches KubeVela release version
- Patch releases (v1.9.x) are backwards compatible
- Minor releases (v1.x.0) may add new definitions/fields
- Major releases may have breaking changes (following semver)

### Custom Definition SDK Workflow

A key question: **How do users get fluent APIs for custom definitions they create?**

The answer is **SDK generation**. The fluent APIs are not manually written - they are generated from the X-Definition's parameter schema.

#### Example: Custom AWS RDS Component

**Step 1: Platform engineer creates the definition (CUE or defkit)**

```cue
// aws-rds.cue
"aws-rds": {
    type: "component"
    description: "Provision AWS RDS via Crossplane"
}
template: {
    parameter: {
        engine:        *"postgres" | "mysql" | "mariadb"
        instanceClass: *"db.t3.micro" | string
        storageGB:     *20 | int & >=20 & <=65536
        multiAZ:       *false | bool
    }
    output: {
        apiVersion: "database.aws.crossplane.io/v1beta1"
        kind:       "DBInstance"
        // ...
    }
}
```

**Step 2: Apply the definition to the cluster**

```bash
vela def apply ./aws-rds.cue
```

**Step 3: Generate SDK including the custom definition**

```bash
# Generate SDK from cluster (includes both core and custom definitions)
vela sdk generate --from-cluster --output ./my-sdk --package myplatform/sdk
```

**Step 4: Use the generated SDK**

```go
import (
    sdk "github.com/oam-dev/kubevela/pkg/sdk"
    "myplatform/sdk/pkg/apis/component/awsrds"  // Generated!
)

app := sdk.NewApplication("my-app").
    AddComponent(
        awsrds.New("production-db").
            Engine("postgres").           // Generated method
            InstanceClass("db.t3.large"). // Generated method
            StorageGB(100).               // Generated method
            MultiAZ(true),                // Generated method
    )
```

#### Generated Code Example

For the `aws-rds` definition above, the generator produces:

```go
// my-sdk/pkg/apis/component/awsrds/awsrds.go (GENERATED)
package awsrds

import "myplatform/sdk/pkg/apis"

const AwsRdsType = "aws-rds"

type AwsRdsSpec struct {
    Base       apis.ComponentBase
    Properties AwsRdsProperties
}

type AwsRdsProperties struct {
    Engine        *string `json:"engine,omitempty"`
    InstanceClass *string `json:"instanceClass,omitempty"`
    StorageGB     *int    `json:"storageGB,omitempty"`
    MultiAZ       *bool   `json:"multiAZ,omitempty"`
}

func New(name string) *AwsRdsSpec {
    return &AwsRdsSpec{
        Base: apis.ComponentBase{Name: name, Type: AwsRdsType},
    }
}

func (a *AwsRdsSpec) Engine(engine string) *AwsRdsSpec {
    a.Properties.Engine = &engine
    return a
}

func (a *AwsRdsSpec) InstanceClass(class string) *AwsRdsSpec {
    a.Properties.InstanceClass = &class
    return a
}

func (a *AwsRdsSpec) StorageGB(size int) *AwsRdsSpec {
    a.Properties.StorageGB = &size
    return a
}

func (a *AwsRdsSpec) MultiAZ(enabled bool) *AwsRdsSpec {
    a.Properties.MultiAZ = &enabled
    return a
}

func (a *AwsRdsSpec) Build() common.ApplicationComponent { ... }
func (a *AwsRdsSpec) Validate() error { ... }

// Auto-registration with namespaced key to avoid conflicts
func init() {
    common.RegisterComponent("myplatform/aws-rds", FromComponent)
}
```

#### Namespaced Builder Registration

When combining core SDK with custom SDK, builder registration uses namespaced keys to prevent conflicts:

```go
// Core SDK registers with core.oam.dev namespace
common.RegisterComponent("core.oam.dev/webservice", FromComponent)

// Custom SDK registers with custom namespace
common.RegisterComponent("myplatform/webservice", FromComponent)  // Extended version
common.RegisterComponent("myplatform/aws-rds", FromComponent)

// Lookup checks exact match first, then falls back to short name
func GetComponentBuilder(typeName string) ComponentBuilder {
    // Exact match
    if b, ok := builders[typeName]; ok {
        return b
    }
    // Short name fallback (for backwards compatibility)
    for k, b := range builders {
        if strings.HasSuffix(k, "/"+typeName) {
            return b
        }
    }
    return nil
}
```

#### Combining Core and Custom SDKs

Users typically have two SDK sources:

```go
// go.mod
module myapp

require (
    // Core KubeVela SDK (in-repo, matches KubeVela version)
    github.com/oam-dev/kubevela v1.9.0

    // Custom platform SDK (generated from your definitions)
    myplatform/sdk v1.2.0
)
```

```go
import (
    sdk "github.com/oam-dev/kubevela/pkg/sdk"

    // Core definitions (in-repo)
    "github.com/oam-dev/kubevela/pkg/sdk/apis/component/webservice"
    "github.com/oam-dev/kubevela/pkg/sdk/apis/trait/scaler"

    // Custom definitions (your platform's generated SDK)
    "myplatform/sdk/pkg/apis/component/awsrds"
    "myplatform/sdk/pkg/apis/trait/backup"
)

app := sdk.NewApplication("my-app").
    AddComponent(
        webservice.New("api").Image("myapp:v1"),
    ).
    AddComponent(
        awsrds.New("database").Engine("postgres"),
    )
```

#### SDK Regeneration Workflow

When definitions change, regenerate the SDK:

```bash
# After updating aws-rds.cue and applying it
vela def apply ./aws-rds.cue

# Regenerate SDK to pick up changes
vela sdk generate --from-cluster --output ./my-sdk

# Commit and tag the new SDK version
cd my-sdk
git add .
git commit -m "Update SDK for aws-rds v2 changes"
git tag v1.3.0
git push --tags
```

#### Watch Mode for Development

During development, use watch mode to auto-regenerate:

```bash
# Watch cluster for definition changes, regenerate SDK automatically
vela sdk develop --from-cluster --output ./my-sdk --watch
```

### Versioning Strategy

The SDK follows the same versioning model as CUE definitions - the controller manages `DefinitionRevision` objects that track changes. SDK packages include metadata about which revision they were generated from:

```go
// pkg/apis/component/webservice/version.go (GENERATED)
package webservice

const (
    // DefinitionName is the X-Definition this SDK was generated from
    DefinitionName = "webservice"

    // DefinitionRevision matches the DefinitionRevision.spec.revision
    DefinitionRevision = 3

    // GeneratedAt timestamp
    GeneratedAt = "2024-01-15T10:00:00Z"
)
```

**How it works (same as CUE):**

1. User generates SDK from cluster: `vela sdk generate --from-cluster`
2. SDK generator reads `ComponentDefinition` and its current `DefinitionRevision`
3. Generated code includes the revision number for documentation purposes
4. At runtime, the controller uses the definition currently in the cluster (same as CUE)

**Version alignment:**

1. **Core SDK**: Aligned with KubeVela releases (e.g., `v1.9.0`, `v1.10.0`) - definitions ship with KubeVela
2. **Custom Definitions**: Regenerate SDK when platform definitions change

```go
// go.mod
module myplatform

require (
    github.com/oam-dev/kubevela v1.9.0  // Core SDK is in-repo
)
```

> **Note:** Unlike CUE where the controller evaluates the definition at runtime, the SDK generates parameter values that the controller validates against the *current* cluster definition. If the definition schema changes incompatibly, the controller will reject the Application with a validation error - the same behavior as submitting stale YAML.

### Validation

The SDK provides compile-time and runtime validation:

**Compile-time:**

- Required parameters enforced by method signatures
- Type checking for parameter values
- Enum validation via type constraints

**Runtime:**

- `Validate()` method on all builders
- Cross-field validation rules (conditional requirements)
- Trait compatibility checks
- OneOf variant validation

```go
// Validation example
app := sdk.NewApplication("test").
    AddComponent(
        webservice.New("frontend"), // Missing Image - caught at runtime
    )

if err := app.Validate(); err != nil {
    // Error: webservice "frontend": image is required
}
```

### Round-Trip Fidelity

When converting existing Applications to SDK objects and back, unknown fields are preserved:

```go
// Load existing application that may have fields not in SDK schema
app, _ := sdk.FromYAML(existingYAML)

// Modify using SDK
app.GetComponentByName("web").(*webservice.WebserviceSpec).Replicas(5)

// Unknown fields in properties are preserved
yaml, _ := app.ToYAML()  // customField still present
```

Implementation uses `json.RawMessage` for unknown field preservation:

```go
type WebserviceProperties struct {
    Image    string  `json:"image"`
    Port     *int    `json:"port,omitempty"`
    // ... known fields

    // Preserve fields not in SDK schema for round-trip fidelity
    unknownFields map[string]json.RawMessage
}
```

### Client Operations

```go
// pkg/client/client.go

type Client interface {
    // CRUD operations
    Apply(ctx context.Context, app apis.TypedApplication) error
    Create(ctx context.Context, app apis.TypedApplication) error
    Update(ctx context.Context, app apis.TypedApplication) error
    Delete(ctx context.Context, app apis.TypedApplication) error
    Get(ctx context.Context, name, namespace string) (apis.TypedApplication, error)
    List(ctx context.Context, namespace string) ([]apis.TypedApplication, error)

    // Status operations
    GetStatus(ctx context.Context, name, namespace string) (*ApplicationStatus, error)
    WaitForHealthy(ctx context.Context, name, namespace string, timeout time.Duration) error

    // Version compatibility check
    CheckDefinitionCompatibility(ctx context.Context, defType, defName string) error
}

func NewClient(config *rest.Config) (Client, error)
func NewClientFromKubeconfig(kubeconfigPath string) (Client, error)
```

## SDK Deployment Workflow

This section describes the complete lifecycle of how Go code using the SDK gets built, tested, and deployed to a Kubernetes cluster.

### Workflow Overview

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                         SDK Application Lifecycle                                │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐    ┌──────────────┐   │
│  │  1. Write    │    │  2. Test     │    │  3. Build    │    │  4. Deploy   │   │
│  │              │    │              │    │              │    │              │   │
│  │ • Import SDK │───▶│ • Unit tests │───▶│ • go build   │───▶│ • Run binary │   │
│  │ • Define app │    │ • Validation │    │ • Container  │    │ • CI/CD      │   │
│  │ • Use fluent │    │ • Mock tests │    │              │    │ • GitOps     │   │
│  │   builders   │    │              │    │              │    │              │   │
│  └──────────────┘    └──────────────┘    └──────────────┘    └──────────────┘   │
│                                                                                  │
│                                            ▼                                     │
│                              ┌─────────────────────────────┐                     │
│                              │  5. Apply to Cluster        │                     │
│                              │                             │                     │
│                              │ • SDK Client.Apply()        │                     │
│                              │ • kubectl apply (YAML)      │                     │
│                              │ • vela up (CLI)             │                     │
│                              └─────────────────────────────┘                     │
│                                            │                                     │
│                                            ▼                                     │
│                              ┌─────────────────────────────┐                     │
│                              │  6. Controller Reconciles   │                     │
│                              │                             │                     │
│                              │ • CUE template evaluation   │                     │
│                              │ • Resource rendering        │                     │
│                              │ • Kubernetes object creation│                     │
│                              └─────────────────────────────┘                     │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Step 1: Write Application Code

Create a Go application using the SDK:

```go
// cmd/deploy-app/main.go
package main

import (
    "context"
    "log"
    "os"

    // Core SDK from KubeVela main module
    "github.com/oam-dev/kubevela/pkg/sdk"
    "github.com/oam-dev/kubevela/pkg/sdk/apis/component/webservice"
    "github.com/oam-dev/kubevela/pkg/sdk/apis/trait/scaler"
    "github.com/oam-dev/kubevela/pkg/sdk/client"
)

func main() {
    // Build the application using fluent API
    app := BuildApplication(os.Getenv("ENVIRONMENT"))

    // Validate before deployment
    if err := app.Validate(); err != nil {
        log.Fatalf("Validation failed: %v", err)
    }

    // Deploy to cluster
    if err := DeployApplication(context.Background(), app); err != nil {
        log.Fatalf("Deployment failed: %v", err)
    }

    log.Println("Application deployed successfully")
}

func BuildApplication(env string) sdk.TypedApplication {
    replicas := 3
    if env == "development" {
        replicas = 1
    }

    return sdk.NewApplication("my-app").
        Namespace(env).
        Labels(map[string]string{"team": "platform", "env": env}).
        AddComponent(
            webservice.New("api").
                Image("myregistry/api:v1.2.3").
                Port(8080).
                Replicas(replicas).
                Cpu("500m").
                Memory("512Mi").
                AddTrait(scaler.New().Min(replicas).Max(replicas * 3)),
        )
}

func DeployApplication(ctx context.Context, app sdk.TypedApplication) error {
    // Option 1: Use SDK client for programmatic deployment
    velaClient, err := client.NewClientFromKubeconfig(os.Getenv("KUBECONFIG"))
    if err != nil {
        return err
    }
    return velaClient.Apply(ctx, app)
}
```

### Step 2: Project Structure

```
my-deployment-tool/
├── cmd/
│   └── deploy-app/
│       └── main.go              # Entry point
├── internal/
│   └── apps/
│       ├── api.go               # API component builder
│       ├── worker.go            # Worker component builder
│       └── common.go            # Shared utilities
├── test/
│   ├── unit/
│   │   ├── api_test.go          # Unit tests
│   │   └── validation_test.go
│   └── integration/
│       └── deploy_test.go       # Integration tests (requires cluster)
├── go.mod
├── go.sum
└── Makefile
```

### Step 3: Build and Package

```makefile
# Makefile

.PHONY: build test deploy

# Build the deployment binary
build:
 go build -o bin/deploy-app ./cmd/deploy-app

# Build container image (for CI/CD)
docker-build:
 docker build -t myregistry/deploy-app:$(VERSION) .

# Run all tests
test: test-unit test-integration

test-unit:
 go test -v ./test/unit/...

test-integration:
 go test -v ./test/integration/... -tags=integration
```

```dockerfile
# Dockerfile
FROM golang:1.21 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /deploy-app ./cmd/deploy-app

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /deploy-app /deploy-app
ENTRYPOINT ["/deploy-app"]
```

### Step 4: Deployment Options

The SDK provides multiple ways to deploy your application. All paths ultimately result in an Application CR being applied to a Kubernetes cluster where the KubeVela controller processes it.

#### Option A: Direct Programmatic Deployment (SDK Client)

```go
// Using SDK client directly
client, _ := client.NewClient(kubeconfig)
err := client.Apply(ctx, app)

// Wait for application to be healthy
err = client.WaitForHealthy(ctx, "my-app", "production", 5*time.Minute)
```

**What happens under the hood:**

```
┌─────────────────────────────────────────────────────────────────────────┐
│  SDK client.Apply()                                                      │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  1. app.Build() → v1beta1.Application (K8s CR struct)                   │
│                                                                          │
│  2. Uses controller-runtime client (same as kubectl)                     │
│     k8sClient.Create() or k8sClient.Update()                            │
│                                                                          │
│  3. Application CR is persisted to etcd                                  │
│                                                                          │
│  4. KubeVela controller watches for Application changes                  │
│     → Renders CUE templates with your properties                         │
│     → Creates Deployment, Service, etc.                                  │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

The SDK client is a thin wrapper around `controller-runtime/pkg/client`, which is the same client library used by `kubectl`. It provides KubeVela-specific conveniences like `WaitForHealthy()`.

#### Option A2: Using kubectl or vela CLI

If you prefer CLI tools, export to YAML and apply:

```go
// Generate YAML
yaml, _ := app.ToYAML()
os.WriteFile("app.yaml", []byte(yaml), 0644)
```

```bash
# Apply using kubectl (same result as SDK client.Apply())
kubectl apply -f app.yaml

# Or using vela CLI
vela up -f app.yaml
```

#### Option B: Export YAML for GitOps

```go
// Export to YAML for ArgoCD/Flux
yaml, _ := app.ToYAML()
os.WriteFile("manifests/my-app.yaml", []byte(yaml), 0644)
```

Then commit to Git and let ArgoCD/Flux sync:

```yaml
# argocd/application.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: my-app
spec:
  source:
    repoURL: https://github.com/myorg/manifests
    path: manifests
  destination:
    server: https://kubernetes.default.svc
    namespace: production
```

#### Option C: CI/CD Pipeline Integration

```yaml
# .github/workflows/deploy.yaml
name: Deploy Application
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: "1.21"

      - name: Run tests
        run: make test-unit

      - name: Build deployment tool
        run: make build

      - name: Deploy to staging
        env:
          KUBECONFIG: ${{ secrets.STAGING_KUBECONFIG }}
          ENVIRONMENT: staging
        run: ./bin/deploy-app

      - name: Run integration tests
        run: make test-integration

      - name: Deploy to production
        if: github.ref == 'refs/heads/main'
        env:
          KUBECONFIG: ${{ secrets.PROD_KUBECONFIG }}
          ENVIRONMENT: production
        run: ./bin/deploy-app
```

### Step 5: What Happens After Apply

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Post-Apply: Controller Processing                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  SDK Client.Apply()                                                          │
│        │                                                                     │
│        ▼                                                                     │
│  ┌──────────────────────────────────────────────────────────────────┐       │
│  │ Application CR Created/Updated                                    │       │
│  │                                                                   │       │
│  │ apiVersion: core.oam.dev/v1beta1                                  │       │
│  │ kind: Application                                                 │       │
│  │ spec:                                                             │       │
│  │   components:                                                     │       │
│  │     - name: api                                                   │       │
│  │       type: webservice        ◄── References X-Definition        │       │
│  │       properties:             ◄── SDK Properties serialized      │       │
│  │         image: "myregistry/api:v1.2.3"                           │       │
│  │         port: 8080                                                │       │
│  └──────────────────────────────────────────────────────────────────┘       │
│        │                                                                     │
│        ▼                                                                     │
│  ┌──────────────────────────────────────────────────────────────────┐       │
│  │ KubeVela Controller                                               │       │
│  │                                                                   │       │
│  │  1. Watch Application CR                                          │       │
│  │  2. Load ComponentDefinition "webservice"                         │       │
│  │  3. Fill CUE template with properties as `parameter`              │       │
│  │  4. Inject `context.*` (name, namespace, clusterVersion, etc.)    │       │
│  │  5. Evaluate CUE conditionals (if parameter.cpu != _|_)           │       │
│  │  6. Render Kubernetes resources (Deployment, Service, etc.)       │       │
│  └──────────────────────────────────────────────────────────────────┘       │
│        │                                                                     │
│        ▼                                                                     │
│  ┌──────────────────────────────────────────────────────────────────┐       │
│  │ Kubernetes Resources Created                                      │       │
│  │                                                                   │       │
│  │  • Deployment/api                                                 │       │
│  │  • Service/api                                                    │       │
│  │  • HorizontalPodAutoscaler/api (from scaler trait)               │       │
│  └──────────────────────────────────────────────────────────────────┘       │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Conversion Utilities

```go
// Convert to/from YAML
yaml, err := app.ToYAML()
app, err := sdk.FromYAML(yamlBytes)

// Convert to/from JSON
json, err := app.ToJSON()
app, err := sdk.FromJSON(jsonBytes)

// Convert to/from K8s Application object
k8sApp, err := app.Build()
app, err := sdk.FromK8sApplication(k8sApp)
```

## Testing Best Practices

This section provides comprehensive guidance on testing applications built with the SDK using Ginkgo/Gomega and standard Go testing patterns.

### Testing Pyramid

```
                    ┌───────────────┐
                    │  E2E Tests    │  ← Fewest, slowest, require real cluster
                    │  (Optional)   │
                    ├───────────────┤
                    │  Integration  │  ← Some, slower, require envtest/cluster
                    │    Tests      │
                    ├───────────────┤
                    │               │
                    │  Unit Tests   │  ← Most, fastest, no cluster required
                    │               │
                    └───────────────┘
```

### What to Test vs What NOT to Test

#### ✅ WHAT TO TEST (Your Responsibility)

| Category                     | What to Test                               | Why                                        |
| ---------------------------- | ------------------------------------------ | ------------------------------------------ |
| **Builder correctness**      | Your code that calls SDK builders          | Ensures you're constructing apps correctly |
| **Validation**               | `app.Validate()` returns expected errors   | Catches invalid configurations early       |
| **Conditional logic**        | Environment-specific configurations        | Your business logic for different envs     |
| **YAML output structure**    | `app.ToYAML()` produces expected structure | Snapshot testing for drift detection       |
| **Integration with cluster** | App creates expected resources             | End-to-end verification                    |
| **Error handling**           | Your code handles SDK errors properly      | Application resilience                     |

#### ❌ WHAT NOT TO TEST (SDK/Controller Responsibility)

| Category                                     | Why Not to Test                                                      |
| -------------------------------------------- | -------------------------------------------------------------------- |
| **CUE template evaluation**                  | Controller does this at runtime; you can't test it without a cluster |
| **SDK internal validation logic**            | Already tested in SDK; trust it works                                |
| **Builder method chaining**                  | SDK tests cover this; you just use it                                |
| **Kubernetes resource rendering**            | Controller responsibility; not predictable in unit tests             |
| **Controller reconciliation**                | Integration test only; unit tests can't verify this                  |
| **Context variables (clusterVersion, etc.)** | Runtime-only; can't be tested without real cluster                   |

### Test Structure with Ginkgo/Gomega

```go
// test/unit/app_builder_test.go
package unit_test

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/oam-dev/kubevela/pkg/sdk"
    "github.com/oam-dev/kubevela/pkg/sdk/apis/component/webservice"
    "github.com/oam-dev/kubevela/pkg/sdk/apis/trait/scaler"

    "myproject/internal/apps"
)

func TestAppBuilder(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Application Builder Suite")
}

var _ = Describe("Application Builder", func() {

    Describe("BuildAPIComponent", func() {

        Context("when building for production environment", func() {
            var app sdk.TypedApplication

            BeforeEach(func() {
                app = apps.BuildApplication("production")
            })

            It("should set the correct namespace", func() {
                Expect(app.GetNamespace()).To(Equal("production"))
            })

            It("should use production replicas", func() {
                comp := app.GetComponentByName("api")
                Expect(comp).NotTo(BeNil())

                // Access built component to check properties
                built := comp.Build()
                Expect(built.Properties.Raw).To(ContainSubstring(`"replicas":3`))
            })

            It("should include required labels", func() {
                labels := app.GetLabels()
                Expect(labels).To(HaveKeyWithValue("team", "platform"))
                Expect(labels).To(HaveKeyWithValue("env", "production"))
            })

            It("should pass validation", func() {
                err := app.Validate()
                Expect(err).NotTo(HaveOccurred())
            })
        })

        Context("when building for development environment", func() {
            var app sdk.TypedApplication

            BeforeEach(func() {
                app = apps.BuildApplication("development")
            })

            It("should use fewer replicas", func() {
                comp := app.GetComponentByName("api")
                built := comp.Build()
                Expect(built.Properties.Raw).To(ContainSubstring(`"replicas":1`))
            })
        })
    })

    Describe("Validation", func() {

        Context("when image is missing", func() {
            It("should fail validation with clear error", func() {
                // Intentionally create invalid component
                app := sdk.NewApplication("test").
                    AddComponent(
                        webservice.New("bad-component"),
                        // Missing Image() - required field
                    )

                err := app.Validate()
                Expect(err).To(HaveOccurred())
                Expect(err.Error()).To(ContainSubstring("image"))
                Expect(err.Error()).To(ContainSubstring("required"))
            })
        })

        Context("when incompatible trait is added", func() {
            It("should fail validation", func() {
                app := sdk.NewApplication("test").
                    AddComponent(
                        webservice.New("api").
                            Image("nginx").
                            AddTrait(incompatibleTrait.New()), // Hypothetical incompatible trait
                    )

                err := app.Validate()
                Expect(err).To(HaveOccurred())
                Expect(err.Error()).To(ContainSubstring("not compatible"))
            })
        })
    })

    Describe("YAML Output", func() {
        It("should produce valid YAML structure", func() {
            app := apps.BuildApplication("production")

            yaml, err := app.ToYAML()
            Expect(err).NotTo(HaveOccurred())

            // Check key structure elements
            Expect(yaml).To(ContainSubstring("apiVersion: core.oam.dev/v1beta1"))
            Expect(yaml).To(ContainSubstring("kind: Application"))
            Expect(yaml).To(ContainSubstring("name: my-app"))
            Expect(yaml).To(ContainSubstring("type: webservice"))
        })

        It("should match snapshot", func() {
            app := apps.BuildApplication("production")
            yaml, _ := app.ToYAML()

            // Golden file / snapshot testing
            Expect(yaml).To(MatchSnapshot()) // Using go-snapshot or similar
        })
    })
})
```

### Table-Driven Tests for Parameter Variations

```go
var _ = Describe("Component Configuration Variations", func() {

    DescribeTable("should correctly configure resources",
        func(cpu, memory string, expectCpu, expectMemory bool) {
            builder := webservice.New("test").Image("nginx")

            if cpu != "" {
                builder.Cpu(cpu)
            }
            if memory != "" {
                builder.Memory(memory)
            }

            comp := builder.Build()
            props := string(comp.Properties.Raw)

            if expectCpu {
                Expect(props).To(ContainSubstring(`"cpu"`))
            } else {
                Expect(props).NotTo(ContainSubstring(`"cpu"`))
            }

            if expectMemory {
                Expect(props).To(ContainSubstring(`"memory"`))
            } else {
                Expect(props).NotTo(ContainSubstring(`"memory"`))
            }
        },

        Entry("no resources set", "", "", false, false),
        Entry("only CPU set", "500m", "", true, false),
        Entry("only memory set", "", "512Mi", false, true),
        Entry("both set", "500m", "512Mi", true, true),
    )

    DescribeTable("should validate replicas within bounds",
        func(replicas int, shouldPass bool) {
            app := sdk.NewApplication("test").
                AddComponent(
                    webservice.New("api").
                        Image("nginx").
                        Replicas(replicas),
                )

            err := app.Validate()
            if shouldPass {
                Expect(err).NotTo(HaveOccurred())
            } else {
                Expect(err).To(HaveOccurred())
            }
        },

        Entry("minimum valid replicas", 1, true),
        Entry("typical replicas", 3, true),
        Entry("maximum replicas", 100, true),
        Entry("zero replicas", 0, false),        // Assuming validation rejects 0
        Entry("negative replicas", -1, false),
    )
})
```

### Integration Tests with envtest

```go
// test/integration/deploy_test.go
//go:build integration

package integration_test

import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "sigs.k8s.io/controller-runtime/pkg/envtest"
    "k8s.io/client-go/kubernetes/scheme"

    "github.com/oam-dev/kubevela-core-api/apis/core.oam.dev/v1beta1"
    sdk "github.com/oam-dev/kubevela/pkg/sdk"
    "github.com/oam-dev/kubevela/pkg/sdk/client"

    "myproject/internal/apps"
)

var testEnv *envtest.Environment
var velaClient client.Client

var _ = BeforeSuite(func() {
    By("bootstrapping test environment")
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{
            "../../config/crd/bases",
        },
    }

    cfg, err := testEnv.Start()
    Expect(err).NotTo(HaveOccurred())

    err = v1beta1.AddToScheme(scheme.Scheme)
    Expect(err).NotTo(HaveOccurred())

    velaClient, err = client.NewClient(cfg)
    Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
    By("tearing down the test environment")
    err := testEnv.Stop()
    Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("Application Deployment", func() {

    Context("when deploying a valid application", func() {
        var app sdk.TypedApplication

        BeforeEach(func() {
            app = apps.BuildApplication("test")
        })

        AfterEach(func() {
            // Cleanup
            ctx := context.Background()
            _ = velaClient.Delete(ctx, app)
        })

        It("should create the Application CR successfully", func() {
            ctx := context.Background()

            err := velaClient.Apply(ctx, app)
            Expect(err).NotTo(HaveOccurred())

            // Verify Application exists
            retrieved, err := velaClient.Get(ctx, "my-app", "test")
            Expect(err).NotTo(HaveOccurred())
            Expect(retrieved).NotTo(BeNil())
        })

        It("should update an existing application", func() {
            ctx := context.Background()

            // Initial apply
            err := velaClient.Apply(ctx, app)
            Expect(err).NotTo(HaveOccurred())

            // Modify and re-apply
            app = app.(*sdk.ApplicationImpl).
                Labels(map[string]string{"updated": "true"})

            err = velaClient.Apply(ctx, app)
            Expect(err).NotTo(HaveOccurred())

            // Verify update
            retrieved, err := velaClient.Get(ctx, "my-app", "test")
            Expect(err).NotTo(HaveOccurred())
            Expect(retrieved.GetLabels()).To(HaveKeyWithValue("updated", "true"))
        })
    })
})
```

### E2E Tests (Full Cluster Required)

```go
// test/e2e/full_deploy_test.go
//go:build e2e

package e2e_test

import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    appsv1 "k8s.io/api/apps/v1"
    corev1 "k8s.io/api/core/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"

    sdk "github.com/oam-dev/kubevela/pkg/sdk"
    velaclient "github.com/oam-dev/kubevela/pkg/sdk/client"

    "myproject/internal/apps"
)

var _ = Describe("E2E: Full Application Deployment", func() {

    // These tests require:
    // - A real Kubernetes cluster with KubeVela installed
    // - The webservice ComponentDefinition to be applied
    // - Network access to pull container images

    Context("deploying webservice to real cluster", func() {
        var velaClient velaclient.Client
        var k8sClient client.Client
        var ctx context.Context

        BeforeEach(func() {
            ctx = context.Background()
            var err error
            velaClient, err = velaclient.NewClientFromKubeconfig("")
            Expect(err).NotTo(HaveOccurred())
        })

        It("should create Deployment and Service from webservice component", func() {
            app := apps.BuildApplication("e2e-test")

            // Apply application
            err := velaClient.Apply(ctx, app)
            Expect(err).NotTo(HaveOccurred())

            // Wait for healthy (controller renders resources)
            err = velaClient.WaitForHealthy(ctx, "my-app", "e2e-test", 2*time.Minute)
            Expect(err).NotTo(HaveOccurred())

            // Verify Kubernetes resources were created
            deployment := &appsv1.Deployment{}
            err = k8sClient.Get(ctx, client.ObjectKey{
                Namespace: "e2e-test",
                Name:      "api",  // Component name
            }, deployment)
            Expect(err).NotTo(HaveOccurred())
            Expect(*deployment.Spec.Replicas).To(Equal(int32(3)))

            service := &corev1.Service{}
            err = k8sClient.Get(ctx, client.ObjectKey{
                Namespace: "e2e-test",
                Name:      "api",
            }, service)
            Expect(err).NotTo(HaveOccurred())
        })
    })
})
```

### Testing Utilities

```go
// test/helpers/builders.go
package helpers

import (
    sdk "github.com/oam-dev/kubevela/pkg/sdk"
    "github.com/oam-dev/kubevela/pkg/sdk/apis/component/webservice"
)

// MinimalValidApp creates a minimal valid application for testing
func MinimalValidApp(name string) sdk.TypedApplication {
    return sdk.NewApplication(name).
        Namespace("default").
        AddComponent(
            webservice.New("test-component").
                Image("nginx:latest"),
        )
}

// AppWithComponent creates an app with a customizable component
func AppWithComponent(name string, comp interface{ Build() common.ApplicationComponent }) sdk.TypedApplication {
    return sdk.NewApplication(name).
        Namespace("default").
        AddComponent(comp)
}
```

### Test Fixtures and Snapshots

```go
// test/fixtures/expected_yaml.go
package fixtures

const ProductionAppYAML = `apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: my-app
  namespace: production
  labels:
    team: platform
    env: production
spec:
  components:
    - name: api
      type: webservice
      properties:
        image: "myregistry/api:v1.2.3"
        port: 8080
        replicas: 3
        cpu: "500m"
        memory: "512Mi"
      traits:
        - type: scaler
          properties:
            min: 3
            max: 9
`

// Use in tests
var _ = Describe("YAML Output", func() {
    It("should match expected production YAML", func() {
        app := apps.BuildApplication("production")
        yaml, _ := app.ToYAML()
        Expect(yaml).To(Equal(fixtures.ProductionAppYAML))
    })
})
```

### Testing Checklist

| Test Type             | Framework                  | When to Run      | Cluster Required   |
| --------------------- | -------------------------- | ---------------- | ------------------ |
| Unit tests            | Ginkgo/Gomega or `go test` | Every commit, CI | No                 |
| Validation tests      | Ginkgo/Gomega or `go test` | Every commit, CI | No                 |
| Snapshot tests        | `go-snapshot`              | Every commit, CI | No                 |
| Integration (envtest) | Ginkgo + envtest           | PR, nightly      | Partial (envtest)  |
| E2E tests             | Ginkgo                     | Release, nightly | Yes (real cluster) |

### Running Tests

```bash
# Unit tests only (fast, no cluster)
ginkgo run ./test/unit/...

# Integration tests (requires envtest binaries)
make envtest-setup
ginkgo run --tags=integration ./test/integration/...

# E2E tests (requires real cluster with KubeVela)
export KUBECONFIG=~/.kube/config
ginkgo run --tags=e2e ./test/e2e/...

# All tests with coverage
ginkgo run -r --cover --coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Specific test focus
ginkgo run --focus="Validation" ./test/unit/...
```

### Anti-Patterns to Avoid

❌ **Don't test SDK internals:**

```go
// BAD: Testing SDK's own validation logic
It("should validate port range", func() {
    comp := webservice.New("test").Image("nginx").Port(99999)
    err := comp.Validate()
    Expect(err).To(HaveOccurred()) // Testing SDK, not your code
})
```

❌ **Don't test CUE template evaluation:**

```go
// BAD: Trying to verify CUE conditional logic
It("should add resources when cpu is set", func() {
    app := BuildApp().Cpu("500m")
    // CAN'T verify the CUE `if parameter.cpu != _|_` here
    // That only runs in the controller
})
```

❌ **Don't mock the SDK:**

```go
// BAD: Mocking SDK to return fake data
mockBuilder := &MockWebserviceBuilder{}
mockBuilder.On("Image", "nginx").Return(mockBuilder)
// This doesn't test anything useful
```

✅ **DO test your business logic:**

```go
// GOOD: Testing YOUR code that uses the SDK
It("should use production config in production", func() {
    app := apps.BuildApplication("production")  // Your function
    Expect(app.GetNamespace()).To(Equal("production"))
    // ... verify YOUR logic
})
```

## Compatibility

### Coexistence with Existing Approaches

| Existing Approach          | SDK Approach            | Conflict?              |
| -------------------------- | ----------------------- | ---------------------- |
| Write Application YAML     | Use fluent SDK builders | No - both valid        |
| Use `kubectl apply`        | Use SDK client          | No - same result       |
| Current `vela def gen-api` | New `vela sdk generate` | No - separate commands |

### Migration Path

Users can adopt incrementally:

1. Continue using YAML for existing applications
2. Use SDK for new applications
3. Convert existing YAML to SDK code as needed

```bash
# Convert existing YAML to SDK code (future enhancement)
vela sdk convert ./app.yaml --output ./app.go
```

## Tool Version Requirements

The SDK generation process uses specific tool versions for reproducibility:

| Tool                  | Version | Purpose                  |
| --------------------- | ------- | ------------------------ |
| Go                    | 1.21+   | SDK compilation          |
| CUE                   | 0.6.0+  | Schema extraction        |
| openapi-generator-cli | 7.0.1   | OpenAPI to Go (pinned)   |
| Docker                | 20.10+  | Containerized generation |

These versions are enforced by `vela sdk generate` and documented in generated SDK.

## Implementation Plan

### Phase 1: Core Framework

- Enhanced IR definition with CUE metadata preservation
- Go code generator with fluent builders
- Application, Component, Trait builders
- Basic validation including cross-field rules
- Trait compatibility validation
- Namespaced builder registration

### Phase 2: Complete Definition Support

- Policy and WorkflowStep builders
- Workflow mode support
- OneOf/discriminated union support with proper serialization
- Client implementation
- Version compatibility checking

### Phase 3: Advanced Features

- Custom definition SDK generation
- OCI registry source
- SDK version management
- Documentation generation
- Unknown field preservation for round-trip fidelity

### Phase 4: Multi-Language SDKs (Future)

- Publish IR artifacts as part of KubeVela releases
- Python SDK in `github.com/oam-dev/kubevela-sdk-python`
- TypeScript SDK in `github.com/oam-dev/kubevela-sdk-typescript`
- Language-specific generators consuming shared IR
- PyPI/npm package publishing workflows

## Alternatives Considered

### 1. Separate Repository for SDK (kubevela-go-sdk)

The previous approach considered a separate repository. This would create versioning complexity and require separate CI/CD pipelines.

**Decision:** In-repo SDK generation ensures version alignment with KubeVela releases and simplifies testing.

### 2. Use OpenAPI Generator Directly

OpenAPI generators produce valid code but lack:

- Fluent builder patterns
- KubeVela-specific validation
- Definition relationship handling
- CUE-specific metadata (conditional requirements, closed structs)

**Decision:** Custom generator using enhanced IR for better control and accuracy.

### 3. Code Generation via Protobuf

Would require CUE → Protobuf conversion and custom plugins.

**Decision:** Direct Go generation is simpler and sufficient.
