# api_template

A **Go REST API** with MySQL database support, featuring **dynamic API endpoints**, **role-based security**, **auto migrations**, and **Swagger documentation**. The project is containerized using **Docker** and orchestrated with **Docker Compose** for a seamless development and deployment workflow.

---

## Features

* **Dynamic endpoints** via reflection
* **Role-based security** (JWT + middleware)
* **Auto migrations** with GORM
* **Swagger UI** ready to go
* **Containerized** (Docker + Compose)
* **User management** including getting user info and changing passwords
* **Configurable Swagger** visibility via `.env`
* **Paginated API** responses
* **Advanced filtering** and **sorting** support (`limit`, `page`, `sort`, operators)
* **Bulk inserts** using JSON arrays `[{}]`
* **Custom permissions per model type**
* **Service actions** with role/username gates (no CRUD payloads required)
* **GUI-ready API responses** filtered by user access level
* **Automatic CreatedAt, EditedAt, UpdatedBy, CreatedBy** fields on all objects (not editable)
* **User API key generation** (no expiration, validated on server)

---

## Quickstart

1. **Create from Template**
   On GitHub, click **Use this template** → **Create a new repository**.

2. **Clone your new repo**

   ```bash
   git clone https://github.com/yourusername/yourproject.git
   cd yourproject
   ```

3. **Configure environment**

   ```bash
   cp .env.example .env
   ```

   Edit `.env` and supply your values:

   ```dotenv
   # MySQL Settings
   MYSQL_ROOT_PASSWORD=example
   MYSQL_DATABASE=demo_db
   MYSQL_USER=demo_user
   MYSQL_PASSWORD=demo_pass

   # App Settings
   DB_PORT=3306
   JWT_SECRET=your_jwt_secret_key
   ADMIN_PASSWORD=admin

   # Allow users to access stats for GUI
   USER_GUI=true
   SWAGGER=true
   ```

4. **Update module path**

   ```bash
   sed -i 's|module github.com/r4ulcl/api_template|module github.com/yourusername/yourproject|' go.mod
   go mod tidy
   ```

5. **Build & run**

   ```bash
   docker-compose up --build
   ```

   * API:  `http://localhost:8080`
   * Swagger UI: `http://localhost:8080/swagger/index.html`

---

## Project Layout

```
api_template/
├── api/
│   ├── controllers/
│   │   ├── auth_controller.go     # Registration & login handlers
│   │   ├── base_controller.go     # Generic CRUD handlers
│   │   └── service_controller.go  # Custom service endpoints & helpers
│   ├── middlewares/
│   │   └── auth_middleware.go   # JWT & RBAC middleware
│   └── routes/
│       └── routes.go            # Dynamic route registration
├── database/
│   └── database.go              # GORM connection, retries, auto-migrate
├── utils/
│   ├── config.go                # .env loader & DSN constructor
│   ├── auth.go                  # Password hashing & JWT utils
│   └── models/                  # GORM models & DTOs
│       ├── api.go               # LoginRequest, RegisterRequest, JWTResponse, ErrorResponse
│       ├── database.go          # Example1, Example2, ExampleRelational structs
│       ├── login.go             # User model & Role enum
│       └── permissions.go       # RolePermissions, ServiceDefinitions & ModelMap
├── docs/
│   ├── docs.go                  # Swagger annotations
│   ├── swagger.json             # Generated OpenAPI spec (JSON)
│   └── swagger.yaml             # Generated OpenAPI spec (YAML)
├── .env.example                 # Sample environment variables
├── Dockerfile                   # Container build instructions
├── docker-compose.yml           # Compose setup for DB + API
├── main.go                      # Application entry point
├── go.mod                       # Module path & dependencies
└── go.sum                       # Dependency checksums
```

---

## Adding Your Own Data Models

To add a new resource (for example a `Product`):

1. **Create the model** in `utils/models/product.go`

   ```go
   package models

   import "time"

   // Product represents an item for sale.
   type Product struct {
     ID          uint      `gorm:"primaryKey" json:"id"`
     Name        string    `gorm:"size:100;not null" json:"name"`
     Description string    `gorm:"type:text" json:"description,omitempty"`
     Price       float64   `gorm:"not null" json:"price"`
     CreatedAt   time.Time `json:"created_at"`
     UpdatedAt   time.Time `json:"updated_at"`
   }
   ```

2. **Register resource and permissions**
   In `utils/models/permissions.go`, add `"product"` to each appropriate permissions slice and to `ModelMap`:

   ```diff
   // permissions.go

   var UserGetResources = []string{"example1", "example2", "exampleRelational", "product"}
   // … likewise for AdminGetResources, AdminPostResources, etc.

   var ModelMap = map[string]interface{}{
     "user":              &User{},
     "example1":          &Example1{},
     "example2":          &Example2{},
     "exampleRelational": &ExampleRelational{},
   + "product":           &Product{},
   }
   ```

