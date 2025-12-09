# KEP: Definition Kit (defkit) - Go SDK for X-Definition Authoring

## Summary

Definition Kit (defkit) is a Go SDK that enables platform engineers to author KubeVela X-Definitions using native Go code instead of CUE. The SDK compiles Go to CUE transparently, providing full IDE support while maintaining compatibility with the KubeVela controller.

## Motivation

### Goals

1. **Author X-Definitions in Go** - Write ComponentDefinition, TraitDefinition, PolicyDefinition, and WorkflowStepDefinition using fluent Go APIs
2. **Transparent CUE compilation** - Go code compiles to CUE automatically; developers never see or write CUE
3. **Full IDE support** - Autocomplete, type checking, and inline documentation
4. **Testable definitions** - Unit test definitions using standard Go testing frameworks
5. **Schema-agnostic resource construction** - Support any Kubernetes resource without coupling to K8s versions
6. **Easy distribution via Go modules** - Share and version X-Definitions as standard Go packages, enabling `go get` for platform capabilities

### Non-Goals

1. Multi-language support in initial release (Go only)
2. Replacing CUE as the controller's internal engine
3. Runtime CUE evaluation in the SDK

## Proposal

### Why Go for X-Definitions?

**1. Familiar Tooling** - Platform engineers already use Go for controllers, operators, and CLI tools. Writing definitions in the same language eliminates context switching and leverages existing skills.

**2. IDE Experience** - Full autocomplete, type checking, go-to-definition, and inline documentation. No special CUE plugins required.

**3. Standard Distribution** - Definitions become Go packages that can be versioned, shared via `go get`, and composed like any other library. No custom registries or tooling needed.

**4. Testability** - Use standard Go testing frameworks (go test, testify, gomega) to unit test definitions before deployment. Mock contexts enable testing without a cluster.

**5. Compile-Time Safety** - Catch errors at compile time rather than at deployment time. Invalid field names, type mismatches, and missing required parameters are caught immediately.

### Fluent API Design

defkit provides a fluent builder API where parameters are defined inline and bound to local variables:

```go
func WebserviceDefinition() *component.Definition {
    // Parameters defined as local variables within the function
    image := defkit.String("image").Required()
    replicas := defkit.Int("replicas").Default(3).Min(1).Max(100)
    cpu := defkit.String("cpu").Optional()

    return component.New("webservice").
        Description("A production-ready web service").
        Parameters(image, replicas, cpu).
        Template(func(tpl *component.TemplateContext) *component.Output {
            deploy := defkit.NewResource("apps/v1", "Deployment").
                SetName(tpl.Name()).
                Set("spec.replicas", replicas).
                Set("spec.template.spec.containers", []map[string]any{{
                    "name":  tpl.Name(),
                    "image": image,
                }}).
                // Optional parameters use fluent conditional
                SetIf(cpu.IsSet(), "spec.template.spec.containers[0].resources.limits.cpu", cpu)

            return tpl.Output(deploy)
        })
}
```

**Key design insight**: Parameters are Go variables that can be used directly in both definition and template. No string lookups, no dual references. The variable `image` carries both its schema definition AND serves as the accessor.

This reads naturally: "Define image, replicas, and cpu as parameters. Create a webservice component using them."

