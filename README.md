# **api_template**

A Golang API with MySQL database support, featuring **dynamic API endpoints** and **Swagger documentation**. The project is containerized using **Docker** and managed via **Docker Compose**, ensuring a streamlined development and deployment process.

---

## **Features** 🌟

✅ **Modular Code Structure** – Organized into controllers, models, middlewares, and utilities.  
✅ **JWT Authentication** – Secure API with authentication and role-based access control.  
✅ **Dynamic API** – Auto-generated CRUD endpoints for structured data models.  
✅ **Swagger Documentation** – Auto-generated API docs for easy usage.  
✅ **Dockerized Deployment** – Seamless setup with **Docker Compose**.  
✅ **Persistent MySQL Database** – Ensures data remains intact across restarts.  

---

## **Using as a Template** 🏗️

This project is designed to be a **boilerplate template** for Golang-based REST APIs. You can use it as a foundation for building your own API without starting from scratch.

### **How to Use it as a Template**
1️⃣ **Click "Use this template"** on GitHub.  
2️⃣ **Clone your new repository**:  
   ```sh
   git clone https://github.com/yourusername/yourproject.git
   cd yourproject
   ```
3️⃣ **Update Module Name** in `go.mod`:  
   ```sh
   module github.com/yourusername/yourproject
   ```
   Then, run:
   ```sh
   go mod tidy
   ```
4️⃣ **Modify Models & Controllers**  
   - Add your own **data models** inside `models/`.
   - Create **custom endpoints** in `controllers/`.
   - Adjust **database migrations** in `database/`.

5️⃣ **Run Your API** 🚀  
   ```sh
   docker-compose up --build
   ```

🎉 **Your Golang API is now running!** Modify and expand it as needed.

--- 

## **Getting Started** 🏁

### **Prerequisites** 🛠️

- **[Docker](https://www.docker.com/get-started)**
- **[Docker Compose](https://docs.docker.com/compose/install/)**
- **[Go 1.22+](https://go.dev/doc/install) (For local development, not needed with Docker)**

---

## **Installation & Setup** ⚙️

### **1. Clone the repository**
```sh
git clone https://github.com/r4ulcl/api_template.git
cd api_template
```

### **2. Start the application using Docker**
```sh
docker-compose up --build
```

> This command will:
> - Start a **MySQL database** container (`db`).
> - Build and launch the **Go API application** (`app`).
> - Expose the API on `http://localhost:8080`.

---

## **Project Structure** 📂

api_template/
├── api/                        # Contains all API-related logic
│   ├── controllers/            # Request handlers for API endpoints (business logic)
│   ├── middlewares/            # Authentication, authorization, and other middleware
│   └── routes/                 # Routing definitions that map endpoints to controllers
├── database/                   # Database connection and query logic
├── docs/                       # Swagger/OpenAPI files and other documentation
├── utils/                      # Utility functions (e.g., hashing, JWT creation)
│   └── models/                 # Data models and structs (e.g., User, Roles)
├── main.go                     # Application entry point: runs the server
├── Dockerfile                  # Instructions to containerize the application
├── docker-compose.yml          # Docker Compose config for multi-service setups
├── go.mod                      # Go module dependencies and module path
└── go.sum                      # Dependency checksums for reproducible builds


---

## **Environment Variables** ⚙️

The application requires some **environment variables** to be set. These are defined in `docker-compose.yml`.

| Variable      | Description                  | Default Value |
|--------------|-------------------------------|--------------|
| `DB_HOST`    | MySQL Database Host           | `db` |
| `DB_PORT`    | MySQL Port                    | `3306` |
| `DB_USER`    | MySQL Username                | `demo_user` |
| `DB_PASSWORD` | MySQL Password               | `demo_pass` |
| `DB_NAME`    | MySQL Database Name           | `demo_db` |
| `JWT_SECRET` | JWT Secret Key for Tokens     | `your_jwt_secret_key` |
| `ADMIN_PASSWORD` | Default Admin Password    | `SuperSecurePassword` |

> **⚠️ Important**: Modify these values in `docker-compose.yml` or set them manually before running the app.

---

## **API Documentation** 📖

Swagger UI is available at:

📌 **[http://localhost:8080/swagger/index.html](http://localhost:8080/swagger/index.html)**

This provides a detailed overview of all endpoints, parameters, and responses.

---

## **Usage** 🚀

### **1. Register a New User**
```sh
curl -X POST "http://localhost:8080/register" \
     -H "Content-Type: application/json" \
     -d '{"username": "testuser", "password": "password123", "role": "user"}'
```

### **2. Login to Get JWT Token**
```sh
curl -X POST "http://localhost:8080/login" \
     -H "Content-Type: application/json" \
     -d '{"username": "testuser", "password": "password123"}'
```
_Response:_
```json
{
  "token": "your.jwt.token"
}
```

### **3. Access Protected Routes**
Include the JWT token in the `Authorization` header:
```sh
curl -X GET "http://localhost:8080/xxxxxxx" \
     -H "Authorization: Bearer your.jwt.token"
```


## **License** 📜

🔓 **MIT License** – Feel free to use, modify, and distribute this project.

---

## **Contributors** 🤝

🚀 **Maintained by:** [r4ulcl](https://github.com/r4ulcl)

---

💡 **Have suggestions or found an issue?** Open a pull request or file an issue in the repository!