3. **Rebuild and restart**

   ```bash
   docker-compose up --build
   ```

4. **Test the endpoints**

   ```bash
   # List products
   curl -H "Authorization: Bearer <token>" http://localhost:8080/product

   # Create product (admin only)
   curl -X POST http://localhost:8080/product \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer <admin-token>" \
        -d '{"name":"Gadget","price":19.99}'
   ```

---

## Adding Service Actions

Service actions let you expose ad-hoc endpoints (restart a workflow, fetch a status, etc.) without creating a database-backed model. They rely on the new `ServiceDefinitions` map and the service controller helpers.

1. **Add or update a handler** in `api/controllers/service_controller.go`:

   ```go
   func (c *Controller) sendReminderService(w http.ResponseWriter, r *http.Request) {
     requester, _ := r.Context().Value(middlewares.ContextUserID).(string)
     if strings.TrimSpace(requester) == "" {
       respondJSON(w, http.StatusUnauthorized, models.ErrorResponse{Error: "Unauthorized"})
       return
     }

     respondJSON(w, http.StatusOK, map[string]interface{}{
       "service":     "sendReminder",
       "requestedBy": requester,
       "message":     "Reminder queued",
     })
   }
   ```

   Make sure the required imports (for example, `strings`) are present at the top of the file.

   Register the handler in the local factory map (keep the existing entries and append yours):

   ```go
   var serviceHandlers = map[string]serviceHandlerFactory{
     "currentExam":      func(c *Controller) http.Handler { return http.HandlerFunc(c.currentExamService) },
     "restartExam":      func(c *Controller) http.Handler { return http.HandlerFunc(c.restartExamService) },
     "exampleFunction":  func(c *Controller) http.Handler { return http.HandlerFunc(c.exampleFunctionService) },
     "exampleFunction2": func(c *Controller) http.Handler { return http.HandlerFunc(c.exampleFunction2Service) },
     "sendReminder":     func(c *Controller) http.Handler { return http.HandlerFunc(c.sendReminderService) },
   }
   ```

   Handlers receive the same request context as normal routes, so you can read JWT claims or call into your persistence layer.

2. **Configure permissions** in `utils/models/permissions.go` by appending to `ServiceDefinitions`:

   ```go
   var ServiceDefinitions = []ServiceDefinition{
     {
       Name:    "sendReminder",
       Method:  "POST",
       Path:    "/services/reminders/send",
       Handler: "sendReminder",
       Access: ServiceAccess{
         FullRoles: []string{"admin", "reviewer"},
         OwnRoles:  []string{"user"},
         Usernames: []string{"automation-bot"},
       },
     },
   }
   ```

   * `FullRoles` bypass own-only checks and can act on any target.
   * `OwnRoles` are automatically flagged as own-only; handlers can check `middlewares.IsOwnOnly(r.Context())` to restrict scope.
   * `Usernames` is a shortcut allowlist that applies regardless of role (useful for system accounts).
   * Leave lists empty to allow all roles with the authenticated token (`FullRoles: nil` gives access to every role).

3. **Call the endpoint** once the API is running:

   ```bash
   curl -X POST \
     -H "Authorization: Bearer <token>" \
     http://localhost:8080/services/reminders/send
   ```

   Service routes are mounted under the authenticated router, so they require JWT/API-key authentication just like other protected endpoints. Run `gofmt` after editing Go files to keep formatting tidy.

> Tip: if you need audit logging or side effects, the service handlers have full access to `c.BC.DB` via the controller.

---

## Roadmap

* [x] User manage section, get info, change password, etc. Rol user too
  * [x] Change password
  * [x] Get user info
* [x] Bool in .env to swagger
* [x] Paginate API
* [x] Filters in GET
* [x] Allow insert array of JSON `[{}]`
* [x] Allow sort `limit=20&page=3&sort=created_at:desc`
* [x] Allow advanced filters
* [x] Own permission on each type
* [x] Default info for GUI send only info with read access for that user
* [x] CreatedAt, EditedAt, UpdatedBy, CreateBy in all objects, not editable in API
* [ ] Add user groups with shared permissions and visibility scopes
* [ ] Implement email verification on registration and password reset
* [x] Option user to generate API key no expiracy but validated in server
* [ ] Security check everything
* [x] Integrate audit logging for all CRUD operations
* [ ] Add rate limiting and IP allowlist / denylist

---

## License

Distributed under the **MIT License**. See `LICENSE` for details.

---

## Maintainer

**r4ulcl** [github.com/r4ulcl](https://github.com/r4ulcl)

Have suggestions or found a bug? Please open an issue or submit a pull request!
