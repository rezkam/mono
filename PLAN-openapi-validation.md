# OpenAPI Validation Middleware Integration Plan

## Overview

This plan integrates `nethttp-middleware` for automatic OpenAPI request/response validation while maintaining clean DDD/hexagonal architecture separation.

**Key Principles:**
1. **OpenAPI validates contract** (format, types, enums, constraints)
2. **Domain validates business rules** (archived item exclusion, etag semantics)
3. **No layer pollution** - each layer has single responsibility
4. **Response validation in dev/test mode** - catch spec drift early

---

## Architecture After Implementation

```
                         HTTP Layer
  +-------------+  +------------------+  +----------------------+
  | Chi Router  |->| OpenAPI Validator|->| Auth Middleware      |
  |             |  | (nethttp-mw)     |  |                      |
  | Routes      |  | - Request body   |  | - API key validation |
  |             |  | - Query params   |  |                      |
  |             |  | - Path params    |  |                      |
  |             |  | - Response body  |  |                      |
  +-------------+  +------------------+  +----------------------+
                           |
  +------------------------------------------------------------+
  | Handlers (thin)                                             |
  | - Decode pre-validated request                              |
  | - Call application service                                  |
  | - Map domain -> DTO                                         |
  | - NO validation logic                                       |
  +------------------------------------------------------------+
                           |
                      Application Layer
  +------------------------------------------------------------+
  | Service                                                     |
  | - Orchestrates use cases                                    |
  | - Applies business defaults (excluded statuses, pagination) |
  | - Calls domain value object constructors                    |
  | - NO HTTP knowledge                                         |
  +------------------------------------------------------------+
                           |
                        Domain Layer
  +------------------------------------------------------------+
  | Value Objects (validated at construction)                   |
  | - Title: required, 1-255 chars                              |
  | - TaskStatus: enum validation                               |
  | - TaskPriority: enum validation                             |
  | - RecurrencePattern: enum validation                        |
  | - ItemsFilter: composite validation                         |
  |                                                             |
  | Aggregates                                                  |
  | - TodoList, TodoItem, RecurringTemplate                     |
  |                                                             |
  | Domain Errors                                               |
  | - Pure business errors, no HTTP knowledge                   |
  +------------------------------------------------------------+
```

---

## What Gets Validated Where

### OpenAPI Middleware (HTTP Layer) - Contract Validation

| Validation | OpenAPI Constraint | Current Location | After |
|------------|-------------------|------------------|-------|
| Title required | `required: [title]` | Domain | OpenAPI |
| Title length | `minLength: 1, maxLength: 255` | Domain | OpenAPI |
| UUID format | `format: uuid` | oapi-codegen | OpenAPI |
| DateTime format | `format: date-time` | Manual | OpenAPI |
| Status enum | `enum: [todo, in_progress, ...]` | Domain | OpenAPI |
| Priority enum | `enum: [low, medium, high, urgent]` | Domain | OpenAPI |
| Pattern enum | `enum: [daily, weekly, ...]` | Domain | OpenAPI |
| Page size range | `minimum: 1, maximum: 100` | Service | OpenAPI |
| Status filter count | `maxItems: 6` | Domain | OpenAPI |
| Priority filter count | `maxItems: 4` | Domain | OpenAPI |
| Tags filter count | `maxItems: 5` | Domain | OpenAPI |
| Generation window | `minimum: 1, maximum: 365` | Domain | OpenAPI |
| Sort by enum | `enum: [due_time, priority, ...]` | Domain | OpenAPI |
| Sort dir enum | `enum: [asc, desc]` | Domain | OpenAPI |

### Domain Layer - Business Rule Validation (KEEP)

| Validation | Why Domain |
|------------|-----------|
| Etag format (numeric string) | Semantic business rule for optimistic concurrency |
| IANA timezone validation | Requires runtime check against IANA database |
| ISO 8601 duration parsing | Complex parsing with business meaning |
| Default excluded statuses | Business policy (archived/cancelled hidden) |
| Default pagination limits | Business policy applied in application layer |

---

## Implementation Tasks

### Phase 1: OpenAPI Spec Enhancement

**File: `api/openapi/mono.yaml`**

Add missing validation constraints to make the spec complete:

