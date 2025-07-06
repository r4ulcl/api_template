# api\_template

A **Go REST API** with MySQL database support, featuring **dynamic API endpoints**, **role-based security**, **auto migrations**, and **Swagger documentation**. The project is containerized using **Docker** and orchestrated with **Docker Compose** for a seamless development and deployment workflow.

---

## Features

* **Dynamic endpoints** via reflection
* **Role-based security** (JWT + middleware)
* **Auto migrations** with GORM
* **Swagger UI** ready to go
* **Containerized** (Docker + Compose)

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
│   │   ├── auth_controller.go   # Registration & login handlers
│   │   └── base_controller.go   # Generic CRUD handlers
│   ├── middlewares/
│   │   └── auth_middleware.go   # JWT & RBAC middleware
│   └── routes/
│       └── routes.go            # Dynamic route registration
├── database/
│   └── database.go          # GORM connection, retries, auto-migrate
├── utils/
│   ├── config.go            # .env loader & DSN constructor
│   ├── auth.go              # Password hashing & JWT utils
│   └── models/              # GORM models & DTOs
│       ├── api.go           # LoginRequest, RegisterRequest, JWTResponse, ErrorResponse
│       ├── database.go      # Example1, Example2, ExampleRelational structs
│       ├── login.go         # User model & Role enum
│       └── permissions.go   # RolePermissions & ModelMap
├── docs/
│   ├── docs.go              # Swagger annotations
│   ├── swagger.json         # Generated OpenAPI spec (JSON)
│   └── swagger.yaml         # Generated OpenAPI spec (YAML)
├── .env.example             # Sample environment variables
├── Dockerfile               # Container build instructions
├── docker-compose.yml       # Compose setup for DB + API
├── main.go                  # Application entry point
├── go.mod                   # Module path & dependencies
└── go.sum                   # Dependency checksums
```

---

## Adding Your Own Data Models

To add a new resource (e.g. a `Product`):

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
2. **Register resource & permissions**
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

4. **Rebuild & restart**

   ```bash
   docker-compose up --build
   ```

5. **Test the endpoints**

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

## Roadmap

- [ ] User manage section, get info, change password, etc. Rol user too
   - [ ] Change password
   - [ ] Get user info
- [ ] Bool in .env to swagger
- [x] Paginate API
- [x] Filters in GET
- [ ] Allow insert array of JSON `[{}]`
- [x] Allow sort `limit=20&page=3&sort=created_at:desc`
- [x] Allow advanced filters

---

## License

Distributed under the **MIT License**. See `LICENSE` for details.

---

## Maintainer

**r4ulcl** – [github.com/r4ulcl](https://github.com/r4ulcl)

Have suggestions or found a bug? Please open an issue or submit a pull request!