**Note:** Parameters are defined as local variables within the function (not package-level) to ensure proper encapsulation and test isolation. The template function receives a `TemplateContext` (named `tpl` to avoid confusion with Go's `context` package) for runtime context access.

### Common Patterns

The SDK provides typed helpers for common definition patterns:

| Pattern | Go API | Example |
|---------|--------|---------|
| Required parameter | `defkit.String("image").Required()` | `image` must be provided |
| Default value | `defkit.Int("replicas").Default(3)` | `replicas` defaults to 3 |
| Validation | `defkit.Int("replicas").Min(1).Max(100)` | `replicas` between 1-100 |
| Enums | `defkit.Enum("policy", "Always", "Never")` | `policy` with allowed values |
| Optional struct | `defkit.Struct("persistence", ...).Optional()` | `persistence` block |
| Arrays | `defkit.Array("args", defkit.String())` | `args` list |

### Runtime Context Access

CUE templates access runtime values (like deployment status) through well-known paths. The Go SDK provides fluent methods that generate these paths:

```go
tpl.Output()                                  // → context.output
tpl.Output().Status().Field("readyReplicas")  // → context.output.status.readyReplicas
tpl.Outputs("ingress")                        // → context.outputs.ingress
tpl.ClusterVersion().Minor()                  // → context.clusterVersion.minor
tpl.Name()                                    // → context.name
replicas                                      // → parameter.replicas (from var)
```

This means health policies and custom status can be written in Go:

```go
// Health check: are all replicas ready?
component.HealthPolicy(func(tpl *component.TemplateContext) defkit.Condition {
    return defkit.Eq(
        tpl.Output().Spec().Field("replicas"),
        tpl.Output().Status().Field("readyReplicas").Default(0),
    )
})
```

### When to Use What

```
┌─────────────────────────────────────────────────────────────────┐
│                     DECISION FRAMEWORK                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   Can I express it with defkit Go API?                          │
│                                                                 │
│       YES ──────────► Use Go (95% of cases)                     │
│                       • Static values: Set("image", "nginx")    │
│                       • Parameters: use the variable directly   │
│                       • Runtime status: tpl.Output().Status()   │
│                       • Cluster info: tpl.ClusterVersion()      │
│                                                                 │
│       NO ───────────► Use RawCUE() (rare escape hatch)          │
│                       • True unification semantics (a & b)      │
│                       • Complex nested comprehensions (3+ levels)│
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Target Developer Experience

```go
package myplatform

import (
    "github.com/oam-dev/kubevela/pkg/defkit"
    "github.com/oam-dev/kubevela/pkg/defkit/component"
)

func WebserviceComponent() *component.Definition {
    // Parameters scoped to definition function
    image := defkit.String("image").Required()
    replicas := defkit.Int("replicas").Default(3).Min(1).Max(100)
    cpu := defkit.String("cpu").Optional()

    return component.New("webservice").
        Description("A production-ready web service").
        Parameters(image, replicas, cpu).
        Template(func(tpl *component.TemplateContext) *component.Output {
            deploy := defkit.NewResource("apps/v1", "Deployment").
                SetName(tpl.Name()).
                Set("spec.replicas", replicas).
                Set("spec.template.spec.containers", []map[string]any{{
                    "name":  tpl.Name(),
                    "image": image,
                }}).
                SetIf(cpu.IsSet(), "spec.template.spec.containers[0].resources.limits.cpu", cpu)

            return tpl.Output(deploy)
        }).
        // Health policy using runtime context
        HealthPolicy(func(tpl *component.TemplateContext) defkit.Condition {
            return defkit.Eq(
                tpl.Output().Spec().Field("replicas"),
                tpl.Output().Status().Field("readyReplicas").Default(0),
            )
        }).
        // Custom status message
        CustomStatus(func(tpl *component.TemplateContext) defkit.Status {
            return defkit.Message("Ready: %v/%v",
                tpl.Output().Status().Field("readyReplicas"),
                tpl.Output().Spec().Field("replicas"),
            )
        })
}
```

---

## TemplateContext API Reference

### Runtime Context Accessors

| Go Method | Generated CUE | Description |
|-----------|---------------|-------------|
| `tpl.Output()` | `context.output` | Primary rendered resource |
| `tpl.Output().Spec()` | `context.output.spec` | Resource spec |
| `tpl.Output().Status()` | `context.output.status` | Resource status |
| `tpl.Output().Metadata()` | `context.output.metadata` | Resource metadata |
| `tpl.Outputs("name")` | `context.outputs.name` | Named auxiliary output |
| `tpl.ClusterVersion()` | `context.clusterVersion` | Target cluster version |
| `tpl.ClusterVersion().Minor()` | `context.clusterVersion.minor` | K8s minor version |
| `tpl.AppName()` | `context.appName` | Application name |
| `tpl.Name()` | `context.name` | Component name |
| `tpl.Namespace()` | `context.namespace` | Target namespace |

### Parameter Variables

Parameters are typed variables that generate CUE paths automatically:

| Go Usage | Generated CUE | Description |
|----------|---------------|-------------|
| `image` (in template) | `parameter.image` | Parameter value (from variable name) |
| `cpu.IsSet()` | `parameter.cpu != _\|_` | Check if optional parameter is set |
| `persistence.Field("size")` | `parameter.persistence.size` | Nested field access |

**How it works**: Each parameter variable knows its name and type. When used in a template expression, it compiles to the corresponding `parameter.X` CUE path.

### Comparison and Conditional Helpers

| Go Method | Generated CUE |
|-----------|---------------|
| `defkit.Eq(a, b)` | `a == b` |
| `defkit.Ne(a, b)` | `a != b` |
| `defkit.Lt(a, b)` | `a < b` |
| `defkit.Gte(a, b)` | `a >= b` |
| `defkit.And(a, b)` | `a && b` |
| `defkit.Or(a, b)` | `a \|\| b` |
| `defkit.If(cond)` | `if cond {...}` |

---

## Examples

### Health Policy

```go
// Go - inside definition function
HealthPolicy(func(tpl *component.TemplateContext) defkit.Condition {
    return defkit.Eq(
        tpl.Output().Spec().Field("replicas"),
        tpl.Output().Status().Field("readyReplicas"),
    )
})

// Generated CUE
// isHealth: context.output.spec.replicas == context.output.status.readyReplicas
```

### Cluster Version Check

```go
// Go - inside template function
Template(func(tpl *component.TemplateContext) *component.Output {
    deploy := defkit.NewResource("batch/v1", "CronJob").
        SetName(tpl.Name()).
        SetIf(tpl.ClusterVersion().Minor().Lt(25), "apiVersion", "batch/v1beta1").
        SetIf(tpl.ClusterVersion().Minor().Gte(25), "apiVersion", "batch/v1")

    return tpl.Output(deploy)
})

// Generated CUE
// if context.clusterVersion.minor < 25 {
//     apiVersion: "batch/v1beta1"
// }
// if context.clusterVersion.minor >= 25 {
//     apiVersion: "batch/v1"
// }
```

### Cross-Resource Reference

```go
// Go - inside CustomStatus function
CustomStatus(func(tpl *component.TemplateContext) defkit.Status {
    igs := tpl.Outputs("ingress").Status().Field("loadBalancer.ingress")
    return defkit.If(igs.IsSet()).
        Message("Visit %v to access", igs.Index(0).Field("ip"))
})

// Generated CUE
// let igs = context.outputs.ingress.status.loadBalancer.ingress
// if igs != _|_ {
//     message: "Visit " + igs[0].ip + " to access"
// }
```

### Health Check with Defaults

```go
// Go - inside HealthPolicy function
HealthPolicy(func(tpl *component.TemplateContext) defkit.Condition {
    return defkit.Eq(
        tpl.Output().Spec().Field("replicas"),
        tpl.Output().Status().Field("readyReplicas").Default(0),
    )
})

// Generated CUE (handles the unification pattern automatically)
// ready: { readyReplicas: *0 | int } & {
//     if context.output.status.readyReplicas != _|_ {
//         readyReplicas: context.output.status.readyReplicas
//     }
// }
// isHealth: context.output.spec.replicas == ready.readyReplicas
```

### Compound Conditions

Use `defkit.And()` and `defkit.Or()` to compose conditions:

```go
// Single condition
deploy.SetIf(cpu.IsSet(), "spec.resources.limits.cpu", cpu)

// AND - both must be true
deploy.SetIf(
    defkit.And(
        tpl.ClusterVersion().Minor().Gte(25),
        enableNewAPI.IsSet(),
    ),
    "apiVersion", "batch/v1",
)

// OR - either condition
deploy.SetIf(
    defkit.Or(
        tpl.ClusterVersion().Minor().Lt(21),
        legacyMode.IsSet(),
    ),
    "apiVersion", "batch/v1beta1",
)

// Complex: (A && B) || C
deploy.SetIf(
    defkit.Or(
        defkit.And(isProduction, highAvailability),
        forceHA.IsSet(),
    ),
    "spec.replicas", 3,
)
```

**Generated CUE:**
```cue
if parameter.cpu != _|_ {
    spec: resources: limits: cpu: parameter.cpu
}
if context.clusterVersion.minor >= 25 && parameter.enableNewAPI != _|_ {
    apiVersion: "batch/v1"
}
if context.clusterVersion.minor < 21 || parameter.legacyMode != _|_ {
    apiVersion: "batch/v1beta1"
}
if (parameter.isProduction && parameter.highAvailability) || parameter.forceHA != _|_ {
    spec: replicas: 3
}
```

**Multiple fields under the same condition:**

```go
// Use If().EndIf() block for multiple fields
deploy.
    If(defkit.And(persistence.IsSet(), persistence.Field("enabled"))).
        Set("spec.volumeClaimTemplates", []map[string]any{{...}}).
        Set("spec.template.spec.volumes", []map[string]any{{...}}).
    EndIf()
```

---

## RawCUE() Escape Hatch

For the ~5% of patterns that cannot be expressed in Go, use `RawCUE()`:

### True Unification Semantics

```go
// CUE's & operator has mathematical semantics Go cannot replicate
defkit.RawCUE(`
    baseConfig: {
        replicas: >=1
        image: string
    }
    prodConfig: baseConfig & {
        replicas: >=3 & <=10
    }
`)
```

### Complex Nested Comprehensions

```go
// Triple-nested comprehension with complex conditions
defkit.RawCUE(`
    result: [
        for ns in namespaces
        for svc in ns.services if svc.exposed
        for port in svc.ports if port.protocol == "TCP" {
            name: "\(ns.name)-\(svc.name)-\(port.port)"
        }
    ]
`)
```

---

## Parameter Types

| defkit Type | CUE Equivalent | Go Type |
|-------------|----------------|---------|
| `defkit.String("name")` | `name: string` | `string` |
| `defkit.Int("name")` | `name: int` | `int` |
| `defkit.Bool("name")` | `name: bool` | `bool` |
| `defkit.Float("name")` | `name: number` | `float64` |
| `defkit.Array("name", T)` | `name: [...T]` | `[]T` |
| `defkit.Map("name", V)` | `name: {[string]: V}` | `map[string]V` |
| `defkit.Struct("name", ...)` | `name: {field: type}` | struct |
| `defkit.Enum("name", "a", "b")` | `name: "a" \| "b"` | string with validation |
| `defkit.OneOf("name", ...)` | `name: close({...}) \| close({...})` | discriminated union |

### Parameter Modifiers

```go
defkit.String("image").
    Required().                    // Must be provided
    Default("nginx:latest").       // CUE: *"nginx:latest" | string
    Optional().                    // Can be omitted
    Description("Container image")  // Schema description

defkit.Int("replicas").
    Min(1).Max(100).              // Numeric constraints
    Default(3)

defkit.String("name").
    Pattern("^[a-z][a-z0-9-]*$")  // Regex validation
```

### Complex Parameter Types

**Struct parameters** for nested configuration:

```go
persistence := defkit.Struct("persistence",
    defkit.Bool("enabled").Default(false),
    defkit.String("storageClass").Required(),
    defkit.String("size").Default("10Gi"),
).Optional()

// Usage in template - fluent conditional for optional struct
deploy.
    If(defkit.And(persistence.IsSet(), persistence.Field("enabled"))).
    Set("spec.volumeClaimTemplates", []map[string]any{{
        "spec": map[string]any{
            "storageClassName": persistence.Field("storageClass"),
            "resources": map[string]any{
                "requests": map[string]any{
                    "storage": persistence.Field("size"),
                },
            },
        },
    }})
```

**Discriminated unions** for variant types:

```go
volume := defkit.OneOf("volume",
    defkit.Variant("emptyDir",
        defkit.String("medium").Default(""),
    ),
    defkit.Variant("pvc",
        defkit.String("claimName").Required(),
    ),
    defkit.Variant("configMap",
        defkit.String("name").Required(),
        defkit.Int("defaultMode").Default(420),
    ),
)

// Usage: volume.Discriminator() returns "emptyDir", "pvc", or "configMap"
```

---

## Definition Types

### ComponentDefinition

```go
func MyComponent() *component.Definition {
    image := defkit.String("image").Required()

    return component.New("webservice").
        Description("...").
        Parameters(image).
        Template(func(tpl *component.TemplateContext) *component.Output { ... }).
        HealthPolicy(func(tpl *component.TemplateContext) defkit.Condition { ... }).
        CustomStatus(func(tpl *component.TemplateContext) defkit.Status { ... })
}
```

### TraitDefinition

```go
func RateLimitTrait() *trait.Definition {
    rps := defkit.Int("rps").Required()

    return trait.New("rate-limit").
        Description("...").
        AppliesTo("webservice", "microservice").
        Parameters(rps).
        Patch(func(tpl *trait.TemplateContext) *trait.Patches {
            return tpl.PatchWorkload().
                AddAnnotation("ratelimit.example.com/rps", rps)
        })
}
```

### PolicyDefinition

```go
func TopologyPolicy() *policy.Definition {
    clusters := defkit.Array("clusters", defkit.String()).Required()

    return policy.New("topology").
        Description("...").
        Parameters(clusters).
        Template(func(tpl *policy.TemplateContext) *policy.Output { ... })
}
```

### WorkflowStepDefinition

```go
func DeployStep() *workflow.StepDefinition {
    componentName := defkit.String("component").Required()

    return workflow.NewStep("deploy").
        Description("...").
        Parameters(componentName).
        Execute(func(tpl *workflow.TemplateContext) *workflow.Result {
            comp := tpl.Op(op.Load{Component: componentName})
            tpl.Op(op.ApplyComponent{Component: comp})
            return tpl.Success()
        })
}
```

---

## Schema-Agnostic Resource Construction

defkit does NOT ship typed Kubernetes helpers. Instead, it provides a universal builder that works with any resource:

```go
// Core Kubernetes
defkit.NewResource("apps/v1", "Deployment").
    SetName("my-app").
    Set("spec.replicas", 3)

// CRDs (Crossplane, KRO, etc.)
defkit.NewResource("database.aws.crossplane.io/v1beta1", "DBInstance").
    SetName("my-db").
    Set("spec.forProvider.engine", "postgres")
```

**Optional typed adapter** for users who want compile-time type safety:

```go
import appsv1 "k8s.io/api/apps/v1"

deployment := &appsv1.Deployment{...}
return defkit.FromTyped(deployment)
```

---

## CLI Commands

The following commands extend the existing `vela def` command group to support Go definitions:

```bash
# Apply Go definitions (extends existing `vela def apply`)
# CUE compilation is transparent - .go files are automatically detected
vela def apply ./definitions/webservice.go

# Apply all definitions in directory (supports mixed .cue and .go)
vela def apply ./definitions/

# Dry-run to see generated resources (existing flag)
vela def apply ./definitions/ --dry-run

# Render Go definition to CUE (extends existing `vela def render`)
vela def render ./definitions/webservice.go --output cue

# Validate without applying (existing command, extended for .go)
vela def vet ./definitions/webservice.go

# Initialize new Go definition from template (extends existing `vela def init`)
vela def init webservice --type component --lang go
```

### New Subcommands

| Command | Description |
|---------|-------------|
| `vela def gen-go` | Generate Go defkit code from existing CUE definitions (migration) |

### Extended Existing Commands

| Command | Extension |
|---------|-----------|
| `vela def apply` | Accepts `.go` files, compiles to CUE transparently |
| `vela def render` | Accepts `.go` files, `--output cue` to see generated CUE |
| `vela def vet` | Validates `.go` definitions including Go compilation |
| `vela def init` | `--lang go` flag to scaffold Go definition |
| `vela def gen-api` | Already generates Go SDK from CUE (inverse direction) |

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Definition Authoring Pipeline                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐       │
│  │   defkit     │    │      IR      │    │   Compiler   │       │
│  │   Go API     │───▶│   (JSON)     │───▶│   Go → CUE   │──▶ CR │
│  │              │    │              │    │              │       │
│  │ tpl.Output() │    │ • Schema     │    │ • Validation │       │
│  │ param vars   │    │ • Template   │    │ • CUE Gen    │       │
│  └──────────────┘    └──────────────┘    └──────────────┘       │
│                                                                  │
│  (CUE compilation is transparent - developers never see it)      │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Compilation Approach

The SDK uses **declarative capture** rather than Go AST transformation:
- Template functions execute with a tracing context
- Each `Set()`, `If()`, `tpl.Output()` call is recorded
- Parameter variables track their usage and generate corresponding CUE paths
- Recorded operations form a declarative tree that maps directly to CUE

### How Parameter Variables Work

Parameter variables like `image := defkit.String("image").Required()` are **expression builders**, not value holders:

```go
// What you write:
image := defkit.String("image").Required()
deploy.Set("spec.image", image)

// What happens internally:
// 1. defkit.String("image") creates a Param{name: "image", type: "string"}
// 2. deploy.Set() receives this Param and records: SetOp{path: "spec.image", value: ParamRef("image")}
// 3. During compilation, ParamRef("image") becomes `parameter.image` in CUE
```

**Conditional handling** uses the fluent API rather than Go's `if`:

```go
// For optional parameters, use SetIf or the If() fluent method:
deploy.SetIf(cpu.IsSet(), "spec.resources.limits.cpu", cpu)

// Or with fluent chaining:
deploy.If(cpu.IsSet()).Set("spec.resources.limits.cpu", cpu)

// Both compile to CUE:
// if parameter.cpu != _|_ {
//     spec: resources: limits: cpu: parameter.cpu
// }
```

This approach is similar to how query builders (like GORM, Squirrel) work - the Go code describes operations declaratively without executing them at definition time.

---

## Testing

defkit provides testing support using Ginkgo and Gomega with custom matchers, enabling BDD-style test-driven development without requiring a Kubernetes cluster.

### Test Levels

| Test Level | Framework | Cluster Required | What It Tests |
|------------|-----------|------------------|---------------|
| Unit tests | Ginkgo/Gomega | No | Parameter validation, template output, conditional logic |
| CUE compilation | Ginkgo/Gomega | No | Generated CUE is syntactically valid |
| Integration | envtest | No | Controller reconciliation with fake cluster |
| E2E | Full cluster | Yes | Complete deployment lifecycle |

### Custom Matchers

defkit provides custom Gomega matchers for readable, expressive tests:

```go
// Resource type matchers
BeDeployment(), BeService(), BeIngress(), BeConfigMap(), BeSecret()
BeResourceOfKind(kind string)

// Metadata matchers
HaveAPIVersion(version string), HaveName(name string), HaveNamespace(ns string)
HaveLabel(key, value string), HaveAnnotation(key, value string)

// Spec matchers
HaveReplicas(count int), HaveImage(image string), HaveContainerNamed(name string)
HavePort(port int), HaveEnvVar(name, value string)
HaveResourceLimit(resource, value string), HaveResourceRequest(resource, value string)

// Path-based matcher for any field
HaveFieldPath(path string, value any)

// Validation and health matchers
FailValidationWith(substring string), PassValidation()
BeHealthy(), BeUnhealthy(), HaveHealthMessage(msg string)
```

### Example: Testing a ComponentDefinition

```go
package webservice_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/oam-dev/kubevela/pkg/defkit"
    . "github.com/oam-dev/kubevela/pkg/defkit/testing/matchers"
)

