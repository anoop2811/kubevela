# KubeVela Definition Kit (defkit) - Overview

## The Problem

Today, platform engineers who want to extend KubeVela must write definitions in CUE - a configuration language that most developers don't know. This creates several challenges:

1. **Steep learning curve** - CUE has unique concepts (unification, lattices, structural typing) that take weeks to master
2. **Poor IDE support** - Limited autocomplete, type checking, and debugging compared to mainstream languages
3. **No existing ecosystem** - Can't leverage familiar testing frameworks, CI/CD tools, or package managers
4. **Maintenance burden** - The current `vela def gen-api` requires Docker and generates verbose, hard-to-use code

### Real-World Impact

```
Platform Engineer: "I just want to create a simple component definition..."

Current Reality:
- Learn CUE syntax and semantics
- Understand KubeVela's CUE conventions
- No unit testing for definitions
- Can't share definitions via npm/pip/maven
```

## The Solution: Definition Kit (defkit)

**Write KubeVela definitions in the language you already know.**

defkit is a multi-language SDK that lets you create ComponentDefinitions, TraitDefinitions, PolicyDefinitions, and WorkflowStepDefinitions using Go, TypeScript, Python, or Java.

### Before (CUE)

```cue
webservice: {
    type: "component"
    attributes: workload: definition: {
        apiVersion: "apps/v1"
        kind: "Deployment"
    }
}
template: {
    output: {
        apiVersion: "apps/v1"
        kind: "Deployment"
        metadata: name: context.name
        spec: {
            replicas: parameter.replicas
            selector: matchLabels: app: context.name
            template: {
                metadata: labels: app: context.name
                spec: containers: [{
                    name:  context.name
                    image: parameter.image
                    ports: [{containerPort: parameter.port}]
                }]
            }
        }
    }
    parameter: {
        image:    string
        replicas: *3 | int
        port:     *8080 | int
    }
}
```

### After (Go with defkit)

```go
func WebserviceComponent() *component.Definition {
    return component.New("webservice").
        Description("A web service component").
        Parameter("image", defkit.String().Required()).
        Parameter("replicas", defkit.Int().Default(3)).
        Parameter("port", defkit.Int().Default(8080)).
        Template(func(ctx *component.Context) *component.Output {
            return ctx.Output(
                defkit.NewResource("apps/v1", "Deployment").
                    SetName(ctx.Name()).
                    Set("spec.replicas", ctx.Param("replicas").Int()).
                    Set("spec.template.spec.containers", []map[string]any{{
                        "name":  ctx.Name(),
                        "image": ctx.Param("image").String(),
                        "ports": []map[string]any{{
                            "containerPort": ctx.Param("port").Int(),
                        }},
                    }}),
            )
        })
}
```

## Key Design Decisions

### 1. Flexible Resource Construction

defkit does **not** ship typed Kubernetes helpers like `ctx.Deployment()` or `ctx.Service()`.

**Why?** Because that would require KubeVela to:
- Track every Kubernetes release (1.28, 1.29, 1.30...)
- Handle API deprecations (`extensions/v1beta1` → `apps/v1`)
- Support multiple K8s versions simultaneously

Instead, defkit provides three approaches:

#### Option A: Unstructured Builder (Default)

A universal builder that works with any resource:

```go
// Works with ANY Kubernetes resource - core or custom
defkit.NewResource("apps/v1", "Deployment").
    SetName("my-app").
    Set("spec.replicas", 3).
    Set("spec.template.spec.containers", []map[string]any{{
        "name":  "app",
        "image": "nginx:latest",
    }})
```

#### Option B: Bring Your Own Types (Optional)

If you want type safety and IDE autocomplete, bring your own typed K8s structs:

```go
import (
    appsv1 "k8s.io/api/apps/v1"
    corev1 "k8s.io/api/core/v1"
)

// Use familiar typed K8s objects, then convert
deploy := &appsv1.Deployment{
    Spec: appsv1.DeploymentSpec{
        Replicas: ptr.To(int32(3)),
        Template: corev1.PodTemplateSpec{
            Spec: corev1.PodSpec{
                Containers: []corev1.Container{{
                    Name:  "app",
                    Image: "nginx:latest",
                }},
            },
        },
    },
}

// Convert to defkit.Resource
return defkit.FromTyped(deploy).SetName(ctx.Name())
```

#### Option C: Generate Types from CRDs (Optional)

For custom CRDs, generate typed structs:

```bash
# Generate types from Crossplane CRDs
vela defkit gen-types --crd=database.aws.crossplane.io/v1beta1 --output=./types
```

```go
import "myplatform/types/crossplane"

instance := &crossplane.DBInstance{
    Spec: crossplane.DBInstanceSpec{
        Engine:        "postgres",
        InstanceClass: "db.t3.micro",
    },
}
return defkit.FromTyped(instance)
```

**The choice is yours** - use unstructured for simplicity, or bring typed objects for IDE support. defkit doesn't force either approach.

#### Composability: Reusable Resource Builders

Since `defkit.NewResource()` returns a pointer, you can create reusable helper functions:

