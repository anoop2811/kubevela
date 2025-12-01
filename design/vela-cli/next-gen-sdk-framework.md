# Next-Generation Multi-Language SDK Framework

## Introduction

This proposal introduces a comprehensive framework for generating developer-friendly, type-safe SDKs in multiple programming languages from KubeVela X-Definitions. Additionally, it enables platform engineers to **author X-Definitions in native languages** (Go, TypeScript, Python, Java) without needing to learn CUE.

The framework provides:
1. **SDK Generation** - Generate fluent, type-safe SDKs for consuming X-Definitions
2. **Definition Authoring** - Create X-Definitions in native languages with transparent CUE compilation
3. **Always-in-Sync** - Live introspection keeps SDKs synchronized with cluster state

The goal is to **eliminate the CUE learning curve** as an adoption barrier while maintaining the full power and flexibility of KubeVela's abstraction layer.

## Background

### Current State

KubeVela has an existing SDK generation mechanism (`vela def gen-api`) that:

1. Converts CUE definitions to OpenAPI schemas
2. Uses `openapi-generator` to produce language-specific type definitions
3. Generates Go code with basic helper methods via the jennifer library

**Current Limitations:**

| Limitation | Impact | User Feedback |
|------------|--------|---------------|
| Generated types, not fluent builders | Poor developer experience | "Still need to understand CUE structure" |
| Go-only fully supported | Limited language reach | "We need TypeScript/Java SDKs" |
| Manual regeneration required | Sync drift with core | "SDK doesn't match latest KubeVela version" |
| No custom definition support | Platform teams can't use SDK | "Can't generate SDK for our definitions" |
| Separate packages per definition | No unified API surface | "Import paths are complex" |
| Can't author definitions in native languages | Platform engineers must learn CUE | "CUE learning curve is too steep" |

### The Problem

The existing kubevela-go-sdk (kubevela-contrib) has:
- Minimal community adoption
- Auto-generated code that doesn't provide a significantly better experience than raw YAML
- Known issues with labels/annotations traits and workflow steps

**Observed CUE Adoption Challenges:**
- CUE is a specialized configuration language with limited mainstream adoption
- Platform engineers familiar with Go/TypeScript/Python/Java face a learning curve
- IDE tooling and debugging support for CUE is less mature than mainstream languages
- Community feedback in GitHub issues and Slack indicates CUE complexity as a common pain point

### Why This Matters

KubeVela's technical capabilities (multi-cluster, workflows, OAM model) are industry-leading, but adoption is limited by the CUE learning curve. A developer-friendly SDK would:

1. **Lower the entry barrier** for new users
2. **Enable platform teams** to build on KubeVela programmatically
3. **Support GitOps workflows** with typed configuration
4. **Improve IDE experience** with auto-completion and type checking

## Goals & Non-Goals

### Goals

1. **Multi-language SDK generation** from a single source of truth
   - Go (enhanced), TypeScript, Java, Python

2. **Fluent builder pattern** for all SDKs
   ```go
   // Target developer experience
   app := vela.WebService("frontend").
       Image("nginx:1.21").
       Port(80).
       AddTrait(vela.Autoscaler().Min(2).Max(10))
   ```

3. **Native language X-Definition authoring** (Definition Kit)
   - Author ComponentDefinition, TraitDefinition, PolicyDefinition, WorkflowStepDefinition in Go/TypeScript/Python/Java
   - CUE compilation happens transparently behind the scenes

4. **Automatic synchronization** with KubeVela core releases
   - SDK versions aligned with KubeVela versions
   - Live introspection from cluster state
   - Deprecation notices for removed definitions

5. **Custom X-Definition support** for platform teams
   - Generate SDK from custom definitions
   - Support definitions from cluster, local files, or OCI registries

6. **Unified SDK package** with consistent API surface across languages

### Non-Goals

1. Replacing CUE as the internal engine (CUE remains the implementation detail)
2. Supporting every programming language (focus on popular ones)
3. Runtime CUE evaluation in SDK (compile-time only)
4. Backwards compatibility with the current kubevela-go-sdk structure
5. Backporting to KubeVela versions prior to 2.0.0

### Prerequisites

This framework assumes the following are in place:

1. **Definition Versioning** - Definitions should have `spec.version` set for version-specific SDK generation (see `design/vela-core/definition-versioning.md`)
2. **OpenAPI Schema Generation** - Existing ConfigMap-based schema storage continues to work
3. **Namespace-based Resolution** - Existing definition resolution hierarchy is unchanged

---

## Versioning & Migration Strategy

### Version Support

This SDK framework will be introduced as an **alpha feature in KubeVela 2.0.0-alpha**:

- **Supported Version:** KubeVela 2.0.0-alpha and later only
- **No Backport:** Previous versions (1.x) will not receive this feature
- **Alpha Status:** The API may evolve based on community feedback during the alpha period

### Coexistence with Existing Functionality

This framework is designed to **coexist** with existing KubeVela functionality without conflicts:

| Existing Approach | New SDK Approach | Conflict? |
|-------------------|------------------|-----------|
| Write CUE definitions directly | Write definitions in Go/TS/Java/Python | No - both produce valid X-Definitions |
| Use `vela def apply` with CUE files | Use `vela def apply` with native code | No - same command, different input formats |
| Generate SDK with `vela def gen-api` | Generate SDK with `vela sdk generate` | No - new command, old command remains |
| Write Application YAML manually | Use fluent SDK builders | No - both produce valid Applications |

**Key Principle:** Users can adopt the new SDK incrementally. Existing CUE-based workflows continue to work unchanged.

### Migration Path

Users with existing CUE definitions can migrate at their own pace:

```bash
# Convert existing CUE definitions to native language (optional)
vela def import ./existing-definitions/*.cue --language go --output ./converted/

# Or continue using CUE definitions alongside new native definitions
# Both work with the same vela def apply command
vela def apply ./cue-definitions/
vela def apply ./go-definitions/
```

**Migration is optional.** Users can:
1. Continue using CUE definitions indefinitely
2. Mix CUE and native language definitions in the same project
3. Gradually convert definitions as needed

### Deprecation Policy

| Version | Status |
|---------|--------|
| 2.0.0-alpha | SDK framework introduced as alpha |
| 2.0.0 | SDK framework becomes stable |
| 2.1.0 | Evaluate deprecation of legacy `vela def gen-api` (if warranted by adoption) |
| 2.1.0+ | Legacy approaches remain supported; deprecation only after community consultation |

**Commitment:** No existing functionality will be deprecated until at least version 2.1.0, and only after:
1. The new SDK framework is proven stable
2. Migration tooling is mature
3. Community feedback supports the deprecation

---

## Part 1: Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           SDK Generation Pipeline                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐                   │
│  │    Source    │    │      IR      │    │  Generator   │                   │
│  │              │    │              │    │              │                   │
│  │ • Cluster    │───▶│ • Schema     │───▶│ • Go         │──▶ SDK Packages  │
│  │ • Local      │    │ • Metadata   │    │ • TypeScript │                   │
│  │ • OCI        │    │ • Relations  │    │ • Java       │                   │
│  │ • Release    │    │              │    │ • Python     │                   │
│  └──────────────┘    └──────────────┘    └──────────────┘                   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│                        Definition Authoring Pipeline                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐                   │
│  │   defkit     │    │      IR      │    │  Compiler    │                   │
│  │              │    │              │    │              │                   │
│  │ • Go API     │───▶│ • JSON       │───▶│ • CUE Gen    │──▶ X-Definition  │
│  │ • TS API     │    │ • Portable   │    │ • Validation │     (Applied)    │
│  │ • Python API │    │              │    │ • Apply      │                   │
│  └──────────────┘    └──────────────┘    └──────────────┘                   │
│                                                                              │
│  (CUE compilation is transparent - developers never see it)                  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Intermediate Representation (IR)

Instead of generating directly from OpenAPI, introduce an Intermediate Representation that captures:

```go
// pkg/definition/gen_sdk/ir/definition.go

// DefinitionIR is the intermediate representation of a KubeVela definition
type DefinitionIR struct {
    // Metadata
    Name        string            `json:"name"`
    Type        DefinitionType    `json:"type"` // component, trait, policy, workflow-step
    Description string            `json:"description"`
    Version     string            `json:"version"`
    Labels      map[string]string `json:"labels"`
    Annotations map[string]string `json:"annotations"`

    // Schema
    Properties  *PropertySchema   `json:"properties"`

    // Relationships (for traits)
    AppliesToWorkloads []string   `json:"appliesToWorkloads,omitempty"`
    ConflictsWith      []string   `json:"conflictsWith,omitempty"`

    // Source tracking
    Source      DefinitionSource  `json:"source"`
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
    Min         *int                       `json:"min,omitempty"`
    Max         *int                       `json:"max,omitempty"`
    MinLength   *int                       `json:"minLength,omitempty"`
    MaxLength   *int                       `json:"maxLength,omitempty"`
    Pattern     string                     `json:"pattern,omitempty"`

    // Builder hints
    BuilderName string                     `json:"builderName,omitempty"`
    Chainable   bool                       `json:"chainable"`
}
```

### Language-Agnostic Generator Interface

```go
// pkg/definition/gen_sdk/generator.go

// LanguageGenerator defines the interface for language-specific generators
type LanguageGenerator interface {
    // Name returns the language name (e.g., "go", "typescript", "java")
    Name() string

    // Generate produces SDK code from the intermediate representation
    Generate(ctx context.Context, config *GeneratorConfig, defs []*ir.DefinitionIR) error

    // GenerateBuilders creates fluent builder patterns for definitions
    GenerateBuilders(ctx context.Context, def *ir.DefinitionIR) ([]GeneratedFile, error)

    // GenerateClient creates the API client for applying resources
    GenerateClient(ctx context.Context, config *GeneratorConfig) ([]GeneratedFile, error)

    // PostProcess handles language-specific post-processing (formatting, linting)
    PostProcess(ctx context.Context, outputDir string) error
}
```

---

## Part 2: Native Language X-Definition Authoring (defkit)

### Problem Statement

Currently, platform engineers must learn CUE to create X-Definitions. This presents significant barriers:

1. **Learning Curve** - CUE is a specialized language unfamiliar to most developers
2. **Tooling Gap** - IDE support, debugging, and testing for CUE is limited compared to mainstream languages
3. **Ecosystem Isolation** - Cannot leverage existing libraries, patterns, and frameworks from the developer's primary language

### Solution: Definition Kit (defkit)

The Definition Kit provides native language APIs for authoring X-Definitions. Developers write definitions in their language of choice, and the KubeVela CLI or controller handles all CUE compilation transparently.

#### Go Definition Kit API

```go
package myplatform

import (
    "github.com/oam-dev/kubevela/pkg/defkit"
    "github.com/oam-dev/kubevela/pkg/defkit/component"
    "github.com/oam-dev/kubevela/pkg/defkit/trait"
)

// Define a custom microservice component
func MicroserviceComponent() *component.Definition {
    return component.New("microservice").
        Description("A production-ready microservice with sidecar support").
        Labels(map[string]string{
            "platform.example.com/tier": "application",
        }).
        Workload(component.Deployment()).
        Parameter("name", defkit.String().Required().
            Description("Name of the microservice")).
        Parameter("image", defkit.String().Required().
            Description("Container image")).
        Parameter("replicas", defkit.Int().Default(3).
            Min(1).Max(100).
            Description("Number of replicas")).
        Parameter("port", defkit.Int().Default(8080).
            Description("Service port")).
        Parameter("env", defkit.Map(defkit.String()).Optional().
            Description("Environment variables")).
        Parameter("resources", defkit.Struct(
            defkit.Field("cpu", defkit.String().Default("100m")),
            defkit.Field("memory", defkit.String().Default("128Mi")),
        ).Description("Resource requests")).
        Template(func(ctx *component.Context) *component.Output {
            // Use schema-agnostic resource construction (see Part 2a)
            deploy := defkit.NewResource("apps/v1", "Deployment").
                SetName(ctx.Param("name").String()).
                SetNamespace(ctx.Namespace()).
                Set("spec.replicas", ctx.Param("replicas").Int()).
                Set("spec.selector.matchLabels", map[string]string{
                    "app.oam.dev/component": ctx.ComponentName(),
                }).
                Set("spec.template.metadata.labels", map[string]string{
                    "app.oam.dev/component": ctx.ComponentName(),
                }).
                Set("spec.template.spec.containers", []map[string]any{{
                    "name":  ctx.Param("name").String(),
                    "image": ctx.Param("image").String(),
                    "ports": []map[string]any{{
                        "containerPort": ctx.Param("port").Int(),
                    }},
                    "env":       defkit.EnvFromMap(ctx.Param("env").StringMap()),
                    "resources": ctx.Param("resources").Raw(),
                }})
            return ctx.Output(deploy)
        })
}

// Define a custom rate-limiting trait
func RateLimitTrait() *trait.Definition {
    return trait.New("rate-limit").
        Description("Apply rate limiting to a service").
        AppliesTo("webservice", "microservice").
        Parameter("rps", defkit.Int().Required().
            Min(1).Max(10000).
            Description("Requests per second limit")).
        Parameter("burst", defkit.Int().Default(100).
            Description("Burst capacity")).
        Patch(func(ctx *trait.Context) *trait.Patches {
            return ctx.PatchWorkload().
                AddAnnotation("ratelimit.example.com/rps",
                    ctx.Param("rps").String()).
                AddAnnotation("ratelimit.example.com/burst",
                    ctx.Param("burst").String())
        })
}
```