var _ = Describe("Webservice ComponentDefinition", func() {
    var def *component.Definition

    BeforeEach(func() {
        def = webservice.New()
    })

    Describe("Template Rendering", func() {
        It("should render a deployment with defaults", func() {
            ctx := defkit.TestContext().
                WithName("my-app").
                WithNamespace("production").
                WithParam("image", "nginx:1.21")

            output := def.Render(ctx)

            Expect(output).To(BeDeployment())
            Expect(output).To(HaveAPIVersion("apps/v1"))
            Expect(output).To(HaveName("my-app"))
            Expect(output).To(HaveReplicas(3)) // default value
            Expect(output).To(HaveImage("nginx:1.21"))
        })

        It("should set resource limits when cpu is provided", func() {
            ctx := defkit.TestContext().
                WithName("my-app").
                WithParam("image", "nginx:1.21").
                WithParam("cpu", "500m")

            Expect(def.Render(ctx)).To(HaveResourceLimit("cpu", "500m"))
        })
    })

    Describe("Parameter Validation", func() {
        It("should fail when required image is missing", func() {
            ctx := defkit.TestContext().WithName("my-app")
            Expect(def.Validate(ctx)).To(FailValidationWith("image is required"))
        })

        It("should fail when replicas exceeds maximum", func() {
            ctx := defkit.TestContext().
                WithName("my-app").
                WithParam("image", "nginx:1.21").
                WithParam("replicas", 200)

            Expect(def.Validate(ctx)).To(FailValidationWith("replicas must be <= 100"))
        })
    })

    Describe("Health Policy", func() {
        It("should report healthy when all replicas are ready", func() {
            ctx := defkit.TestContext().
                WithName("my-app").
                WithParam("image", "nginx:1.21").
                WithParam("replicas", 3).
                WithOutputStatus(map[string]any{"readyReplicas": 3})

            Expect(def.EvaluateHealth(ctx)).To(BeHealthy())
            Expect(def.EvaluateHealth(ctx)).To(HaveHealthMessage("Ready: 3/3"))
        })
    })
})
```

### Table-Driven Tests

```go
DescribeTable("parameter combinations",
    func(params map[string]any, expectedField string, expectedValue any, shouldFail bool) {
        ctx := defkit.TestContext().WithName("test")
        for k, v := range params {
            ctx = ctx.WithParam(k, v)
        }

        if shouldFail {
            Expect(def.Validate(ctx)).NotTo(PassValidation())
            return
        }
        Expect(def.Render(ctx)).To(HaveFieldPath(expectedField, expectedValue))
    },
    Entry("minimal config uses default replicas",
        map[string]any{"image": "nginx:1.21"}, "spec.replicas", 3, false),
    Entry("custom replicas are respected",
        map[string]any{"image": "nginx:1.21", "replicas": 5}, "spec.replicas", 5, false),
    Entry("missing image fails validation",
        map[string]any{"replicas": 3}, "", nil, true),
)
```

### Testing Traits

```go
var _ = Describe("RateLimit Trait", func() {
    It("should patch workload with annotation", func() {
        workload := defkit.NewResource("apps/v1", "Deployment").SetName("my-app")

        ctx := defkit.TestContext().
            WithWorkload(workload).
            WithParam("rps", 1000)

        patches := ratelimit.New().Patch(ctx)
        Expect(patches).To(HaveAnnotation("ratelimit.example.com/rps", "1000"))
    })
})
```

### Testing Cluster Version Conditionals

```go
var _ = Describe("CronJob", func() {
    It("should use v1beta1 on old clusters", func() {
        ctx := defkit.TestContext().
            WithName("my-job").
            WithParam("schedule", "0 * * * *").
            WithClusterVersion(1, 24)

        Expect(cronjob.New().Render(ctx)).To(HaveAPIVersion("batch/v1beta1"))
    })

    It("should use v1 on new clusters", func() {
        ctx := defkit.TestContext().
            WithName("my-job").
            WithParam("schedule", "0 * * * *").
            WithClusterVersion(1, 25)

        Expect(cronjob.New().Render(ctx)).To(HaveAPIVersion("batch/v1"))
    })
})
```

### What to Test vs What NOT to Test

#### DO Test

| What | Why | Example |
|------|-----|---------|
| Parameter validation | Catch invalid inputs early | Required fields, min/max constraints |
| Template output structure | Ensure correct K8s resources | Resource kind, API version, spec fields |
| Conditional logic | Verify branches work correctly | If cpu is set, limits are added |
| Default values | Ensure defaults are applied | replicas defaults to 3 |
| Health policy evaluation | Verify health checks work | Ready when replicas match |
| Auxiliary outputs | Verify all resources generated | Service, Ingress alongside Deployment |
| Edge cases | Handle unusual inputs | Empty arrays, nil values |

#### DO NOT Test

| What | Why |
|------|-----|
| CUE language behavior | CUE is well-tested; trust it |
| Controller reconciliation logic | Out of defkit's scope |
| Kubernetes API behavior | Not your code |
| Network operations | Use mocks/fakes for integration tests |

### Testing Anti-Patterns to Avoid

```go
// ❌ BAD: Testing CUE internals
func TestBad(t *testing.T) {
    cue, _ := def.ToCUE()
    assert.Contains(t, cue, "replicas: parameter.replicas") // Too brittle
}