```yaml
# 1. Add pattern validation for duration strings
estimated_duration:
  type: string
  pattern: '^P(?:\d+Y)?(?:\d+M)?(?:\d+W)?(?:\d+D)?(?:T(?:\d+H)?(?:\d+M)?(?:\d+(?:\.\d+)?S)?)?$'
  description: ISO 8601 duration (e.g., 'PT2H30M')

# 2. Add title constraints to TodoItem (for UpdateItemRequest)
TodoItem:
  type: object
  properties:
    title:
      type: string
      minLength: 1
      maxLength: 255
    # ... rest of properties

# 3. Add update_mask validation
UpdateItemRequest:
  type: object
  required:
    - update_mask  # Make required - must specify what to update
  properties:
    item:
      $ref: '#/components/schemas/TodoItem'
    update_mask:
      type: array
      minItems: 1  # Must update at least one field
      items:
        type: string
        enum: [title, status, priority, due_time, tags, estimated_duration, actual_duration, timezone]

# 4. Add valid update_mask fields for recurring templates
UpdateRecurringTemplateRequest:
  type: object
  required:
    - update_mask
  properties:
    template:
      $ref: '#/components/schemas/RecurringItemTemplate'
    update_mask:
      type: array
      minItems: 1
      items:
        type: string
        enum: [title, tags, priority, estimated_duration, recurrence_pattern, recurrence_config, due_offset, is_active, generation_window_days]

# 5. Add etag pattern validation
TodoItem:
  properties:
    etag:
      type: string
      pattern: '^\d+$'
      description: Numeric version string for optimistic concurrency
```

### Phase 2: Create Validation Middleware Package

**New file: `internal/http/middleware/validation.go`**

```go
package middleware

import (
    "context"
    "encoding/json"
    "net/http"

    "github.com/getkin/kin-openapi/openapi3"
    "github.com/getkin/kin-openapi/openapi3filter"
    nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
)

// ValidationConfig controls validation behavior.
type ValidationConfig struct {
    // ValidateResponses enables response validation (recommended for dev/test).
    ValidateResponses bool
    // MultiError returns all validation errors instead of first.
    MultiError bool
}

// ValidationError represents a structured validation error.
type ValidationError struct {
    Code    string            `json:"code"`
    Message string            `json:"message"`
    Details []ValidationField `json:"details,omitempty"`
}

// ValidationField describes a field-specific validation error.
type ValidationField struct {
    Field string `json:"field"`
    Issue string `json:"issue"`
}

// NewValidator creates OpenAPI validation middleware.
// The spec should have Servers cleared to avoid Host header validation.
func NewValidator(spec *openapi3.T, config ValidationConfig) func(http.Handler) http.Handler {
    // Clear servers to avoid Host header validation issues
    spec.Servers = nil

    opts := &nethttpmiddleware.Options{
        Options: openapi3filter.Options{
            MultiError: config.MultiError,
            // Skip auth - handled by separate auth middleware
            AuthenticationFunc: func(ctx context.Context, input *openapi3filter.AuthenticationInput) error {
                return nil
            },
        },
        ErrorHandlerWithOpts: validationErrorHandler,
    }

    return nethttpmiddleware.OapiRequestValidatorWithOptions(spec, opts)
}

// validationErrorHandler formats validation errors as structured JSON.
func validationErrorHandler(ctx context.Context, err error, w http.ResponseWriter, r *http.Request, opts nethttpmiddleware.ErrorHandlerOpts) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(opts.StatusCode)

    // Parse error to extract field details
    details := extractValidationDetails(err)

    response := struct {
        Error ValidationError `json:"error"`
    }{
        Error: ValidationError{
            Code:    "VALIDATION_ERROR",
            Message: err.Error(),
            Details: details,
        },
    }

    json.NewEncoder(w).Encode(response)
}

// extractValidationDetails parses openapi3filter errors into field details.
func extractValidationDetails(err error) []ValidationField {
    var details []ValidationField

    // Handle RequestError which contains schema errors
    if reqErr, ok := err.(*openapi3filter.RequestError); ok {
        if schemaErr, ok := reqErr.Err.(*openapi3.SchemaError); ok {
            details = append(details, ValidationField{
                Field: schemaErr.JSONPointer(),
                Issue: schemaErr.Reason,
            })
        }
    }

    // Handle MultiError
    if multiErr, ok := err.(openapi3.MultiError); ok {
        for _, e := range multiErr {
            if schemaErr, ok := e.(*openapi3.SchemaError); ok {
                details = append(details, ValidationField{
                    Field: schemaErr.JSONPointer(),
                    Issue: schemaErr.Reason,
                })
            }
        }
    }

    return details
}
```