#### TypeScript Definition Kit API

```typescript
import { component, trait, defkit } from '@kubevela/defkit';

// Define a custom microservice component
export const MicroserviceComponent = component.new('microservice')
  .description('A production-ready microservice with sidecar support')
  .labels({ 'platform.example.com/tier': 'application' })
  .workload(component.deployment())
  .parameter('name', defkit.string().required()
    .description('Name of the microservice'))
  .parameter('image', defkit.string().required()
    .description('Container image'))
  .parameter('replicas', defkit.int().default(3).min(1).max(100)
    .description('Number of replicas'))
  .parameter('port', defkit.int().default(8080)
    .description('Service port'))
  .parameter('env', defkit.map(defkit.string()).optional()
    .description('Environment variables'))
  .parameter('resources', defkit.struct({
    cpu: defkit.string().default('100m'),
    memory: defkit.string().default('128Mi'),
  }).description('Resource requests'))
  .template((ctx) => {
    // Schema-agnostic resource construction (see Part 2a)
    return defkit.newResource('apps/v1', 'Deployment')
      .setName(ctx.param('name').string())
      .setNamespace(ctx.namespace())
      .set('spec.replicas', ctx.param('replicas').int())
      .set('spec.selector.matchLabels', {
        'app.oam.dev/component': ctx.componentName(),
      })
      .set('spec.template.metadata.labels', {
        'app.oam.dev/component': ctx.componentName(),
      })
      .set('spec.template.spec.containers', [{
        name: ctx.param('name').string(),
        image: ctx.param('image').string(),
        ports: [{ containerPort: ctx.param('port').int() }],
        env: defkit.envFromMap(ctx.param('env').stringMap()),
        resources: ctx.param('resources').raw(),
      }]);
  });

// Define a custom rate-limiting trait
export const RateLimitTrait = trait.new('rate-limit')
  .description('Apply rate limiting to a service')
  .appliesTo('webservice', 'microservice')
  .parameter('rps', defkit.int().required().min(1).max(10000)
    .description('Requests per second limit'))
  .parameter('burst', defkit.int().default(100)
    .description('Burst capacity'))
  .patch((ctx) => ctx.patchWorkload()
    .addAnnotation('ratelimit.example.com/rps', ctx.param('rps').string())
    .addAnnotation('ratelimit.example.com/burst', ctx.param('burst').string())
  );
```

#### Python Definition Kit API

```python
from kubevela.defkit import component, trait, defkit

# Define a custom microservice component
@component.define("microservice")
class MicroserviceComponent:
    """A production-ready microservice with sidecar support"""

    name = defkit.string(required=True, description="Name of the microservice")
    image = defkit.string(required=True, description="Container image")
    replicas = defkit.int(default=3, min=1, max=100, description="Number of replicas")
    port = defkit.int(default=8080, description="Service port")
    env = defkit.map(defkit.string(), optional=True, description="Environment variables")
    resources = defkit.struct(
        cpu=defkit.string(default="100m"),
        memory=defkit.string(default="128Mi"),
        description="Resource requests"
    )

    def template(self, ctx):
        # Schema-agnostic resource construction (see Part 2a)
        return defkit.new_resource("apps/v1", "Deployment") \
            .set_name(ctx.param("name")) \
            .set_namespace(ctx.namespace()) \
            .set("spec.replicas", ctx.param("replicas")) \
            .set("spec.selector.matchLabels", {
                "app.oam.dev/component": ctx.component_name(),
            }) \
            .set("spec.template.metadata.labels", {
                "app.oam.dev/component": ctx.component_name(),
            }) \
            .set("spec.template.spec.containers", [{
                "name": ctx.param("name"),
                "image": ctx.param("image"),
                "ports": [{"containerPort": ctx.param("port")}],
                "env": defkit.env_from_map(ctx.param("env")),
                "resources": ctx.param("resources").raw(),
            }])

# Define a custom rate-limiting trait
@trait.define("rate-limit", applies_to=["webservice", "microservice"])
class RateLimitTrait:
    """Apply rate limiting to a service"""

    rps = defkit.int(required=True, min=1, max=10000,
                     description="Requests per second limit")
    burst = defkit.int(default=100, description="Burst capacity")

    def patch(self, ctx):
        return ctx.patch_workload() \
            .add_annotation("ratelimit.example.com/rps", str(ctx.param("rps"))) \
            .add_annotation("ratelimit.example.com/burst", str(ctx.param("burst")))
```

#### Java Definition Kit API (Java 21 LTS / OpenJDK)

```java
package com.myplatform.definitions;

import io.kubevela.defkit.Component;
import io.kubevela.defkit.Trait;
import io.kubevela.defkit.Defkit;
import io.kubevela.defkit.Parameter;
import io.kubevela.defkit.component.ComponentDefinition;
import io.kubevela.defkit.trait.TraitDefinition;

// Define a custom microservice component using Java records and builders
public class PlatformDefinitions {

    public static ComponentDefinition microserviceComponent() {
        return Component.create("microservice")
            .description("A production-ready microservice with sidecar support")
            .labels(Map.of("platform.example.com/tier", "application"))
            .workload(Component.deployment())
            .parameter("name", Defkit.string()
                .required()
                .description("Name of the microservice"))
            .parameter("image", Defkit.string()
                .required()
                .description("Container image"))
            .parameter("replicas", Defkit.integer()
                .defaultValue(3)
                .min(1)
                .max(100)
                .description("Number of replicas"))
            .parameter("port", Defkit.integer()
                .defaultValue(8080)
                .description("Service port"))
            .parameter("env", Defkit.map(Defkit.string())
                .optional()
                .description("Environment variables"))
            .parameter("resources", Defkit.struct()
                .field("cpu", Defkit.string().defaultValue("100m"))
                .field("memory", Defkit.string().defaultValue("128Mi"))
                .description("Resource requests"))
            // Schema-agnostic resource construction (see Part 2a)
            .template(ctx -> Defkit.newResource("apps/v1", "Deployment")
                .setName(ctx.param("name").asString())
                .setNamespace(ctx.namespace())
                .set("spec.replicas", ctx.param("replicas").asInt())
                .set("spec.selector.matchLabels", Map.of(
                    "app.oam.dev/component", ctx.componentName()
                ))
                .set("spec.template.metadata.labels", Map.of(
                    "app.oam.dev/component", ctx.componentName()
                ))
                .set("spec.template.spec.containers", List.of(Map.of(
                    "name", ctx.param("name").asString(),
                    "image", ctx.param("image").asString(),
                    "ports", List.of(Map.of("containerPort", ctx.param("port").asInt())),
                    "env", Defkit.envFromMap(ctx.param("env").asStringMap()),
                    "resources", ctx.param("resources").raw()
                )))
            )
            .build();
    }

    public static TraitDefinition rateLimitTrait() {
        return Trait.create("rate-limit")
            .description("Apply rate limiting to a service")
            .appliesTo("webservice", "microservice")
            .parameter("rps", Defkit.integer()
                .required()
                .min(1)
                .max(10000)
                .description("Requests per second limit"))
            .parameter("burst", Defkit.integer()
                .defaultValue(100)
                .description("Burst capacity"))
            .patch(ctx -> ctx.patchWorkload()
                .addAnnotation("ratelimit.example.com/rps",
                    ctx.param("rps").asString())
                .addAnnotation("ratelimit.example.com/burst",
                    ctx.param("burst").asString())
            )
            .build();
    }
}
```

---

## Part 2a: Schema-Agnostic Resource Construction

### Design Philosophy: Zero Kubernetes Version Coupling

A critical design decision for the Definition Kit is that **defkit must not ship typed Kubernetes helpers**. Providing pre-built types like `ctx.Deployment()` or `ctx.Service()` would:

1. **Create maintenance burden** - KubeVela would need to track every K8s release (1.28, 1.29, 1.30, etc.)
2. **Handle API deprecations** - Managing transitions like `extensions/v1beta1` → `apps/v1`
3. **Support multiple versions** - Different clusters run different K8s versions
4. **Track alpha/beta promotions** - APIs graduate through stability levels

Instead, defkit provides **schema-agnostic resource construction** with three approaches:

### Approach 1: Unstructured Builder (Default, Zero Coupling)

The primary and recommended way to construct resources:

```go
package defkit

// Resource is the core type for constructing any Kubernetes resource
// It has ZERO dependencies on k8s.io/api/* packages
type Resource struct {
    object map[string]any
}

// NewResource creates a resource of any kind - works for core K8s, CRDs, Crossplane, KRO, etc.
func NewResource(apiVersion, kind string) *Resource {
    return &Resource{
        object: map[string]any{
            "apiVersion": apiVersion,
            "kind":       kind,
            "metadata":   map[string]any{},
            "spec":       map[string]any{},
        },
    }
}

// Fluent methods for common metadata
func (r *Resource) Name(name string) *Resource {
    r.object["metadata"].(map[string]any)["name"] = name
    return r
}

func (r *Resource) Namespace(ns string) *Resource {
    r.object["metadata"].(map[string]any)["namespace"] = ns
    return r
}

func (r *Resource) Labels(labels map[string]string) *Resource {
    r.object["metadata"].(map[string]any)["labels"] = labels
    return r
}

func (r *Resource) Annotations(annotations map[string]string) *Resource {
    r.object["metadata"].(map[string]any)["annotations"] = annotations
    return r
}

// Spec sets the entire spec field
func (r *Resource) Spec(spec map[string]any) *Resource {
    r.object["spec"] = spec
    return r
}

// SetField sets a nested field using dot notation (e.g., "spec.replicas")
func (r *Resource) SetField(path string, value any) *Resource {
    setNestedField(r.object, path, value)
    return r
}

// ToUnstructured converts to K8s unstructured for applying
func (r *Resource) ToUnstructured() *unstructured.Unstructured {
    return &unstructured.Unstructured{Object: deepCopy(r.object)}
}

// ToMap returns the raw map representation
func (r *Resource) ToMap() map[string]any {
    return deepCopy(r.object)
}
```

**Usage Example - Deployment:**

```go
func MicroserviceComponent() *component.Definition {
    return component.New("microservice").
        Parameter("name", defkit.String().Required()).
        Parameter("image", defkit.String().Required()).
        Parameter("replicas", defkit.Int().Default(3)).
        Parameter("port", defkit.Int().Default(8080)).

        Output(func(ctx *component.Context) *defkit.Resource {
            return defkit.NewResource("apps/v1", "Deployment").
                Name(ctx.Param("name").String()).
                Namespace(ctx.Namespace()).
                Labels(map[string]string{
                    "app.oam.dev/component": ctx.Name(),
                }).
                Spec(map[string]any{
                    "replicas": ctx.Param("replicas").Int(),
                    "selector": map[string]any{
                        "matchLabels": map[string]string{
                            "app": ctx.Param("name").String(),
                        },
                    },
                    "template": map[string]any{
                        "metadata": map[string]any{
                            "labels": map[string]string{
                                "app": ctx.Param("name").String(),
                            },
                        },
                        "spec": map[string]any{
                            "containers": []map[string]any{{
                                "name":  ctx.Param("name").String(),
                                "image": ctx.Param("image").String(),
                                "ports": []map[string]any{{
                                    "containerPort": ctx.Param("port").Int(),
                                }},
                            }},
                        },
                    },
                })
        })
}
```