// ✅ GOOD: Test the behavior, not the implementation
func TestGood(t *testing.T) {
    ctx := defkit.TestContext().WithParam("replicas", 5)
    output := def.Render(ctx)
    assert.Equal(t, 5, output.Get("spec.replicas"))
}

// ❌ BAD: Hardcoding exact output
func TestBad2(t *testing.T) {
    output := def.Render(ctx)
    expected := `{"apiVersion":"apps/v1",...}` // Brittle, hard to maintain
    assert.Equal(t, expected, output.ToJSON())
}

// ✅ GOOD: Assert on specific fields you care about
func TestGood2(t *testing.T) {
    output := def.Render(ctx)
    assert.Equal(t, "apps/v1", output.APIVersion())
    assert.Equal(t, 3, output.Get("spec.replicas"))
}
```

### Integration Testing with envtest

For testing controller integration without a full cluster:

```go
package integration_test

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
    "sigs.k8s.io/controller-runtime/pkg/envtest"

    "myplatform/definitions/webservice"
)

func TestWebserviceIntegration(t *testing.T) {
    // Start envtest
    testEnv := &envtest.Environment{}
    cfg, err := testEnv.Start()
    require.NoError(t, err)
    defer testEnv.Stop()

    // Create client
    k8sClient, err := client.New(cfg, client.Options{})
    require.NoError(t, err)

    // Apply definition
    def := webservice.New()
    cr, err := def.BuildCR()
    require.NoError(t, err)

    err = k8sClient.Create(context.Background(), cr)
    require.NoError(t, err)

    // Verify definition was created
    var retrieved v1beta1.ComponentDefinition
    err = k8sClient.Get(context.Background(),
        types.NamespacedName{Name: "webservice", Namespace: "vela-system"},
        &retrieved)
    require.NoError(t, err)
    assert.Equal(t, "webservice", retrieved.Name)
}
```

---

## Definition Deployment Workflow

### Development Cycle

```
┌─────────────────────────────────────────────────────────────────┐
│                    Definition Development Cycle                   │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  1. WRITE          2. TEST           3. VALIDATE    4. APPLY    │
│  ┌──────────┐     ┌──────────┐      ┌──────────┐  ┌──────────┐  │
│  │  Go Code │────▶│go test   │─────▶│vela def  │─▶│vela def  │  │
│  │          │     │          │      │vet       │  │apply     │  │
│  └──────────┘     └──────────┘      └──────────┘  └──────────┘  │
│       │                │                  │             │        │
│       │                │                  │             │        │
│       └────────────────┴──────────────────┴─────────────┘        │
│                         ITERATE                                  │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### CI/CD Integration