```go
// Reusable helper for creating containers
func standardContainer(name, image string, port int) map[string]any {
    return map[string]any{
        "name":  name,
        "image": image,
        "ports": []map[string]any{{"containerPort": port}},
        "resources": map[string]any{
            "requests": map[string]any{"cpu": "100m", "memory": "128Mi"},
            "limits":   map[string]any{"cpu": "500m", "memory": "512Mi"},
        },
    }
}

// Reusable helper for creating deployments
func baseDeployment(name, namespace string, replicas int) *defkit.Resource {
    return defkit.NewResource("apps/v1", "Deployment").
        SetName(name).
        SetNamespace(namespace).
        Set("spec.replicas", replicas).
        Set("spec.selector.matchLabels", map[string]string{"app": name}).
        Set("spec.template.metadata.labels", map[string]string{"app": name})
}

// Use in your component definition
func MyComponent() *component.Definition {
    return component.New("my-component").
        // ... parameters ...
        Template(func(ctx *component.Context) *component.Output {
            name := ctx.Param("name").String()

            // Reuse the helper, then customize
            deploy := baseDeployment(name, ctx.Namespace(), ctx.Param("replicas").Int()).
                Set("spec.template.spec.containers", []map[string]any{
                    standardContainer(name, ctx.Param("image").String(), 8080),
                })

            return ctx.Output(deploy)
        })
}
```

This enables:
- **Shared patterns** across multiple definitions
- **Platform-wide defaults** (resource limits, labels, annotations)
- **Testable units** - test helpers independently

### 2. CUE Becomes Optional, Not Required

defkit gives developers a choice. Write definitions in Go/TypeScript/Python/Java, and the CLI compiles to CUE:

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Go/TS/Py/Java │ ──▶ │   vela def      │ ──▶ │   CUE Output    │
│   Definition    │     │   compile       │     │                 │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

**CUE remains fully supported.** Platform engineers who prefer CUE or have existing CUE definitions can continue using them. defkit is an alternative path for those who want:
- Full IDE support (autocomplete, type checking)
- Unit testing with familiar frameworks
- Package distribution via npm/pip/maven

Both approaches produce the same CUE output that KubeVela's controller understands.

### 3. Multi-Language Support

| Language   | Package Manager | Testing Framework |
|------------|-----------------|-------------------|
| Go         | Go modules      | go test           |
| TypeScript | npm             | Jest/Vitest       |
| Python     | pip             | pytest            |
| Java       | Maven/Gradle    | JUnit             |

## What You Can Build

### Component Definitions
Define how workloads are deployed (Deployments, StatefulSets, custom CRDs).

### Trait Definitions
Add capabilities to components (storage, networking, autoscaling).

### Policy Definitions
Control application-level behavior (traffic splitting, topology).

### Workflow Step Definitions
Define delivery operations (approvals, notifications, deployments).

## Example: Crossplane AWS RDS

defkit makes it trivial to create definitions for any CRD:

```go
func RDSComponent() *component.Definition {
    return component.New("aws-rds").
        Description("Provision AWS RDS via Crossplane").
        Parameter("engine", defkit.Enum("mysql", "postgres").Default("postgres")).
        Parameter("instanceClass", defkit.String().Default("db.t3.micro")).
        Parameter("storageGB", defkit.Int().Default(20)).
        Template(func(ctx *component.Context) *component.Output {
            return ctx.Output(
                defkit.NewResource("database.aws.crossplane.io/v1beta1", "DBInstance").
                    SetName(ctx.Name()).
                    Set("spec.forProvider", map[string]any{
                        "engine":          ctx.Param("engine").String(),
                        "instanceClass":   ctx.Param("instanceClass").String(),
                        "allocatedStorage": ctx.Param("storageGB").Int(),
                    }),
            )
        })
}
```

## CLI Commands

```bash
# Create a new definition project
vela def init my-platform --language=go

# Compile definitions to CUE (transparent)
vela def compile ./definitions

# Apply to cluster
vela def apply ./definitions

# Generate client SDK for application developers
vela def gen-sdk --output=./sdk
```

## Benefits Summary

| Aspect | Before (CUE) | After (defkit) |
|--------|--------------|----------------|
| Learning curve | Weeks | Hours |
| IDE support | Limited | Full autocomplete & type checking |
| Testing | Manual validation | Unit tests with familiar frameworks |
| Distribution | Copy files | npm/pip/maven packages |
| Type safety | CUE types | Native language types |
| Error timing | Runtime (CUE evaluation) | Many caught at compile time |
| Ecosystem | None | Full language ecosystem |

> **Note**: Since defkit compiles to CUE, some runtime errors during template evaluation may still originate from the CUE engine. However, defkit catches many errors earlier through native type systems and generates known-good CUE patterns, reducing the surface area for CUE-related issues.

## Next Steps

1. **RFC Discussion** - Gather community feedback on the design
2. **Go SDK** - Implement the core defkit library
3. **CLI Integration** - Add `vela def compile` command
4. **Multi-language** - TypeScript, Python, Java SDKs
5. **Documentation** - Tutorials and migration guides

---

*For the complete technical specification, see [next-gen-sdk-framework.md](./next-gen-sdk-framework.md)*