**Usage Example - Any CRD (Crossplane, KRO, Istio, etc.):**

```go
// Works identically for ANY Kubernetes resource type
func CrossplaneS3Component() *component.Definition {
    return component.New("s3-bucket").
        Parameter("name", defkit.String().Required()).
        Parameter("region", defkit.String().Default("us-east-1")).

        Output(func(ctx *component.Context) *defkit.Resource {
            return defkit.NewResource("s3.aws.crossplane.io/v1beta1", "Bucket").
                Name(ctx.Param("name").String()).
                Spec(map[string]any{
                    "forProvider": map[string]any{
                        "region": ctx.Param("region").String(),
                        "acl":    "private",
                    },
                    "providerConfigRef": map[string]any{
                        "name": "aws-provider",
                    },
                })
        })
}
```

#### Composability: Reusable Resource Builders

Since `defkit.NewResource()` returns a pointer to `*defkit.Resource`, you can create reusable helper functions that build common resource patterns:

```go
package helpers

import "github.com/oam-dev/kubevela/pkg/defkit"

// StandardContainer creates a container with common configuration
func StandardContainer(name, image string, port int) map[string]any {
    return map[string]any{
        "name":  name,
        "image": image,
        "ports": []map[string]any{{
            "containerPort": port,
        }},
        "resources": map[string]any{
            "requests": map[string]any{
                "cpu":    "100m",
                "memory": "128Mi",
            },
            "limits": map[string]any{
                "cpu":    "500m",
                "memory": "512Mi",
            },
        },
    }
}

// BaseDeployment creates a deployment with standard labels and selector
func BaseDeployment(name, namespace string, replicas int) *defkit.Resource {
    return defkit.NewResource("apps/v1", "Deployment").
        Name(name).
        Namespace(namespace).
        Labels(map[string]string{"app": name}).
        Spec(map[string]any{
            "replicas": replicas,
            "selector": map[string]any{
                "matchLabels": map[string]string{"app": name},
            },
            "template": map[string]any{
                "metadata": map[string]any{
                    "labels": map[string]string{"app": name},
                },
            },
        })
}

// BaseService creates a ClusterIP service
func BaseService(name, namespace string, port int) *defkit.Resource {
    return defkit.NewResource("v1", "Service").
        Name(name).
        Namespace(namespace).
        Spec(map[string]any{
            "selector": map[string]string{"app": name},
            "ports": []map[string]any{{
                "port":       port,
                "targetPort": port,
            }},
        })
}
```

**Using Helpers in Definitions:**

```go
package definitions

import (
    "github.com/oam-dev/kubevela/pkg/defkit/component"
    "myplatform/helpers"
)

func WebserviceComponent() *component.Definition {
    return component.New("webservice").
        Parameter("image", defkit.String().Required()).
        Parameter("replicas", defkit.Int().Default(3)).
        Parameter("port", defkit.Int().Default(8080)).

        Output(func(ctx *component.Context) *defkit.Resource {
            name := ctx.Name()

            // Start with the base deployment, then customize
            return helpers.BaseDeployment(name, ctx.Namespace(), ctx.Param("replicas").Int()).
                Set("spec.template.spec.containers", []map[string]any{
                    helpers.StandardContainer(name, ctx.Param("image").String(), ctx.Param("port").Int()),
                })
        }).

        // Auxiliary resources can also use helpers
        Auxiliary("service", func(ctx *component.Context) *defkit.Resource {
            return helpers.BaseService(ctx.Name(), ctx.Namespace(), ctx.Param("port").Int())
        })
}
```

This composability enables:
- **Shared patterns** across multiple definitions in your organization
- **Platform-wide defaults** (resource limits, labels, annotations)
- **Testable units** - test helpers independently of definitions
- **DRY code** - avoid repeating boilerplate across definitions

### Approach 2: Typed Adapter (Optional, User's Coupling Choice)

If users want compile-time type safety, they can use existing `k8s.io/api` types directly. The coupling to K8s versions becomes **their responsibility**:

```go
import (
    appsv1 "k8s.io/api/apps/v1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/utils/ptr"
)

func MicroserviceComponent() *component.Definition {
    return component.New("microservice").
        Parameter("name", defkit.String().Required()).
        Parameter("image", defkit.String().Required()).
        Parameter("replicas", defkit.Int().Default(3)).

        Output(func(ctx *component.Context) *defkit.Resource {
            // Use upstream K8s types - THEY maintain version compatibility
            deployment := &appsv1.Deployment{
                TypeMeta: metav1.TypeMeta{
                    APIVersion: "apps/v1",
                    Kind:       "Deployment",
                },
                ObjectMeta: metav1.ObjectMeta{
                    Name:      ctx.Param("name").String(),
                    Namespace: ctx.Namespace(),
                },
                Spec: appsv1.DeploymentSpec{
                    Replicas: ptr.To(int32(ctx.Param("replicas").Int())),
                    Selector: &metav1.LabelSelector{
                        MatchLabels: map[string]string{
                            "app": ctx.Param("name").String(),
                        },
                    },
                    Template: corev1.PodTemplateSpec{
                        ObjectMeta: metav1.ObjectMeta{
                            Labels: map[string]string{
                                "app": ctx.Param("name").String(),
                            },
                        },
                        Spec: corev1.PodSpec{
                            Containers: []corev1.Container{{
                                Name:  ctx.Param("name").String(),
                                Image: ctx.Param("image").String(),
                            }},
                        },
                    },
                },
            }

            // Convert typed object to defkit.Resource
            return defkit.FromTyped(deployment)
        })
}
```

**`defkit.FromTyped()` adapter:**

```go
import "k8s.io/apimachinery/pkg/runtime"

// FromTyped converts any typed K8s object to a defkit.Resource
// This is the bridge between typed k8s.io/api usage and defkit
func FromTyped(obj runtime.Object) *Resource {
    unstructuredMap, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
    return &Resource{object: unstructuredMap}
}
```

### Approach 3: User-Generated Types (On-Demand Codegen)

For users who want type safety for specific CRDs without manually writing structs, defkit provides optional codegen from CRD schemas:

```bash
# Generate typed builders from CRD schema (runs in user's project, not shipped with defkit)
vela defkit gen-types --from-crd https://raw.githubusercontent.com/crossplane/provider-aws/main/package/crds/s3.aws.crossplane.io_buckets.yaml

# Generate from cluster-installed CRDs
vela defkit gen-types --from-cluster --api-group s3.aws.crossplane.io

# Output goes to user's project
# ./generated/s3.aws.crossplane.io/bucket.go
```

Generated code lives in **user's project**, not in defkit. Users control when to regenerate for new CRD versions.

### Design Summary

| Approach | Type Safety | K8s Version Coupling | Maintenance |
|----------|-------------|---------------------|-------------|
| Unstructured Builder (default) | Runtime only | None (defkit) | Zero |
| Typed Adapter (`k8s.io/api`) | Compile-time | User's choice | User manages |
| Generated Types (codegen) | Compile-time | User's choice | User regenerates |

**Key Principle:** defkit itself has **zero dependencies** on `k8s.io/api/*`. It only depends on:
- `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured` (for K8s integration)
- `k8s.io/apimachinery/pkg/runtime` (for typed conversion adapter)

This ensures defkit never needs updates when Kubernetes releases new versions.

---

## Part 2b: Multi-Resource Template Generation

### The Problem

Real-world X-Definitions often generate multiple Kubernetes resources, not just a single workload. The current CUE approach uses:

- **`output`**: Primary workload resource (e.g., Deployment)
- **`outputs`**: Additional resources keyed by name (e.g., Service, PVC, ConfigMap)
- **`patch`**: Modifications to apply to existing workloads (for traits)

This pattern is used extensively for:
1. **Native Kubernetes resources** - Deployment + Service + ConfigMap + PVC
2. **Crossplane resources** - CompositeResourceDefinition + Composition + managed resources
3. **KRO resources** - ResourceGroup + composed resources
4. **Custom operators** - CRDs + operator-specific resources

### Solution: Multi-Resource defkit API

The Definition Kit provides explicit support for multi-resource output patterns.

#### Go: Multi-Resource Component Definition

```go
package myplatform

import (
    "github.com/oam-dev/kubevela/pkg/defkit"
    "github.com/oam-dev/kubevela/pkg/defkit/component"
    "github.com/oam-dev/kubevela/pkg/defkit/k8s"
)

// MicroserviceComponent generates Deployment + Service + ConfigMap
func MicroserviceComponent() *component.Definition {
    return component.New("microservice").
        Description("A microservice with service discovery and configuration").
        Workload(component.Deployment()).

        // Define parameters
        Parameter("name", defkit.String().Required()).
        Parameter("image", defkit.String().Required()).
        Parameter("replicas", defkit.Int().Default(3)).
        Parameter("port", defkit.Int().Default(8080)).
        Parameter("config", defkit.Map(defkit.String()).Optional().
            Description("Configuration key-value pairs")).
        Parameter("exposeService", defkit.Bool().Default(true)).
        Parameter("serviceType", defkit.Enum("ClusterIP", "NodePort", "LoadBalancer").
            Default("ClusterIP")).

        // Primary output: Deployment (using schema-agnostic construction)
        Output(func(ctx *component.Context) *defkit.Resource {
            name := ctx.Param("name").String()
            configMapName := name + "-config"

            return defkit.NewResource("apps/v1", "Deployment").
                SetName(name).
                SetNamespace(ctx.Namespace()).
                Set("spec.replicas", ctx.Param("replicas").Int()).
                Set("spec.selector.matchLabels", map[string]string{
                    "app.oam.dev/component": ctx.ComponentName(),
                }).
                Set("spec.template.metadata.labels", map[string]string{
                    "app.oam.dev/component": ctx.ComponentName(),
                }).
                Set("spec.template.spec.containers", []map[string]any{{
                    "name":  name,
                    "image": ctx.Param("image").String(),
                    "ports": []map[string]any{{
                        "containerPort": ctx.Param("port").Int(),
                    }},
                    "envFrom": []map[string]any{{
                        "configMapRef": map[string]any{
                            "name": configMapName,
                        },
                    }},
                }})
        }).

        // Additional outputs: Service and ConfigMap (schema-agnostic)
        Outputs(func(ctx *component.Context) map[string]*defkit.Resource {
            resources := make(map[string]*defkit.Resource)
            name := ctx.Param("name").String()

            // ConfigMap (always created if config is provided)
            if ctx.Param("config").IsSet() {
                resources["configmap"] = defkit.NewResource("v1", "ConfigMap").
                    SetName(name + "-config").
                    SetNamespace(ctx.Namespace()).
                    Set("data", ctx.Param("config").StringMap())
            }

            // Service (conditionally created)
            if ctx.Param("exposeService").Bool() {
                resources["service"] = defkit.NewResource("v1", "Service").
                    SetName(name).
                    SetNamespace(ctx.Namespace()).
                    Set("spec.selector", map[string]string{
                        "app.oam.dev/component": ctx.ComponentName(),
                    }).
                    Set("spec.ports", []map[string]any{{
                        "port":       ctx.Param("port").Int(),
                        "targetPort": ctx.Param("port").Int(),
                    }}).
                    Set("spec.type", ctx.Param("serviceType").String())
            }

            return resources
        })
}
```

#### Go: Crossplane-Based Component Definition