### Phase 3: Create Response Validation Middleware (Dev/Test Only)

**New file: `internal/http/middleware/response_validation.go`**

```go
package middleware

import (
    "bytes"
    "io"
    "net/http"

    "github.com/getkin/kin-openapi/openapi3"
    "github.com/getkin/kin-openapi/openapi3filter"
    "github.com/getkin/kin-openapi/routers"
    "github.com/getkin/kin-openapi/routers/gorillamux"
)

// ResponseValidator validates outgoing responses against OpenAPI spec.
// Use in development/testing to catch spec drift.
type ResponseValidator struct {
    router routers.Router
    logger func(msg string, args ...any)
}

// NewResponseValidator creates a response validation middleware.
func NewResponseValidator(spec *openapi3.T, logger func(msg string, args ...any)) (*ResponseValidator, error) {
    router, err := gorillamux.NewRouter(spec)
    if err != nil {
        return nil, err
    }
    return &ResponseValidator{router: router, logger: logger}, nil
}

// Middleware wraps a handler to validate responses.
func (v *ResponseValidator) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Find the route
        route, pathParams, err := v.router.FindRoute(r)
        if err != nil {
            next.ServeHTTP(w, r)
            return
        }

        // Capture response
        rec := &responseRecorder{ResponseWriter: w, body: &bytes.Buffer{}}
        next.ServeHTTP(rec, r)

        // Validate response
        responseInput := &openapi3filter.ResponseValidationInput{
            RequestValidationInput: &openapi3filter.RequestValidationInput{
                Request:    r,
                PathParams: pathParams,
                Route:      route,
            },
            Status: rec.status,
            Header: rec.Header(),
            Body:   io.NopCloser(bytes.NewReader(rec.body.Bytes())),
        }

        if err := openapi3filter.ValidateResponse(r.Context(), responseInput); err != nil {
            v.logger("Response validation failed",
                "path", r.URL.Path,
                "method", r.Method,
                "status", rec.status,
                "error", err,
            )
        }
    })
}

type responseRecorder struct {
    http.ResponseWriter
    status int
    body   *bytes.Buffer
}

func (r *responseRecorder) WriteHeader(code int) {
    r.status = code
    r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
    r.body.Write(b)
    return r.ResponseWriter.Write(b)
}
```

### Phase 4: Update Router Integration

**File: `internal/http/router.go`**

```go
package http

import (
    "log/slog"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"

    "github.com/rezkam/mono/internal/http/handler"
    mw "github.com/rezkam/mono/internal/http/middleware"
    "github.com/rezkam/mono/internal/http/openapi"
)

// RouterConfig controls router behavior.
type RouterConfig struct {
    // ValidateResponses enables response validation (dev/test only).
    ValidateResponses bool
}

// NewRouter creates and configures the Chi router with all middleware and routes.
func NewRouter(server *handler.Server, authMiddleware *mw.Auth, config RouterConfig) *chi.Mux {
    r := chi.NewRouter()

    // Global middleware
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)

    // Health check endpoint (no auth/validation required)
    r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
            slog.ErrorContext(r.Context(), "Failed to write health check response", "error", err)
        }
    })

    // Load embedded OpenAPI spec
    spec, err := openapi.GetSwagger()
    if err != nil {
        panic("failed to load OpenAPI spec: " + err.Error())
    }

    // Create validation middleware
    validatorMw := mw.NewValidator(spec, mw.ValidationConfig{
        ValidateResponses: false, // Request validation only in middleware
        MultiError:        true,  // Return all validation errors
    })

    // Optional: Response validation for dev/test
    var responseValidator *mw.ResponseValidator
    if config.ValidateResponses {
        responseValidator, err = mw.NewResponseValidator(spec, func(msg string, args ...any) {
            slog.Warn(msg, args...)
        })
        if err != nil {
            panic("failed to create response validator: " + err.Error())
        }
    }

    // API routes
    r.Route("/api", func(r chi.Router) {
        // Order matters: validate request -> authenticate -> handle
        r.Use(validatorMw)           // 1. Validate request against spec
        r.Use(authMiddleware.Validate) // 2. Authenticate

        // Optional response validation
        if responseValidator != nil {
            r.Use(responseValidator.Middleware)
        }

        // Mount OpenAPI-generated routes
        openapi.HandlerFromMux(server, r)
    })

    return r
}
```

