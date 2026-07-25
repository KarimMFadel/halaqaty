# SOLID — the five principles

Source: Robert C. Martin. The five principles were collected on Uncle Bob's "Principles of OOD" page on objectmentor.com (mirrored at butunclebob.com) and updated on blog.cleancoder.com.

## Contents

- S: Single Responsibility Principle
- O: Open/Closed Principle
- L: Liskov Substitution Principle
- I: Interface Segregation Principle
- D: Dependency Inversion Principle
- How AI-generated code typically breaks SOLID
- Self-check for SOLID

---

## S — Single Responsibility Principle

**Definition (Martin 2014, hardened from the original).** *"A module should be responsible to one, and only one, actor."*

Older form: "A class should have only one reason to change."

### Why

The axis is *people*. Different stakeholders (Accounting, Auth, Reporting) want different things from the same class. When their needs change, they edit the same file, conflict, and break each other.

### Smells to flag

- One struct contains methods touching unrelated subsystems (persistence + presentation + business rules).
- Methods on the struct serve disjoint stakeholder groups.

### Halaqaty examples

**Bad:**
```go
type CircleService struct { ... }
func (s *CircleService) CreateCircle(...) // product logic
func (s *CircleService) SendFCMNotification(...) // notification concern
func (s *CircleService) SaveToStorage(...) // persistence concern
```

**Good:**
```go
type CircleService struct { ... }       // product logic
type NotificationService struct { ... } // notification concern
type CircleRepository struct { ... }    // persistence concern
```

---

## O — Open/Closed Principle

**Definition.** *"Software entities should be open for extension, but closed for modification."*

Source: blog.cleancoder.com — OCP, 2014.

### Smells to flag

- Branch dispatching on a type tag — every new type requires editing the same function.
- `switch`/`if` chains that cross module boundaries.

### Go-specific

Use interface dispatch. Avoid `switch v := x.(type)` chains that grow with new types. Instead, put the behavior on the type via an interface.

**Bad:**
```go
func export(record any, kind string) []byte {
    switch kind {
    case "pdf":  return toPdf(record)
    case "csv":  return toCsv(record)
    }
}
// adding "json" requires editing this function
```

**Good:**
```go
type Exporter interface { Export(record any) []byte }

exporters := map[string]Exporter{
    "pdf": &PDFExporter{},
    "csv": &CSVExporter{},
}
// adding "json" is one line in the map
```

---

## L — Liskov Substitution Principle

**Definition.** A subtype must be substitutable for its base type without altering program correctness.

### Smells to flag

- A type overrides an interface method to signal "not implemented" or "unsupported operation."
- A type **strengthens preconditions** (rejects inputs the interface contract accepts).
- A type **weakens postconditions** (returns something the interface guarantees against).
- Callers perform runtime type checks to decide whether to call a method.

### Go-specific

Never implement an interface with `panic("not implemented")` in production code. Use `errors.ErrUnsupported` only when the contract explicitly allows it. If a type cannot honor an interface, the interface boundary is wrong.

---

## I — Interface Segregation Principle

**Definition.** *"Clients should not be forced to depend on methods they do not use."*

### Smells to flag

- A `Service`/`Manager`/`Repository` interface with 10+ methods, where any given caller uses one or two.
- Implementations that stub half the methods with no-op bodies.
- One mock object reconfigured differently across tests because the interface is too broad.

### Go-specific (idiomatic alignment)

Go's small-interface idiom is ISP in practice: prefer many small interfaces (`io.Reader`, `io.Writer`) over one large one. Define interfaces at the point of use, not at the point of implementation:

```go
// In the service package (client) — not in the postgres package (implementation)
type CircleReader interface {
    FindByID(ctx context.Context, id uuid.UUID) (*Circle, error)
}

type CircleWriter interface {
    Create(ctx context.Context, circle *Circle) error
    Update(ctx context.Context, circle *Circle) error
}
```

---

## D — Dependency Inversion Principle

**Definition.**
*(a) High-level modules should not depend on low-level modules. Both should depend on abstractions.*
*(b) Abstractions should not depend on details. Details should depend on abstractions.*

### Smells to flag

- A high-level module imports a concrete low-level client inside business logic.
- A constructor that instantiates concrete collaborators instead of accepting them as parameters.
- Abstractions defined in the *low-level* package (interface lives next to its database or service implementation) — ownership reversed. The interface should live in the **client's** package.

### Go-specific

"Accept interfaces, return concrete types" is the Go idiom. It embodies DIP: the interface lives in the package that uses it, not next to the implementation.

```go
// WRONG — service depends on a concrete postgres type
import "halaqaty/backend/internal/postgres"
func NewCircleService(repo *postgres.CircleRepository) *CircleService { ... }

// CORRECT — service depends on an interface it owns
// interface defined in the service package:
type CircleRepository interface {
    FindByID(ctx context.Context, id uuid.UUID) (*Circle, error)
    Create(ctx context.Context, c *Circle) error
}
func NewCircleService(repo CircleRepository) *CircleService { ... }
```

The `postgres.CircleRepository` concrete type satisfies the interface; the service package never imports `postgres`.

---

## How AI-generated code typically breaks SOLID

1. **God-struct** from "do everything in one struct/file" prompts — SRP + DIP + OCP.
2. **Type-tag dispatch chains** — OCP.
3. **`panic("not implemented")` stubs** when asked to "implement only the methods we need" — LSP + ISP.
4. **Concrete SDK/client imports at module load time** — DIP, hard to test.
5. **Mega-`Service` interfaces** with create/read/update/delete/email/notify/audit — ISP, usually SRP too.
6. **Inverted ownership of abstractions** — putting the interface in the same package as the concrete implementation. Cosmetic DIP fix, real dependency graph unchanged.

---

## Self-check for SOLID

Before you ship code:

1. (SRP) Does any struct in the diff answer to more than one stakeholder group?
2. (OCP) Does any change require a type-tag branch added to an existing function? Could it be data-driven instead?
3. (LSP) Does any new type signal "not implemented", tighten preconditions, or weaken postconditions?
4. (ISP) Does any interface have a method the concrete client doesn't use? Is the interface in the right package?
5. (DIP) Does the high-level package import the low-level concrete? Where do new interfaces live — with the client or with the implementation?