```go
package crossplane

import (
    "github.com/oam-dev/kubevela/pkg/defkit"
    "github.com/oam-dev/kubevela/pkg/defkit/component"
)

// RDSComponent generates Crossplane AWS RDS resources
// Demonstrates schema-agnostic construction for CRDs
func RDSComponent() *component.Definition {
    return component.New("aws-rds").
        Description("Provision AWS RDS database via Crossplane").
        Workload(component.Custom("DBInstance", "database.aws.crossplane.io/v1beta1")).

        Parameter("name", defkit.String().Required()).
        Parameter("engine", defkit.Enum("mysql", "postgres", "mariadb").Default("postgres")).
        Parameter("engineVersion", defkit.String().Default("14.0")).
        Parameter("instanceClass", defkit.String().Default("db.t3.micro")).
        Parameter("storageGB", defkit.Int().Default(20).Min(20).Max(65536)).
        Parameter("multiAZ", defkit.Bool().Default(false)).
        Parameter("publiclyAccessible", defkit.Bool().Default(false)).
        Parameter("secretName", defkit.String().Required().
            Description("Name of secret to store connection credentials")).

        // Primary output: Crossplane DBInstance (schema-agnostic)
        Output(func(ctx *component.Context) *defkit.Resource {
            name := ctx.Param("name").String()

            return defkit.NewResource("database.aws.crossplane.io/v1beta1", "DBInstance").
                SetName(name).
                SetNamespace(ctx.Namespace()).
                Set("spec.forProvider", map[string]any{
                    "region":             "us-west-2",
                    "engine":             ctx.Param("engine").String(),
                    "engineVersion":      ctx.Param("engineVersion").String(),
                    "instanceClass":      ctx.Param("instanceClass").String(),
                    "allocatedStorage":   ctx.Param("storageGB").Int(),
                    "multiAZ":            ctx.Param("multiAZ").Bool(),
                    "publiclyAccessible": ctx.Param("publiclyAccessible").Bool(),
                    "skipFinalSnapshot":  true,
                    "masterUsername":     "admin",
                }).
                Set("spec.writeConnectionSecretToRef", map[string]any{
                    "name":      ctx.Param("secretName").String(),
                    "namespace": ctx.Namespace(),
                }).
                Set("spec.providerConfigRef", map[string]any{
                    "name": "aws-provider",
                })
        }).

        // Additional outputs: Security Group, Subnet Group (schema-agnostic)
        Outputs(func(ctx *component.Context) map[string]*defkit.Resource {
            name := ctx.Param("name").String()

            return map[string]*defkit.Resource{
                "securityGroup": defkit.NewResource("ec2.aws.crossplane.io/v1beta1", "SecurityGroup").
                    SetName(name + "-sg").
                    SetNamespace(ctx.Namespace()).
                    Set("spec.forProvider", map[string]any{
                        "region":      "us-west-2",
                        "description": "Security group for " + name,
                        "ingress": []map[string]any{
                            {
                                "fromPort":   5432,
                                "toPort":     5432,
                                "protocol":   "tcp",
                                "cidrBlocks": []string{"10.0.0.0/8"},
                            },
                        },
                    }),

                "subnetGroup": defkit.NewResource("database.aws.crossplane.io/v1beta1", "DBSubnetGroup").
                    SetName(name + "-subnet").
                    SetNamespace(ctx.Namespace()).
                    Set("spec.forProvider", map[string]any{
                        "region":      "us-west-2",
                        "description": "Subnet group for " + name,
                        "subnetIdSelector": map[string]any{
                            "matchLabels": map[string]string{
                                "access": "private",
                            },
                        },
                    }),
            }
        })
}
```

#### Go: KRO-Based Component Definition

```go
package kro

import (
    "github.com/oam-dev/kubevela/pkg/defkit"
    "github.com/oam-dev/kubevela/pkg/defkit/component"
)

// ApplicationStackComponent generates KRO ResourceGroup
// Demonstrates schema-agnostic construction for KRO CRDs
func ApplicationStackComponent() *component.Definition {
    return component.New("app-stack").
        Description("Full application stack via KRO ResourceGroup").
        Workload(component.Custom("ResourceGroup", "kro.run/v1alpha1")).

        Parameter("name", defkit.String().Required()).
        Parameter("image", defkit.String().Required()).
        Parameter("replicas", defkit.Int().Default(3)).
        Parameter("dbEnabled", defkit.Bool().Default(true)).
        Parameter("cacheEnabled", defkit.Bool().Default(false)).

        // KRO ResourceGroup as primary output (schema-agnostic)
        Output(func(ctx *component.Context) *defkit.Resource {
            name := ctx.Param("name").String()

            resources := []map[string]any{
                // Deployment
                {
                    "id": "deployment",
                    "template": map[string]any{
                        "apiVersion": "apps/v1",
                        "kind":       "Deployment",
                        "metadata": map[string]any{
                            "name": "${schema.spec.name}",
                        },
                        "spec": map[string]any{
                            "replicas": "${schema.spec.replicas}",
                            "selector": map[string]any{
                                "matchLabels": map[string]string{
                                    "app": "${schema.spec.name}",
                                },
                            },
                            "template": map[string]any{
                                "metadata": map[string]any{
                                    "labels": map[string]string{
                                        "app": "${schema.spec.name}",
                                    },
                                },
                                "spec": map[string]any{
                                    "containers": []map[string]any{
                                        {
                                            "name":  "${schema.spec.name}",
                                            "image": "${schema.spec.image}",
                                        },
                                    },
                                },
                            },
                        },
                    },
                },
                // Service
                {
                    "id": "service",
                    "template": map[string]any{
                        "apiVersion": "v1",
                        "kind":       "Service",
                        "metadata": map[string]any{
                            "name": "${schema.spec.name}",
                        },
                        "spec": map[string]any{
                            "selector": map[string]string{
                                "app": "${schema.spec.name}",
                            },
                            "ports": []map[string]any{
                                {"port": 80, "targetPort": 8080},
                            },
                        },
                    },
                },
            }

            // Conditionally add database
            if ctx.Param("dbEnabled").Bool() {
                resources = append(resources, map[string]any{
                    "id":        "database",
                    "dependsOn": []string{"deployment"},
                    "template": map[string]any{
                        "apiVersion": "database.example.com/v1",
                        "kind":       "PostgreSQL",
                        "metadata": map[string]any{
                            "name": "${schema.spec.name}-db",
                        },
                    },
                })
            }

            return defkit.NewResource("kro.run/v1alpha1", "ResourceGroup").
                SetName(name + "-rg").
                SetNamespace(ctx.Namespace()).
                Set("spec.schema", map[string]any{
                    "apiVersion": "example.com/v1alpha1",
                    "kind":       "ApplicationStack",
                    "spec": map[string]any{
                        "name":     name,
                        "image":    ctx.Param("image").String(),
                        "replicas": ctx.Param("replicas").Int(),
                    },
                }).
                Set("spec.resources", resources)
        })
}
```

#### Go: Trait with Patch and Outputs

```go
package myplatform

import (
    "github.com/oam-dev/kubevela/pkg/defkit"
    "github.com/oam-dev/kubevela/pkg/defkit/trait"
)

// StorageTrait adds PVCs and mounts to a workload
// Demonstrates schema-agnostic PVC creation
func StorageTrait() *trait.Definition {
    return trait.New("storage").
        Description("Add persistent storage to workload").
        AppliesTo("deployments.apps", "statefulsets.apps").

        Parameter("pvc", defkit.Array(defkit.Struct(
            defkit.Field("name", defkit.String().Required()),
            defkit.Field("mountPath", defkit.String().Required()),
            defkit.Field("storageClass", defkit.String().Optional()),
            defkit.Field("size", defkit.String().Default("10Gi")),
            defkit.Field("accessMode", defkit.Enum("ReadWriteOnce", "ReadWriteMany", "ReadOnlyMany").
                Default("ReadWriteOnce")),
            defkit.Field("mountOnly", defkit.Bool().Default(false).
                Description("If true, use existing PVC instead of creating new one")),
        )).Required()).

        // Patch: Modify the workload to add volumes and mounts
        Patch(func(ctx *trait.Context) *trait.Patches {
            volumes := []map[string]any{}
            mounts := []map[string]any{}

            for _, pvc := range ctx.Param("pvc").Array() {
                name := pvc.Get("name").String()
                volumes = append(volumes, map[string]any{
                    "name": "pvc-" + name,
                    "persistentVolumeClaim": map[string]any{
                        "claimName": name,
                    },
                })
                mounts = append(mounts, map[string]any{
                    "name":      "pvc-" + name,
                    "mountPath": pvc.Get("mountPath").String(),
                })
            }

            return ctx.PatchWorkload().
                AddVolumes(volumes).
                AddVolumeMountsToContainers(mounts)
        }).

        // Outputs: Create PVCs for storage that doesn't exist (schema-agnostic)
        Outputs(func(ctx *trait.Context) map[string]*defkit.Resource {
            resources := make(map[string]*defkit.Resource)

            for _, pvc := range ctx.Param("pvc").Array() {
                // Skip if mountOnly=true (use existing PVC)
                if pvc.Get("mountOnly").Bool() {
                    continue
                }

                name := pvc.Get("name").String()
                pvcResource := defkit.NewResource("v1", "PersistentVolumeClaim").
                    SetName(name).
                    SetNamespace(ctx.Namespace()).
                    Set("spec.accessModes", []string{pvc.Get("accessMode").String()}).
                    Set("spec.resources.requests.storage", pvc.Get("size").String())

                // Add storage class if specified
                if storageClass := pvc.Get("storageClass").StringOrNil(); storageClass != nil {
                    pvcResource.Set("spec.storageClassName", *storageClass)
                }

                resources["pvc-"+name] = pvcResource
            }

            return resources
        })
}
```

#### TypeScript: Multi-Resource Component

```typescript
import { component, defkit } from '@kubevela/defkit';

export const MicroserviceComponent = component.new('microservice')
  .description('A microservice with service discovery and configuration')
  .workload(component.deployment())

  .parameter('name', defkit.string().required())
  .parameter('image', defkit.string().required())
  .parameter('replicas', defkit.int().default(3))
  .parameter('port', defkit.int().default(8080))
  .parameter('config', defkit.map(defkit.string()).optional())
  .parameter('exposeService', defkit.bool().default(true))
  .parameter('serviceType', defkit.enum('ClusterIP', 'NodePort', 'LoadBalancer').default('ClusterIP'))

  // Primary output: Deployment (schema-agnostic)
  .output((ctx) => {
    const name = ctx.param('name').string();
    return defkit.newResource('apps/v1', 'Deployment')
      .setName(name)
      .setNamespace(ctx.namespace())
      .set('spec.replicas', ctx.param('replicas').int())
      .set('spec.selector.matchLabels', {
        'app.oam.dev/component': ctx.componentName(),
      })
      .set('spec.template.metadata.labels', {
        'app.oam.dev/component': ctx.componentName(),
      })
      .set('spec.template.spec.containers', [{
        name,
        image: ctx.param('image').string(),
        ports: [{ containerPort: ctx.param('port').int() }],
        envFrom: [{ configMapRef: { name: name + '-config' } }],
      }]);
  })

  // Additional outputs (schema-agnostic)
  .outputs((ctx) => {
    const resources: Record<string, defkit.Resource> = {};
    const name = ctx.param('name').string();

    // ConfigMap
    if (ctx.param('config').isSet()) {
      resources['configmap'] = defkit.newResource('v1', 'ConfigMap')
        .setName(name + '-config')
        .setNamespace(ctx.namespace())
        .set('data', ctx.param('config').stringMap());
    }

    // Service
    if (ctx.param('exposeService').bool()) {
      resources['service'] = defkit.newResource('v1', 'Service')
        .setName(name)
        .setNamespace(ctx.namespace())
        .set('spec.selector', { 'app.oam.dev/component': ctx.componentName() })
        .set('spec.ports', [{ port: ctx.param('port').int(), targetPort: ctx.param('port').int() }])
        .set('spec.type', ctx.param('serviceType').string());
    }

    return resources;
  });
```

#### Java: Multi-Resource Component (Java 21 LTS / OpenJDK)