### Phase 5: Simplify Handlers (Remove Redundant Validation)

**File: `internal/http/handler/item.go`**

Before:
```go
func (s *Server) CreateItem(w http.ResponseWriter, r *http.Request, listID types.UUID) {
    var req openapi.CreateItemRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        response.BadRequest(w, "invalid JSON")
        return
    }

    // Manual priority validation - REMOVE (OpenAPI validates enum)
    if req.Priority != nil {
        priority, err := domain.NewTaskPriority(string(*req.Priority))
        if err != nil {
            response.FromDomainError(w, r, err)
            return
        }
        item.Priority = &priority
    }
    // ...
}
```

After:
```go
func (s *Server) CreateItem(w http.ResponseWriter, r *http.Request, listID types.UUID) {
    // Request already validated by OpenAPI middleware:
    // - title: required, 1-255 chars
    // - priority: valid enum value
    // - format: uuid for list_id
    // - format: date-time for due_time

    var req openapi.CreateItemRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        // Should not happen - OpenAPI already validated JSON
        response.BadRequest(w, "invalid JSON")
        return
    }

    // Build domain item - no validation needed for OpenAPI-validated fields
    item := &domain.TodoItem{
        Title:    req.Title,
        ListID:   listID.String(),
        Tags:     derefStringSlice(req.Tags),
        Timezone: req.Timezone,
        DueTime:  req.DueTime,
    }

    // Priority: direct assignment (enum validated by OpenAPI)
    if req.Priority != nil {
        priority := domain.TaskPriority(*req.Priority)
        item.Priority = &priority
    }

    // Duration: still needs domain parsing (complex ISO 8601)
    if req.EstimatedDuration != nil {
        duration, err := domain.ParseDuration(*req.EstimatedDuration)
        if err != nil {
            response.FromDomainError(w, r, err)
            return
        }
        item.EstimatedDuration = &duration
    }

    // Call service layer
    createdItem, err := s.todoService.CreateItem(r.Context(), listID.String(), item)
    if err != nil {
        response.FromDomainError(w, r, err)
        return
    }

    response.Created(w, openapi.CreateItemResponse{
        Item: ptr.To(MapItemToDTO(createdItem)),
    })
}
```

### Phase 6: Simplify Domain Value Objects

Since OpenAPI now validates enums and basic constraints, domain value objects become simpler:

**File: `internal/domain/value_objects.go`**

```go
// NewTaskStatus is kept for internal domain use and parsing from database.
// HTTP layer uses OpenAPI enum validation.
func NewTaskStatus(s string) (TaskStatus, error) {
    status := TaskStatus(strings.ToLower(s))
    switch status {
    case TaskStatusTodo, TaskStatusInProgress, TaskStatusBlocked,
        TaskStatusDone, TaskStatusArchived, TaskStatusCancelled:
        return status, nil
    default:
        return "", fmt.Errorf("%w: %s", ErrInvalidTaskStatus, s)
    }
}

// TaskStatusFromOpenAPI converts OpenAPI enum to domain type.
// No validation needed - OpenAPI already validated.
func TaskStatusFromOpenAPI(s openapi.ItemStatus) TaskStatus {
    return TaskStatus(s)
}

// TaskPriorityFromOpenAPI converts OpenAPI enum to domain type.
func TaskPriorityFromOpenAPI(p openapi.ItemPriority) TaskPriority {
    return TaskPriority(p)
}
```

### Phase 7: Update Error Response Mapping

**File: `internal/http/response/error.go`**

Add OpenAPI validation error types:

```go
// FromDomainError maps domain errors to HTTP responses.
func FromDomainError(w http.ResponseWriter, r *http.Request, err error) {
    switch {
    // Domain validation errors (400) - may still be needed for business rules
    case errors.Is(err, domain.ErrInvalidEtagFormat):
        ValidationError(w, "etag", "must be a numeric string")
    case errors.Is(err, domain.ErrInvalidDurationFormat):
        ValidationError(w, "estimated_duration", "invalid ISO 8601 duration")
    case errors.Is(err, domain.ErrDurationEmpty):
        ValidationError(w, "estimated_duration", "duration cannot be empty")

    // Note: Title, Status, Priority validation now handled by OpenAPI middleware
    // These cases can be removed after migration

    // Not found errors (404)
    case errors.Is(err, domain.ErrListNotFound):
        NotFound(w, "list")
    // ... rest unchanged
    }
}
```