```yaml
# .github/workflows/definitions.yaml
name: Definition CI

on:
  push:
    paths:
      - 'definitions/**'

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Run Unit Tests
        run: go test ./definitions/... -v

      - name: Validate Definitions
        run: vela def vet ./definitions/

      - name: Generate CUE (dry-run)
        run: vela def apply ./definitions/ --dry-run

  deploy:
    needs: test
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Apply Definitions
        run: vela def apply ./definitions/
        env:
          KUBECONFIG: ${{ secrets.KUBECONFIG }}
```

---

## Distribution

### Go Modules

Go definitions are distributed as standard Go packages:

```go
// go.mod for your definitions package
module github.com/myorg/platform-defs

require github.com/oam-dev/kubevela/pkg/defkit v0.1.0
```

```bash
# Consumers import your definitions
go get github.com/myorg/platform-defs@v1.0.0
```

```go
// Use in application code
import "github.com/myorg/platform-defs/components/webservice"

app := application.New("my-app").
    AddComponent(webservice.New("api").Image("myapp:v1"))
```

This leverages Go's existing ecosystem: semantic versioning, checksums, proxy caching, and private module support.

### GitOps

```bash
# Render Go definitions to CUE/YAML for GitOps (extends `vela def render`)
vela def render ./definitions/ --output ./dist/ --format yaml

# Commit and sync with ArgoCD/Flux
git add ./dist/
git commit -m "Update definitions"
git push
```