```java
package com.myplatform.definitions;

import io.kubevela.defkit.*;
import io.kubevela.defkit.component.*;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

public class MicroserviceDefinition {

    public static ComponentDefinition create() {
        return Component.create("microservice")
            .description("A microservice with service discovery and configuration")
            .workload(Component.deployment())

            .parameter("name", Defkit.string().required())
            .parameter("image", Defkit.string().required())
            .parameter("replicas", Defkit.integer().defaultValue(3))
            .parameter("port", Defkit.integer().defaultValue(8080))
            .parameter("config", Defkit.map(Defkit.string()).optional())
            .parameter("exposeService", Defkit.bool().defaultValue(true))
            .parameter("serviceType",
                Defkit.enumeration("ClusterIP", "NodePort", "LoadBalancer")
                    .defaultValue("ClusterIP"))

            // Primary output: Deployment (schema-agnostic)
            .output(ctx -> {
                var name = ctx.param("name").asString();
                return Defkit.newResource("apps/v1", "Deployment")
                    .setName(name)
                    .setNamespace(ctx.namespace())
                    .set("spec.replicas", ctx.param("replicas").asInt())
                    .set("spec.selector.matchLabels", Map.of(
                        "app.oam.dev/component", ctx.componentName()
                    ))
                    .set("spec.template.metadata.labels", Map.of(
                        "app.oam.dev/component", ctx.componentName()
                    ))
                    .set("spec.template.spec.containers", List.of(Map.of(
                        "name", name,
                        "image", ctx.param("image").asString(),
                        "ports", List.of(Map.of("containerPort", ctx.param("port").asInt())),
                        "envFrom", List.of(Map.of("configMapRef", Map.of("name", name + "-config")))
                    )));
            })

            // Additional outputs (schema-agnostic)
            .outputs(ctx -> {
                Map<String, Resource> resources = new HashMap<>();
                var name = ctx.param("name").asString();

                // ConfigMap
                if (ctx.param("config").isSet()) {
                    resources.put("configmap", Defkit.newResource("v1", "ConfigMap")
                        .setName(name + "-config")
                        .setNamespace(ctx.namespace())
                        .set("data", ctx.param("config").asStringMap()));
                }

                // Service
                if (ctx.param("exposeService").asBool()) {
                    resources.put("service", Defkit.newResource("v1", "Service")
                        .setName(name)
                        .setNamespace(ctx.namespace())
                        .set("spec.selector", Map.of("app.oam.dev/component", ctx.componentName()))
                        .set("spec.ports", List.of(Map.of(
                            "port", ctx.param("port").asInt(),
                            "targetPort", ctx.param("port").asInt()
                        )))
                        .set("spec.type", ctx.param("serviceType").asString()));
                }

                return resources;
            })
            .build();
    }
}
```

### Template API Reference

#### Context Methods

| Method | Description |
|--------|-------------|
| `ctx.Param(name)` | Get parameter value |
| `ctx.ComponentName()` | Current component name |
| `ctx.AppName()` | Current application name |
| `ctx.Namespace()` | Target namespace |
| `ctx.Revision()` | Current revision |

#### Resource Builders (Schema-Agnostic)