### Phase 8: Update Server Initialization

**File: `cmd/server/main.go`**

```go
func initializeHTTPServer(ctx context.Context) (*HTTPServer, func(), error) {
    // ... existing code ...

    // 6. Initialize HTTP handler and middleware
    handler := httpHandler.NewServer(todoService)
    authMiddleware := httpMiddleware.NewAuth(authenticator)

    // 7. Create router with validation config
    routerConfig := httpRouter.RouterConfig{
        ValidateResponses: provideValidateResponses(), // From env
    }
    router := httpRouter.NewRouter(handler, authMiddleware, routerConfig)

    // ... rest unchanged
}

// provideValidateResponses reads MONO_VALIDATE_RESPONSES from environment.
func provideValidateResponses() bool {
    enabled, exists := config.GetEnv[bool]("MONO_VALIDATE_RESPONSES")
    if !exists {
        return false // Disabled by default in production
    }
    return enabled
}
```

---

## Files to Modify

| File | Changes |
|------|---------|
| `api/openapi/mono.yaml` | Add missing constraints, patterns, enums |
| `go.mod` | Add `github.com/oapi-codegen/nethttp-middleware` |
| `internal/http/middleware/validation.go` | **NEW** - Request validation middleware |
| `internal/http/middleware/response_validation.go` | **NEW** - Response validation (dev/test) |
| `internal/http/router.go` | Add validation middleware to chain |
| `internal/http/handler/item.go` | Remove redundant validation |
| `internal/http/handler/list.go` | Remove redundant validation |
| `internal/http/handler/recurring.go` | Remove redundant validation |
| `internal/http/handler/mapper.go` | Add OpenAPI->Domain type converters |
| `internal/domain/value_objects.go` | Add FromOpenAPI converters |
| `internal/http/response/error.go` | Remove migrated validation errors |
| `cmd/server/main.go` | Add router config for response validation |

---

## Testing Strategy

### 1. Unit Tests for Validation Middleware

```go
func TestValidationMiddleware_RejectsInvalidTitle(t *testing.T) {
    spec, _ := openapi.GetSwagger()
    mw := middleware.NewValidator(spec, middleware.ValidationConfig{})

    req := httptest.NewRequest("POST", "/api/v1/lists",
        strings.NewReader(`{"title": ""}`))
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder()

    handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        t.Error("Handler should not be called for invalid request")
    }))
    handler.ServeHTTP(rec, req)

    assert.Equal(t, 400, rec.Code)
    assert.Contains(t, rec.Body.String(), "VALIDATION_ERROR")
}
```

### 2. Integration Tests

Existing tests in `tests/integration/http/` should continue to pass.

### 3. Response Validation Tests (Dev Mode)

```go
func TestResponseValidation_CatchesInvalidResponse(t *testing.T) {
    os.Setenv("MONO_VALIDATE_RESPONSES", "true")
    defer os.Unsetenv("MONO_VALIDATE_RESPONSES")
    // Make request that would return invalid response
    // Assert warning is logged
}
```

---

## Migration Path

1. **Phase 1**: Add OpenAPI constraints (spec changes) - no runtime impact
2. **Phase 2-4**: Add middleware (off by default) - test in staging
3. **Phase 5**: Enable in production, monitor for issues
4. **Phase 6-7**: Simplify handlers (remove redundant code) - clean up
5. **Phase 8**: Enable response validation in dev/test environments

---

## Rollback Plan

1. Set `MONO_OPENAPI_VALIDATION=false` env var
2. Middleware checks this and becomes pass-through
3. Handlers continue to work with domain validation as backup

---

## Benefits After Implementation

1. **Single Source of Truth**: OpenAPI spec defines contract, validated automatically
2. **Thinner Handlers**: Less boilerplate validation code
3. **Better Error Messages**: Field-level validation from spec
4. **Spec Drift Detection**: Response validation catches mismatches
5. **Cleaner Architecture**: Each layer has single responsibility
6. **Documentation = Code**: Spec constraints are enforced

---

## Validation Responsibility Summary

| Layer | Validates | Examples |
|-------|-----------|----------|
| **OpenAPI Middleware** | Contract | types, formats, enums, ranges, required |
| **Domain** | Business Rules | timezone validity, duration parsing, etag semantics |
| **Application** | Use Case Logic | default excluded statuses, pagination limits |
| **Infrastructure** | Constraints | foreign keys, unique constraints |