---

## Implementation Plan

### Phase 1: Core Framework
- defkit Go package with component, trait, policy, workflow modules
- TemplateContext API (`tpl.Output()`, `tpl.Outputs()`, `tpl.ClusterVersion()`, etc.)
- Parameter type system with typed variables and validation
- Schema-agnostic resource builder with fluent conditionals
- IR→CUE compiler
- CLI integration (`vela def apply` for Go files)
- `RawCUE()` escape hatch

### Phase 2: Complete Definition Support
- All definition types fully implemented
- Patch strategies for traits
- Workflow operations
- Status and health policy support
- OneOf/discriminated union support

### Phase 3: Distribution & Ecosystem
- OCI registry support
- Testing utilities (`defkit.TestContext`)
- Migration tooling (CUE→Go import)
- Documentation and examples

### Phase 4: Advanced Features
- Other languages based on community demand
- IDE plugins
- Definition composition

---

## Compatibility

### Coexistence with CUE

- CUE definitions remain fully supported
- defkit is an alternative path, not a replacement
- Both produce valid X-Definition CRs

### Migration

```bash
# Optional: Import existing CUE to Go (inverse of gen-api, NEW subcommand)
vela def gen-go ./legacy-definitions/*.cue --output ./converted/
```

---

## FAQ

**Q: Can I still write CUE definitions?**
A: Yes. CUE definitions remain fully supported. defkit is an alternative.

**Q: How do I access runtime values like readyReplicas?**
A: Use `tpl.Output().Status().Field("readyReplicas")` inside template/policy functions. This generates CUE that accesses `context.output.status.readyReplicas` at runtime.

**Q: What about CUE unification (`&`)?**
A: Common patterns like defaults with runtime values are handled automatically. For complex unification, use `RawCUE()`.

**Q: Why Go first?**
A: Go is KubeVela's implementation language and widely used in the Kubernetes ecosystem. Other languages may follow based on community demand.

**Q: How do I test definitions without a cluster?**
A: Use `defkit.TestContext()` to create mock contexts with parameters, cluster version, and output status. All testing can be done with standard Go testing frameworks.

**Q: Can I see the generated CUE?**
A: Yes. Use `vela def render ./definition.go --output cue` or `def.ToCUE()` in tests.