> **Design Decision**: defkit does NOT ship typed Kubernetes helpers. See [Part 2a: Schema-Agnostic Resource Construction](#part-2a-schema-agnostic-resource-construction) for rationale.

| Builder | Usage |
|---------|-------|
| `defkit.NewResource(apiVersion, kind)` | Create any K8s resource (core, CRDs, custom) |
| `.SetName(name)` | Set resource metadata.name |
| `.SetNamespace(ns)` | Set resource metadata.namespace |
| `.SetLabels(labels)` | Set metadata.labels |
| `.SetAnnotations(ann)` | Set metadata.annotations |
| `.Set(path, value)` | Set any field by dot-notation path |
| `.Get(path)` | Get field value by path |
| `.Unstructured()` | Convert to `*unstructured.Unstructured` |
| `.MarshalJSON()` | Serialize to JSON |

**Examples:**

```go
// Core K8s resources
defkit.NewResource("apps/v1", "Deployment")
defkit.NewResource("v1", "Service")
defkit.NewResource("v1", "ConfigMap")

// CRDs (Crossplane, KRO, etc.)
defkit.NewResource("database.aws.crossplane.io/v1beta1", "DBInstance")
defkit.NewResource("kro.run/v1alpha1", "ResourceGroup")

// Dynamic API versioning
defkit.NewResource(apiVersion, "Ingress")  // apiVersion can be v1 or v1beta1
```

#### Patch Methods (Traits)

| Method | Description |
|--------|-------------|
| `ctx.PatchWorkload()` | Start patching the workload |
| `.AddVolumes(vols)` | Add volumes to pod spec |
| `.AddVolumeMountsToContainers(mounts)` | Add mounts to all containers |
| `.AddAnnotation(key, val)` | Add annotation |
| `.AddLabel(key, val)` | Add label |
| `.MergePatch(patch)` | Apply JSON merge patch |
| `.StrategicMergePatch(patch)` | Apply strategic merge patch |

---

## Part 2c: PolicyDefinition and WorkflowStepDefinition

### PolicyDefinition defkit API

Policies operate at the application level, affecting multiple components. They can either generate resources or control the delivery process.

#### Go: Policy Definition

```go
package policies

import (
    "github.com/oam-dev/kubevela/pkg/defkit"
    "github.com/oam-dev/kubevela/pkg/defkit/policy"
)

// TrafficSplitPolicy generates service mesh traffic splitting resources
// Demonstrates schema-agnostic construction for SMI CRDs
func TrafficSplitPolicy() *policy.Definition {
    return policy.New("traffic-split").
        Description("Split traffic between service versions using service mesh").

        Parameter("weights", defkit.Array(defkit.Struct(
            defkit.Field("service", defkit.String().Required()),
            defkit.Field("weight", defkit.Int().Required().Min(0).Max(100)),
        )).Required().Description("Service weights for traffic distribution")).

        // Policy output - operates at application level (schema-agnostic)
        Output(func(ctx *policy.Context) *defkit.Resource {
            return defkit.NewResource("split.smi-spec.io/v1alpha3", "TrafficSplit").
                SetName(ctx.AppName()).
                SetNamespace(ctx.Namespace()).
                Set("spec.service", ctx.AppName()).
                Set("spec.backends", ctx.Param("weights").Array())
        })
}

// TopologyPolicy is a built-in policy that controls delivery (no resource output)
func TopologyPolicy() *policy.Definition {
    return policy.New("topology").
        Description("Describe the destination where components should be deployed").
        BuiltIn(). // Marks as built-in policy processed by controller

        Parameter("clusters", defkit.Array(defkit.String()).Optional().
            Description("Names of clusters to deploy to")).
        Parameter("clusterLabelSelector", defkit.Map(defkit.String()).Optional().
            Description("Label selector for clusters")).
        Parameter("namespace", defkit.String().Optional().
            Description("Target namespace in selected clusters")).
        Parameter("allowEmpty", defkit.Bool().Default(false).
            Description("Ignore empty cluster error"))
}
```

#### TypeScript: Policy Definition

```typescript
import { policy, defkit } from '@kubevela/defkit';

export const TrafficSplitPolicy = policy.new('traffic-split')
  .description('Split traffic between service versions')

  .parameter('weights', defkit.array(defkit.struct({
    service: defkit.string().required(),
    weight: defkit.int().required().min(0).max(100),
  })).required())

  // Schema-agnostic policy output
  .output((ctx) => defkit.newResource('split.smi-spec.io/v1alpha3', 'TrafficSplit')
    .setName(ctx.appName())
    .setNamespace(ctx.namespace())
    .set('spec.service', ctx.appName())
    .set('spec.backends', ctx.param('weights').array())
  );
```

### WorkflowStepDefinition defkit API

Workflow steps execute operations during application delivery. They use KubeVela's built-in operations for resource management, HTTP requests, notifications, and process control.

#### Go: Workflow Step Definition

```go
package workflows

import (
    "github.com/oam-dev/kubevela/pkg/defkit"
    "github.com/oam-dev/kubevela/pkg/defkit/workflow"
    "github.com/oam-dev/kubevela/pkg/defkit/workflow/op"
)

// HTTPRequestStep makes HTTP requests and captures responses
func HTTPRequestStep() *workflow.StepDefinition {
    return workflow.NewStep("http-request").
        Description("Send HTTP request to external service").
        Category("External Integration").

        Parameter("url", defkit.String().Required().
            Description("URL to send request to")).
        Parameter("method", defkit.Enum("GET", "POST", "PUT", "DELETE").Default("GET")).
        Parameter("body", defkit.Any().Optional().
            Description("Request body")).
        Parameter("headers", defkit.Map(defkit.String()).Optional()).
        Parameter("timeout", defkit.String().Default("30s")).

        // Define step execution
        Execute(func(ctx *workflow.Context) *workflow.Result {
            // HTTP request operation
            response := ctx.Op(op.HTTPDo{
                Method:  ctx.Param("method").String(),
                URL:     ctx.Param("url").String(),
                Body:    ctx.Param("body").Any(),
                Headers: ctx.Param("headers").StringMap(),
                Timeout: ctx.Param("timeout").String(),
            })

            // Conditional failure
            if response.StatusCode() >= 400 {
                return ctx.Fail("HTTP request failed with status: %d", response.StatusCode())
            }

            // Output response for downstream steps
            return ctx.Success().
                Output("response", response.Body()).
                Output("statusCode", response.StatusCode())
        })
}

// ApprovalStep waits for manual approval
func ApprovalStep() *workflow.StepDefinition {
    return workflow.NewStep("approval").
        Description("Wait for manual approval before proceeding").
        Category("Process Control").

        Parameter("message", defkit.String().Default("Waiting for approval")).
        Parameter("approvers", defkit.Array(defkit.String()).Optional()).

        Execute(func(ctx *workflow.Context) *workflow.Result {
            // Suspend until manually approved
            return ctx.Op(op.Suspend{
                Message: ctx.Param("message").String(),
            })
        })
}

// DeployWithHealthCheckStep deploys and waits for health
func DeployWithHealthCheckStep() *workflow.StepDefinition {
    return workflow.NewStep("deploy-healthy").
        Description("Deploy component and wait for healthy status").
        Category("Application Delivery").

        Parameter("component", defkit.String().Required()).
        Parameter("timeout", defkit.String().Default("5m")).
        Parameter("healthCheck", defkit.Bool().Default(true)).

        Execute(func(ctx *workflow.Context) *workflow.Result {
            // Load component from application
            comp := ctx.Op(op.Load{
                Component: ctx.Param("component").String(),
            })

            // Apply component resources
            ctx.Op(op.ApplyComponent{
                Component:  comp,
                WaitHealth: ctx.Param("healthCheck").Bool(),
                Timeout:    ctx.Param("timeout").String(),
            })

            // Log success
            ctx.Op(op.Log{
                Data:  "Component deployed successfully",
                Level: 2,
            })

            return ctx.Success().
                Output("resources", comp.Resources())
        })
}

// NotificationStep sends notifications to various channels
func NotificationStep() *workflow.StepDefinition {
    return workflow.NewStep("notification").
        Description("Send notifications to Slack, DingTalk, Lark, or Email").
        Category("External Integration").

        Parameter("slack", defkit.Struct(
            defkit.Field("url", defkit.SecretRef().Required()),
            defkit.Field("message", defkit.Struct(
                defkit.Field("text", defkit.String().Required()),
            ).Required()),
        ).Optional()).

        Parameter("email", defkit.Struct(
            defkit.Field("from", defkit.Struct(
                defkit.Field("address", defkit.String().Required()),
                defkit.Field("password", defkit.SecretRef().Required()),
                defkit.Field("host", defkit.String().Required()),
                defkit.Field("port", defkit.Int().Default(587)),
            ).Required()),
            defkit.Field("to", defkit.Array(defkit.String()).Required()),
            defkit.Field("subject", defkit.String().Required()),
            defkit.Field("body", defkit.String().Required()),
        ).Optional()).

        Execute(func(ctx *workflow.Context) *workflow.Result {
            if ctx.Param("slack").IsSet() {
                slack := ctx.Param("slack")
                ctx.Op(op.HTTPPost{
                    URL:  slack.Get("url").SecretValue(),
                    Body: slack.Get("message").JSON(),
                    Headers: map[string]string{
                        "Content-Type": "application/json",
                    },
                })
            }

            if ctx.Param("email").IsSet() {
                email := ctx.Param("email")
                ctx.Op(op.SendEmail{
                    From: op.EmailFrom{
                        Address:  email.Get("from.address").String(),
                        Password: email.Get("from.password").SecretValue(),
                        Host:     email.Get("from.host").String(),
                        Port:     email.Get("from.port").Int(),
                    },
                    To:      email.Get("to").StringArray(),
                    Subject: email.Get("subject").String(),
                    Body:    email.Get("body").String(),
                })
            }

            return ctx.Success()
        })
}
```

### Workflow Operations Reference

The defkit workflow package provides these built-in operations:

| Operation | Description | Parameters |
|-----------|-------------|------------|
| `op.Apply` | Create/update K8s resource | `value`, `cluster` |
| `op.ApplyInParallel` | Apply multiple resources concurrently | `values[]` |
| `op.Read` | Read K8s resource | `value`, `cluster` |
| `op.List` | List K8s resources | `resource`, `filter` |
| `op.Delete` | Delete K8s resource | `value`, `cluster` |
| `op.Load` | Load application components | `component`, `app` |
| `op.ApplyComponent` | Deploy component with health wait | `component`, `waitHealth`, `timeout` |
| `op.ConditionalWait` | Wait for condition | `continue`, `message` |
| `op.Suspend` | Pause workflow | `message` |
| `op.Fail` | Fail step with message | `message` |
| `op.Log` | Write to workflow logs | `data`, `level` |
| `op.Message` | Set step status message | `message` |
| `op.DoVar` | Get/Put context variables | `method`, `path`, `value` |
| `op.HTTPDo` | Send HTTP request | `method`, `url`, `body`, `headers` |
| `op.HTTPGet/Post/Put/Delete` | Specialized HTTP methods | `url`, `body`, `headers` |
| `op.SendEmail` | Send email | `from`, `to`, `content` |
| `op.Steps` | Conditional step grouping | `steps[]` |

### Workflow Context Variables

| Variable | Type | Description |
|----------|------|-------------|
| `ctx.Name()` | string | Application name |
| `ctx.Namespace()` | string | Application namespace |
| `ctx.StepName()` | string | Current step name |
| `ctx.StepSessionID()` | string | Unique step session ID |
| `ctx.WorkflowName()` | string | Workflow name |
| `ctx.SpanID()` | string | Trace span ID |
| `ctx.PublishVersion()` | string | Current publish version |

### Workflow Inputs/Outputs

Steps can pass data between each other:

```go
// Step 1: Produce output
func FetchDataStep() *workflow.StepDefinition {
    return workflow.NewStep("fetch-data").
        Execute(func(ctx *workflow.Context) *workflow.Result {
            data := ctx.Op(op.HTTPGet{URL: "https://api.example.com/data"})
            return ctx.Success().
                Output("fetchedData", data.Body())
        })
}

// Step 2: Consume input from previous step
func ProcessDataStep() *workflow.StepDefinition {
    return workflow.NewStep("process-data").
        Input("data", "fetchedData"). // Maps from previous step output
        Execute(func(ctx *workflow.Context) *workflow.Result {
            data := ctx.Input("data")
            // Process data...
            return ctx.Success()
        })
}
```

---

## Part 2d: Advanced Patterns

### Status and Health Handling

Components and traits can define custom status messages and health policies:

```go
func WebserviceComponent() *component.Definition {
    return component.New("webservice").
        Workload(component.Deployment()).

        // Custom status message shown to users
        CustomStatus(func(ctx *component.StatusContext) string {
            ready := ctx.Output().Status().Get("readyReplicas").IntOr(0)
            total := ctx.Output().Spec().Get("replicas").Int()
            return fmt.Sprintf("Ready: %d/%d", ready, total)
        }).

        // Health policy determines if component is healthy
        HealthPolicy(func(ctx *component.StatusContext) bool {
            status := ctx.Output().Status()
            ready := status.Get("readyReplicas").IntOr(0)
            updated := status.Get("updatedReplicas").IntOr(0)
            replicas := ctx.Output().Spec().Get("replicas").Int()
            generation := ctx.Output().Metadata().Get("generation").Int()
            observed := status.Get("observedGeneration").IntOr(0)

            return ready == replicas &&
                   updated == replicas &&
                   observed >= generation
        }).

        // ... rest of definition
}
```

### Patch Strategies for Traits

Traits can use three patch strategies via annotations:

```go
func EnvTrait() *trait.Definition {
    return trait.New("env").
        Description("Add environment variables to containers").
        AppliesTo("*"). // All workloads

        Parameter("env", defkit.Map(defkit.String()).Required()).
        Parameter("containerName", defkit.String().Optional().
            Description("Target specific container")).
        Parameter("replace", defkit.Bool().Default(false).
            Description("Replace all env vars instead of merging")).

        Patch(func(ctx *trait.Context) *trait.Patches {
            envVars := []map[string]interface{}{}
            for k, v := range ctx.Param("env").StringMap() {
                envVars = append(envVars, map[string]interface{}{
                    "name": k, "value": v,
                })
            }

            containerName := ctx.Param("containerName").StringOr(ctx.ComponentName())

            patch := ctx.PatchWorkload().
                Path("spec.template.spec.containers").
                PatchKey("name"). // +patchKey=name
                Match(containerName)

            if ctx.Param("replace").Bool() {
                // +patchStrategy=replace - replaces entire array
                return patch.Replace("env", envVars)
            } else {
                // +patchStrategy=retainKeys - merges, overwrites duplicates
                return patch.RetainKeys("env", envVars)
            }
        })
}
```

### Patch Strategy Reference

| Strategy | Annotation | Behavior |
|----------|------------|----------|
| Default | (none) | CUE merge - fails on conflicts |
| `PatchKey` | `+patchKey=field` | Find array element by field value |
| `RetainKeys` | `+patchStrategy=retainKeys` | Merge and overwrite duplicates |
| `Replace` | `+patchStrategy=replace` | Replace entire array |

### Trait Attributes

```go
func MyTrait() *trait.Definition {
    return trait.New("my-trait").
        // Which workloads this trait can attach to
        AppliesTo("deployments.apps", "statefulsets.apps").
        // Or use wildcards
        // AppliesTo("*").
        // Or use specific component definitions
        // AppliesTo("webservice", "worker").

        // Traits that conflict with this one
        ConflictsWith("other-trait", "legacy-trait").

        // Whether trait updates cause pod restarts
        PodDisruptive(false).

        // Stage in deployment lifecycle
        Stage(trait.PreDispatch). // or PostDispatch

        // Auto-inject workload reference (for reading workload metadata)
        WorkloadRefPath("spec.workloadRef").

        // ... parameters and patch/outputs
}
```

### Secret Reference Pattern

For sensitive data, use `SecretRef` instead of plain strings:

```go
func DatabaseComponent() *component.Definition {
    return component.New("database").
        Parameter("name", defkit.String().Required()).
        Parameter("image", defkit.String().Required()).
        Parameter("password", defkit.SecretRef().Required().
            Description("Database password from secret")).
        Parameter("connectionString", defkit.OneOf(
            defkit.String(),  // Plain value
            defkit.SecretRef(), // Or from secret
        ).Description("Connection string")).

        Template(func(ctx *component.Context) *component.Output {
            // SecretRef resolves at runtime
            password := ctx.Param("password") // Returns SecretKeyRef structure
            name := ctx.Param("name").String()

            // Schema-agnostic resource construction
            deploy := defkit.NewResource("apps/v1", "Deployment").
                SetName(name).
                SetNamespace(ctx.Namespace()).
                Set("spec.replicas", 1).
                Set("spec.selector.matchLabels", map[string]string{
                    "app.oam.dev/component": ctx.ComponentName(),
                }).
                Set("spec.template.metadata.labels", map[string]string{
                    "app.oam.dev/component": ctx.ComponentName(),
                }).
                Set("spec.template.spec.containers", []map[string]any{{
                    "name":  name,
                    "image": ctx.Param("image").String(),
                    "env": []map[string]any{{
                        "name": "DB_PASSWORD",
                        "valueFrom": map[string]any{
                            "secretKeyRef": map[string]any{
                                "name": password.SecretName(),
                                "key":  password.Key(),
                            },
                        },
                    }},
                }})
            return ctx.Output(deploy)
        })
}
```

SecretRef parameter in Application YAML:

```yaml
properties:
  password:
    secretRef:
      name: db-credentials
      key: password
```

### Complete Context Variables Reference

#### Component Context

| Variable | Description |
|----------|-------------|
| `ctx.Name()` | Component name |
| `ctx.AppName()` | Application name |
| `ctx.Namespace()` | Target namespace |
| `ctx.Revision()` | Application revision |
| `ctx.Cluster()` | Target cluster |
| `ctx.ClusterVersion()` | K8s cluster version info |
| `ctx.Config()` | Config from `config` section |

#### Trait Context (extends Component)

| Variable | Description |
|----------|-------------|
| `ctx.Output()` | Rendered workload resource |
| `ctx.Outputs(name)` | Named auxiliary resources |

#### Policy Context

| Variable | Description |
|----------|-------------|
| `ctx.AppName()` | Application name |
| `ctx.Namespace()` | Application namespace |
| `ctx.Components()` | All application components |

#### Workflow Context

| Variable | Description |
|----------|-------------|
| `ctx.Name()` | Application name |
| `ctx.Namespace()` | Application namespace |
| `ctx.StepName()` | Current step name |
| `ctx.StepSessionID()` | Step session ID |
| `ctx.WorkflowName()` | Workflow name |
| `ctx.SpanID()` | Trace span ID |
| `ctx.PublishVersion()` | Publish version |

### Cluster Version Handling

Support different K8s API versions dynamically with schema-agnostic resource construction:

```go
func IngressTrait() *trait.Definition {
    return trait.New("ingress").
        Parameter("host", defkit.String().Required()).
        Parameter("path", defkit.String().Default("/")).
        Parameter("port", defkit.Int().Default(80)).

        Outputs(func(ctx *trait.Context) map[string]*defkit.Resource {
            // Use appropriate API version based on cluster
            apiVersion := "networking.k8s.io/v1"
            if ctx.ClusterVersion().Minor() < 19 {
                apiVersion = "networking.k8s.io/v1beta1"
            }

            // Schema-agnostic construction works with any API version
            return map[string]*defkit.Resource{
                "ingress": defkit.NewResource(apiVersion, "Ingress").
                    SetName(ctx.ComponentName()).
                    SetNamespace(ctx.Namespace()).
                    Set("spec.rules", []map[string]any{{
                        "host": ctx.Param("host").String(),
                        "http": map[string]any{
                            "paths": []map[string]any{{
                                "path":     ctx.Param("path").String(),
                                "pathType": "Prefix",
                                "backend": map[string]any{
                                    "service": map[string]any{
                                        "name": ctx.ComponentName(),
                                        "port": map[string]any{
                                            "number": ctx.Param("port").Int(),
                                        },
                                    },
                                },
                            }},
                        },
                    }}),
            }
        })
}
```

---

## Part 3: Transparent Compilation

### Design Principle: CUE as Implementation Detail

Developers should never need to interact with CUE directly. The KubeVela CLI and controller handle all compilation transparently.

```
┌─────────────────────────────────────────────────────────────────┐
│                    Developer's Perspective                       │
│                                                                  │
│   Go/TS/Python Code ──▶ vela def apply ──▶ Definition Active    │
│                                                                  │
│   (CUE compilation is completely invisible)                      │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                    Under the Hood (Automatic)                    │
│                                                                  │
│   Native Code ──▶ IR (JSON) ──▶ CUE ──▶ X-Definition CR        │
│                                                                  │
│   Handled by: vela def apply / KubeVela Controller              │
└─────────────────────────────────────────────────────────────────┘
```

### CLI Commands for Definition Authoring

#### Apply a Definition (Transparent Compilation)

```bash
# Apply a Go definition - CUE compilation happens automatically
vela def apply ./myplatform/microservice.go

# Apply a TypeScript definition
vela def apply ./definitions/rate-limit.ts

# Apply a Python definition
vela def apply ./definitions/observability.py

# Apply from a directory (discovers all definitions)
vela def apply ./platform-definitions/

# Apply with dry-run to see generated YAML
vela def apply ./myplatform/microservice.go --dry-run
```

#### Definition Development Mode

```bash
# Watch mode - auto-applies on file changes
vela def develop ./platform-definitions/

# Watch mode with validation
vela def develop ./platform-definitions/ --validate

# Watch specific file
vela def develop ./myplatform/microservice.go
```

### Controller-Based Compilation

For GitOps workflows, the KubeVela controller can compile definitions on the fly:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: DefinitionSource
metadata:
  name: platform-definitions
spec:
  # Source from OCI registry
  source:
    type: oci
    url: oci://registry.example.com/platform/definitions:v1.0.0

  # Or source from Git
  source:
    type: git
    url: https://github.com/myorg/platform-definitions
    path: ./definitions
    branch: main

  # Language hint (auto-detected if omitted)
  language: go

  # Compilation is automatic - no CUE knowledge required
```

### Optional: Manual CUE Access

For advanced users who want direct CUE access:

```bash
# Export to CUE (for inspection or manual editing)
vela def export ./myplatform/microservice.go --format cue

# Import existing CUE definitions into native SDK
vela def import ./legacy-definitions/*.cue --language go --output ./converted/

# Validate CUE output without applying
vela def compile ./myplatform/microservice.go --output ./generated.cue
```

---

## Part 4: Always-in-Sync Architecture

### Live Introspection

The SDK generates types by introspecting the actual definitions available in a KubeVela cluster or registry. This ensures the SDK always matches reality.

```
┌──────────────────┐     Introspect     ┌──────────────────┐
│                  │ ◀──────────────────│                  │
│  KubeVela        │                    │  SDK Generator   │
│  Control Plane   │ ──────────────────▶│                  │
│                  │     Definitions    │                  │
└──────────────────┘                    └────────┬─────────┘
                                                 │
                                                 │ Generate
                                                 ▼
                                        ┌──────────────────┐
                                        │  Type-Safe SDK   │
                                        │  (Go/TS/Python)  │
                                        └──────────────────┘
```

### Definition Sources

The SDK can generate from multiple sources:

#### 1. Cluster Registry (Live)

```bash
# Generate SDK from running cluster
vela sdk generate \
  --from cluster \
  --kubeconfig ~/.kube/config \
  --language go \
  --output ./sdk

# Include only specific namespaces
vela sdk generate \
  --from cluster \
  --namespaces default,platform-system \
  --language typescript \
  --output ./sdk
```

#### 2. OCI Registry

```bash
# Generate from OCI-stored definitions
vela sdk generate \
  --from oci://registry.example.com/kubevela/definitions:v1.4.0 \
  --language go \
  --output ./sdk

# Generate from multiple OCI sources
vela sdk generate \
  --from oci://ghcr.io/kubevela/definitions:v1.4.0 \
  --from oci://registry.example.com/custom-definitions:v2.0.0 \
  --language python \
  --output ./sdk
```

#### 3. Local Directory

```bash
# Generate from local definition files
vela sdk generate \
  --from ./definitions \
  --language go \
  --output ./sdk

# Generate from CUE definitions
vela sdk generate \
  --from ./cue-definitions \
  --language typescript \
  --output ./sdk
```

#### 4. KubeVela Release

```bash
# Generate from a specific KubeVela release
vela sdk generate \
  --from release:v1.9.0 \
  --language go \
  --output ./sdk

# Generate for latest release
vela sdk generate \
  --from release:latest \
  --language typescript \
  --output ./sdk
```

### Version Manifest

Every generated SDK includes a version manifest for traceability:

```yaml
# .vela-sdk-manifest.yaml (auto-generated)
apiVersion: sdk.oam.dev/v1
kind: SDKManifest
metadata:
  generatedAt: "2024-01-15T10:30:00Z"
  generator: "vela-sdk-gen/v1.9.0"
spec:
  kubevelaVersion: "1.9.0"
  language: "go"
  sources:
    - type: release
      ref: "v1.9.0"
      checksum: "sha256:abc123..."
    - type: oci
      ref: "oci://registry.example.com/custom-definitions:v2.0.0"
      checksum: "sha256:def456..."
  definitions:
    components:
      - name: webservice
        version: "1.9.0"
        checksum: "sha256:111..."
      - name: microservice
        version: "2.0.0"
        source: "custom-definitions"
        checksum: "sha256:222..."
    traits:
      - name: scaler
        version: "1.9.0"
        checksum: "sha256:333..."
```

### Watch Mode for Development

```bash
# Watch cluster for definition changes, regenerate SDK automatically
vela sdk develop \
  --from cluster \
  --language go \
  --output ./sdk \
  --watch

# Watch with hot-reload support (for IDEs)
vela sdk develop \
  --from cluster \
  --language typescript \
  --output ./sdk \
  --watch \
  --notify-ide
```

### Compatibility Checking

```bash
# Check if SDK is compatible with current cluster
vela sdk check ./sdk

# Output:
# ✓ SDK version: 1.9.0
# ✓ Cluster version: 1.9.0
# ✓ All 23 definitions matched
# ⚠ 2 custom definitions not in SDK (use --from cluster to include)

# Update SDK to match cluster
vela sdk sync ./sdk --from cluster
```

---

## Part 5: SDK Usage Examples

### Example 1: Go SDK Usage

```go
package main

import (
    "context"
    "fmt"

    vela "github.com/kubevela/sdk-go"
    "github.com/kubevela/sdk-go/components"
    "github.com/kubevela/sdk-go/traits"
    "github.com/kubevela/sdk-go/policies"
    "github.com/kubevela/sdk-go/workflow"
)

func main() {
    // Create a complex application with fluent API
    app := vela.NewApplication("my-app").
        Namespace("production").

        // Add frontend component
        AddComponent(
            components.WebService("frontend").
                Image("nginx:1.21").
                Port(80).
                Replicas(3).
                CPU("500m").
                Memory("512Mi").
                AddTrait(traits.Autoscaler().Min(2).Max(10).CPUUtilization(70)).
                AddTrait(traits.Ingress().Domain("app.example.com").Path("/")),
        ).

        // Add backend component
        AddComponent(
            components.WebService("backend").
                Image("myapp/backend:v1.0").
                Port(8080).
                Env("DATABASE_URL", "postgres://...").
                AddTrait(traits.ServiceBinding().Secret("db-credentials")),
        ).

        // Add workflow
        SetWorkflow(
            workflow.New().
                Step(workflow.DeployStep("deploy-backend").
                    Components("backend")).
                Step(workflow.ApprovalStep("approve-frontend").
                    Users("platform-team")).
                Step(workflow.DeployStep("deploy-frontend").
                    Components("frontend").
                    DependsOn("approve-frontend")),
        ).

        // Add policies
        AddPolicy(policies.Topology("multi-cluster").
            Clusters("cluster-east", "cluster-west").
            Namespace("production"))

    // Apply to cluster
    client, err := vela.NewClient()
    if err != nil {
        panic(err)
    }

    if err := client.Apply(context.Background(), app); err != nil {
        panic(err)
    }

    fmt.Println("Application deployed successfully!")

    // Export as YAML
    yaml, _ := app.ToYAML()
    fmt.Println(string(yaml))
}
```

### Example 2: TypeScript SDK Usage

```typescript
import {
    Application,
    WebService,
    Autoscaler,
    Ingress,
    Topology,
    VelaClient
} from '@kubevela/sdk';

async function main() {
    // Create application with fluent API
    const app = Application.create('my-app')
        .namespace('production')
        .addComponent(
            WebService.create('frontend')
                .image('nginx:1.21')
                .port(80)
                .replicas(3)
                .addTrait(
                    Autoscaler.create()
                        .min(2)
                        .max(10)
                        .cpuUtilization(70)
                )
                .addTrait(
                    Ingress.create()
                        .domain('app.example.com')
                        .path('/')
                )
        )
        .addPolicy(
            Topology.create('multi-cluster')
                .clusters('cluster-east', 'cluster-west')
        );

    // Apply to cluster
    const client = await VelaClient.create();
    await client.apply(app);

    console.log('Application deployed!');

    // Export as YAML
    const yaml = app.toYAML();
    console.log(yaml);
}

main().catch(console.error);
```

### Example 3: Full Circle Workflow

```
┌─────────────────────────────────────────────────────────────────────┐
│                     Platform Engineer Workflow                       │
│                                                                      │
│  1. Author Definition (Go/TS/Python)                                │
│     └─▶ vela def develop ./definitions/                             │
│                                                                      │
│  2. Apply to Cluster (CUE compilation is automatic)                 │
│     └─▶ vela def apply ./definitions/                               │
│                                                                      │
│  3. Generate SDK for End Users                                      │
│     └─▶ vela sdk generate --from cluster -o ./sdk                   │
│                                                                      │
│  4. Publish SDK                                                     │
│     └─▶ npm publish / go mod publish / pip publish                  │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                        End User Workflow                             │
│                                                                      │
│  1. Install SDK                                                     │
│     └─▶ go get / npm install / pip install                          │
│                                                                      │
│  2. Build Application with Type Safety                              │
│     └─▶ Use generated fluent builders                               │
│                                                                      │
│  3. Deploy                                                          │
│     └─▶ vela up / GitOps                                            │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Part 6: Addressing Known Issues from Original SDK Design

The [original SDK generating design](./sdk_generating.md) identified several issues. This section explains how the new framework addresses each:

### Issue 1: Default Values Not Set in Generated Code

**Original Problem:** The `default` field in OpenAPI schema was not used in generated code.

**Solution:** The defkit API has built-in default value support:

```go
Parameter("replicas", defkit.Int().Default(3))
Parameter("port", defkit.Int().Default(8080))
```

The generated SDK includes defaults in constructors:

```go
func NewWebservice(name string) *Webservice {
    return &Webservice{
        Name: name,
        Spec: WebserviceSpec{
            Replicas: 3,    // Default applied
            Port:     8080, // Default applied
        },
    }
}
```

### Issue 2: Parameter Validation Using CUE Constraints

**Original Problem:** CUE constraints like `parameter: { a: >=0 }` weren't enforced in SDK.

**Solution:** The defkit API includes validation constraints:

```go
Parameter("replicas", defkit.Int().Min(1).Max(100))
Parameter("cpu", defkit.String().Pattern(`^\d+m?$`))
Parameter("name", defkit.String().MinLength(1).MaxLength(63))
```

Generated SDK includes runtime validation:

```go
func (w *WebserviceSpec) Validate() error {
    if w.Replicas < 1 || w.Replicas > 100 {
        return fmt.Errorf("replicas must be between 1 and 100")
    }
    return nil
}
```

### Issue 3: Trait-Component Compatibility Validation

**Original Problem:** No validation that a trait is suited for a component (e.g., `K8s-object` only accepts `labels` and `annotation` traits).

**Solution:** The defkit trait API includes `AppliesTo()` and `ConflictsWith()`:

```go
trait.New("scaler").
    AppliesTo("webservice", "worker", "microservice").
    ConflictsWith("manual-scaler")

trait.New("gpu").
    AppliesTo("worker", "task")  // Only GPU-capable workloads
```

Generated SDK enforces compatibility:

```go
func (app *Application) Validate() error {
    for _, comp := range app.Components {
        for _, t := range comp.Traits {
            if !t.AppliesTo(comp.Type) {
                return fmt.Errorf("trait %s cannot be applied to %s", t.Type, comp.Type)
            }
        }
    }
    return nil
}
```

### Issue 4: CUE Union Types (`*#A|#B` Pattern)

**Original Problem:** CUE union types like `parameter: *#A|#B` cause issues (cue-lang/cue#2259).

**Solution:** The defkit API provides `OneOf()` for union types:

```go
Parameter("source", defkit.OneOf(
    defkit.Struct(defkit.Field("git", gitSourceType)),
    defkit.Struct(defkit.Field("helm", helmSourceType)),
))
```

Generated SDK uses discriminated unions:

```go
type Source struct {
    Git  *GitSource  `json:"git,omitempty"`
    Helm *HelmSource `json:"helm,omitempty"`
}

// Fluent builders
func SourceFromGit(url string) Source { return Source{Git: &GitSource{URL: url}} }
func SourceFromHelm(repo string) Source { return Source{Helm: &HelmSource{Repo: repo}} }
```

The IR handles union types explicitly, avoiding the CUE language limitation.

### Issue 5: Top-Level Map Schema

**Original Problem:** openapi-generator doesn't support top-level schema to be a map (affects `label` and `annotation` traits).

**Solution:** Since we no longer use openapi-generator, this limitation is removed:

```go
// Labels trait - top-level map works
trait.New("labels").
    Parameter("labels", defkit.Map(defkit.String()))

// Generated SDK
type LabelsTrait struct {
    Labels map[string]string `json:"labels"`
}

// Usage
traits.Labels(map[string]string{"app": "myapp", "env": "prod"})
```

### Issue 6: Docker Dependency for Generation

**Original Problem:** `vela def gen-api` requires Docker to run openapi-generator.

**Solution:** The new SDK generator is pure Go with no external dependencies:

```bash
# No Docker required
vela sdk generate --from cluster --language go --output ./sdk
```

---

## Part 7: CLI Interface

### Enhanced `vela sdk` Commands

```bash
# Generate SDK from various sources
vela sdk generate --from cluster --language go --output ./sdk
vela sdk generate --from release:v1.9.0 --language typescript --output ./sdk
vela sdk generate --from oci://registry/defs:v1.0 --language python --output ./sdk

# Development mode with watch
vela sdk develop --from cluster --language go --output ./sdk --watch

# Check SDK compatibility
vela sdk check ./sdk

# Sync SDK with cluster
vela sdk sync ./sdk --from cluster
```

### Enhanced `vela def` Commands

```bash
# Apply definitions (transparent compilation)
vela def apply ./definitions/
vela def apply ./myplatform/microservice.go --dry-run

# Development mode with watch
vela def develop ./definitions/

# Export/Import between formats
vela def export ./microservice.go --format cue
vela def import ./legacy.cue --language go --output ./converted/

# Compile without applying
vela def compile ./microservice.go --output ./generated.cue
```

---

## Implementation Phases

### Phase 1: Foundation

**Goals:**
- Implement Intermediate Representation (IR) specification
- Create Go Definition Kit (defkit) with core APIs
- Implement transparent CUE compilation in CLI
- Add `vela def apply` with automatic compilation

**Deliverables:**
- `pkg/defkit/` - Definition Kit core library
- `pkg/defkit/component/` - Component definition API
- `pkg/defkit/trait/` - Trait definition API
- `pkg/ir/` - IR types and serialization
- Enhanced `vela def apply` command

### Phase 2: SDK Generation

**Goals:**
- Implement live cluster introspection
- Create SDK generator framework
- Add Go SDK code generation with fluent builders
- Implement version manifest system

**Deliverables:**
- `pkg/sdk/introspect/` - Cluster introspection
- `pkg/sdk/generate/` - Generator framework
- `pkg/sdk/generate/golang/` - Go generator
- `vela sdk generate` command
- Version manifest specification

### Phase 3: Multi-Language Support

**Goals:**
- Add TypeScript Definition Kit and SDK generator
- Add Python Definition Kit and SDK generator
- Implement registry-based definition sources (OCI, Git)
- Add watch mode for development

**Deliverables:**
- `pkg/sdk/generate/typescript/` - TypeScript generator
- `pkg/sdk/generate/python/` - Python generator
- `pkg/registry/` - Registry abstraction
- Watch mode implementation
- Language-specific npm/pip packages

### Phase 4: Advanced Features

**Goals:**
- Controller-based compilation for GitOps
- IDE integrations (VS Code, GoLand)
- Definition testing framework
- Definition versioning and migrations

**Deliverables:**
- DefinitionSource CRD and controller
- IDE extension APIs
- `pkg/defkit/testing/` - Testing utilities
- Migration tooling

### Phase 5: Ecosystem

**Goals:**
- Community definition marketplace
- CI/CD integration templates
- Documentation and tutorials
- Reference platform implementations

**Deliverables:**
- Definition registry service
- GitHub Actions / GitLab CI templates
- Comprehensive documentation
- Example platform projects

---

## Success Metrics

| Metric | How to Measure |
|--------|----------------|
| SDK adoption | Package downloads (npm/PyPI/Go modules), active users in community channels |
| Developer experience | User feedback, time-to-deploy surveys, documentation engagement |
| Migration success | Users successfully migrating from CUE-only workflows |
| Community contributions | Pull requests, issues filed, community-authored definitions |

*Note: Specific baseline metrics should be established before implementation begins.*

---

## Compatibility Matrix

| KubeVela Version | SDK Framework Status | Go defkit | TS defkit | Java defkit | Python defkit |
|------------------|---------------------|-----------|-----------|-------------|---------------|
| 1.x              | Not supported       | -         | -         | -           | -             |
| 2.0.0-alpha      | Alpha               | Alpha     | Alpha     | Alpha       | Alpha         |
| 2.0.0            | Stable              | v1.0      | v1.0      | v1.0        | v1.0          |
| 2.1.x            | Stable              | v1.1      | v1.1      | v1.1        | v1.1          |

**Language Requirements:**
- **Go:** Go 1.21+
- **TypeScript:** Node.js 20 LTS+, TypeScript 5.0+
- **Java:** Java 21 LTS (OpenJDK)
- **Python:** Python 3.11+

The SDK framework is introduced in KubeVela 2.0.0-alpha and is not backported to 1.x versions.

---

## Appendix A: Generated File Structure

```
sdk-go/
├── go.mod
├── go.sum
├── vela.go                    # Main package entry
├── application.go             # Application builder
├── client.go                  # Kubernetes client wrapper
├── components/
│   ├── webservice.go          # WebService builder
│   ├── worker.go              # Worker builder
│   ├── task.go                # Task builder
│   ├── helm.go                # Helm builder
│   └── ...
├── traits/
│   ├── autoscaler.go
│   ├── ingress.go
│   ├── sidecar.go
│   └── ...
├── policies/
│   ├── topology.go
│   ├── override.go
│   └── ...
├── workflow/
│   ├── workflow.go
│   ├── deploy.go
│   ├── approval.go
│   └── ...
├── custom/                    # Custom definitions (if generated)
│   └── myplatform/
│       └── backend_service.go
├── internal/
│   ├── types/                 # Generated type definitions
│   └── util/                  # Internal utilities
└── manifest.json              # SDK version manifest
```

## Appendix B: defkit API Reference

### Core Types

```go
package defkit

// Parameter types
func String() *StringParam
func Int() *IntParam
func Bool() *BoolParam
func Float() *FloatParam
func Array(elem ParamType) *ArrayParam
func Map(value ParamType) *MapParam
func Struct(fields ...Field) *StructParam
func Enum(values ...string) *EnumParam
func OneOf(types ...ParamType) *OneOfParam

// Validation
type StringParam interface {
    Required() StringParam
    Optional() StringParam
    Default(v string) StringParam
    MinLength(n int) StringParam
    MaxLength(n int) StringParam
    Pattern(regex string) StringParam
    Description(d string) StringParam
}

type IntParam interface {
    Required() IntParam
    Optional() IntParam
    Default(v int) IntParam
    Min(n int) IntParam
    Max(n int) IntParam
    Description(d string) IntParam
}
```

### Component Definition API

```go
package component

type Definition struct{}

func New(name string) *Definition
func (d *Definition) Description(desc string) *Definition
func (d *Definition) Labels(l map[string]string) *Definition
func (d *Definition) Annotations(a map[string]string) *Definition
func (d *Definition) Workload(w WorkloadType) *Definition
func (d *Definition) Parameter(name string, param ParamType) *Definition
func (d *Definition) Template(fn TemplateFunc) *Definition
```

### Trait Definition API

```go
package trait

type Definition struct{}

func New(name string) *Definition
func (d *Definition) Description(desc string) *Definition
func (d *Definition) AppliesTo(components ...string) *Definition
func (d *Definition) ConflictsWith(traits ...string) *Definition
func (d *Definition) Parameter(name string, param ParamType) *Definition
func (d *Definition) Patch(fn PatchFunc) *Definition
```

---

## Design Considerations & Known Challenges

This section documents architectural decisions and known challenges identified during design review.

### Critical Issues

#### C1: DefinitionSource Namespace Hierarchy

**Challenge:** The proposed `DefinitionSource` CRD must integrate with KubeVela's existing namespace-based definition resolution hierarchy:

1. System definitions in `vela-system` namespace
2. Namespace-scoped definitions
3. Application-level inline definitions

**Design Decision:** DefinitionSource follows the same hierarchy:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: DefinitionSource
metadata:
  name: platform-definitions
  namespace: vela-system  # System-wide definitions
spec:
  # ...
---
apiVersion: core.oam.dev/v1beta1
kind: DefinitionSource
metadata:
  name: team-definitions
  namespace: team-a  # Namespace-scoped definitions
spec:
  # ...
```

Resolution order remains: Application inline → Namespace → vela-system.

#### C2: Semantic Versioning Prerequisite

**Challenge:** SDK generation for specific definition versions requires the definition versioning feature from `design/vela-core/definition-versioning.md`.

**Design Decision:**
- Phase 1-2: SDK generates from `metadata.annotations["version"]` or falls back to cluster state checksum
- Phase 3+: Full integration with `spec.version` when definition versioning is implemented
- SDK manifest tracks source checksums for reproducibility regardless of version availability

#### C3: CUE Compilation Path Unification

**Challenge:** Three existing CUE compilation paths must all work with the new IR:

1. `pkg/definition/gen_sdk/` - Current SDK generation
2. `pkg/cue/definition/template.go` - AbstractEngine for runtime workload/trait rendering
3. `pkg/controller/` - Controller-based definition processing

**Design Decision:**
- IR is an **addition**, not a replacement for existing CUE paths
- Native defkit code → IR → CUE → existing paths
- No changes to AbstractEngine or controller CUE processing
- Pure CUE definitions continue to work unchanged

```
┌─────────────────────────────────────────────────────────────┐
│                     CUE Path Unification                     │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Native defkit ──▶ IR ──▶ CUE ─┐                            │
│                                │                            │
│  Raw CUE ──────────────────────┼──▶ Existing Paths         │
│                                │    (AbstractEngine,       │
│  YAML Definition ──────────────┘    Controllers, etc.)     │
│                                                              │
│  (All paths converge to CUE before entering existing code)  │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Moderate Issues

#### M1: CLI Naming Consistency

**Challenge:** New commands (`vela sdk generate`) coexist with legacy (`vela def gen-api`).

**Design Decision:**
- Legacy `vela def gen-api` remains unchanged for backward compatibility
- New `vela sdk generate` is the recommended path for 2.0+
- Documentation clearly distinguishes both
- In 2.1.0+, evaluate adding deprecation notice to `vela def gen-api`

#### M2: File Type Detection for `vela def apply`

**Challenge:** How does `vela def apply` distinguish between CUE files and native language files?

**Design Decision:** Detection by file extension and content:

```
File Type Detection Order:
1. Extension-based:
   .cue         → CUE processing (existing path)
   .go          → Go defkit compilation
   .ts/.mts     → TypeScript defkit compilation
   .py          → Python defkit compilation
   .java        → Java defkit compilation
   .yaml/.yml   → YAML definition (existing path)

2. For directories, scan and process each file by extension

3. Ambiguous cases (e.g., polyglot directories):
   - Process each recognized file type
   - Warn on unrecognized extensions
   - --language flag can force specific processing
```

Example:

```bash
# Auto-detect file type
vela def apply ./microservice.go       # Detected as Go
vela def apply ./definitions/          # Processes all recognized types

# Force specific processing
vela def apply ./ambiguous --language go
```

#### M3: Error Catalog

**Challenge:** Consistent error messages across SDK generation and defkit compilation.

**Design Decision:** Define error code prefixes for each subsystem:

| Prefix | Subsystem | Example |
|--------|-----------|---------|
| `DEFKIT-` | Definition Kit APIs | `DEFKIT-001: Invalid parameter type` |
| `SDKGEN-` | SDK generation | `SDKGEN-001: Cannot connect to cluster` |
| `COMPILE-` | CUE compilation | `COMPILE-001: Template validation failed` |
| `IR-` | Intermediate Representation | `IR-001: Invalid schema structure` |

Implementation: Create `pkg/errors/catalog.go` with documented error codes.

#### M4: ConfigMap Naming Collision Risk

**Challenge:** OpenAPI schemas stored in ConfigMaps could collide if multiple definition versions exist.

**Design Decision:** Include version in ConfigMap naming:

```
Current:  {definition-name}-schema
Proposed: {definition-name}-{version}-schema

Examples:
- webservice-v1.9.0-schema
- webservice-v2.0.0-schema
- microservice-custom-v1.0.0-schema
```

Fallback for unversioned definitions: Use checksum suffix (e.g., `webservice-abc123-schema`).

#### M5: Rollback Strategy

**Challenge:** No defined rollback mechanism if SDK generation or defkit compilation fails mid-process.

**Design Decision:** Atomic operations with rollback:

1. **SDK Generation:**
   - Generate to temporary directory first
   - Validate generated code compiles
   - Atomically move to target directory
   - On failure, temp directory is cleaned up

2. **Definition Application:**
   - Compile and validate before applying
   - Use server-side apply with `--dry-run=server` validation
   - On failure, no changes are made to cluster

3. **Batch Operations:**
   - Track successfully applied definitions
   - On partial failure, report applied vs failed
   - User can rerun for failed definitions

---

## References

- [SDK Generating (Original)](./sdk_generating.md)
- [KubeVela X-Definition System](https://kubevela.io/docs/platform-engineers/x-definition/)
- [OAM Specification](https://oam.dev/)
- [Definition Versioning](../vela-core/definition-versioning.md)
