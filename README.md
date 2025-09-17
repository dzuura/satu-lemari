# SatuLemari Backend API

Platform donasi dan rental pakaian yang menghubungkan mitra (pemilik pakaian) dengan pengguna yang membutuhkan pakaian untuk donasi atau sewa.

## 🚀 Fitur Utama

- **Authentication & Authorization**: Firebase Auth dengan JWT token
- **Donasi & Rental System**: Sistem manajemen pakaian untuk donasi dan sewa
- **Order Management**: Sistem manajemen pesanan dengan tracking status dan notifikasi
- **AI Integration**: Smart listing assistant, intent matching, dan personalized recommendations (Gemini AI)
- **Intelligent Chatbot**: AI-powered chatbot untuk bantuan donasi, sewa, dan edukasi fashion berkelanjutan
- **File Storage**: Supabase Storage untuk upload gambar
- **Real-time Notifications**: Notifikasi in-app, FCM push notifications, dan email dengan template system
- **Geolocation Services**: Pencarian berdasarkan lokasi (latitude/longitude)
- **Quota System**: Weekly donation quota dengan automatic reset scheduler
- **Caching**: Redis caching untuk performa optimal
- **Rate Limiting**: Pembatasan request per endpoint
- **Structured Logging**: Logging terstruktur dengan berbagai level

## 🏗️ Arsitektur

Proyek ini menggunakan arsitektur **Modular Monolith** dengan domain separation:

```
SatuLemari/
├── cmd/
│   └── main.go                 # Entry point aplikasi
├── domain/
│   ├── ai/                     # AI services & recommendation
│   ├── auth/                   # Authentication & Authorization
│   ├── cache/                  # Redis caching
│   ├── category/               # Category management
│   ├── chat/                   # Intelligent chatbot system
│   ├── common/                 # Shared utilities
│   ├── config/                 # Configuration management
│   ├── database/               # Database connection
│   ├── error/                  # Error handling
│   ├── item/                   # Item management
│   ├── logging/                # Structured logging
│   ├── middleware/             # HTTP middleware
│   ├── models/                 # Data models
│   ├── notification/           # Notification services
│   ├── orders/                 # Order processing & management
│   ├── queue/                  # Queue system
│   ├── repository/             # Data access layer
│   ├── requests/               # Request management
│   ├── security/               # Security utilities
│   ├── storage/                # File storage
│   └── user/                   # User management
├── migrations/
│   └── schema.sql              # Database schema
├── scripts/                    # Token generation utility
├── app.yaml                    # App Engine deployment config
├── go.mod                      # Go module dependencies
├── go.sum                      # Go module checksums
├── DEPLOYMENT.md               # Deployment guide
└── README.md                   # Project documentation
```

## 🛠️ Teknologi yang Digunakan

- **Language**: Go 1.24+
- **Framework**: Gorilla Mux (HTTP router)
- **Database**: PostgreSQL (via Supabase REST API)
- **Cache**: Redis
- **Authentication**: Firebase Auth + JWT
- **File Storage**: Supabase Storage
- **AI Services**: Google Gemini AI
- **Notifications**: In-app notifications, FCM push notifications, Email (SMTP)
- **Logging**: Structured JSON logging

## 📋 Prerequisites

- Go 1.24 atau lebih baru
- PostgreSQL database (Supabase)
- Redis server
- Firebase project
- Google Gemini API key

## 🔧 Setup & Installation

### 1. Clone Repository

```bash
git clone https://github.com/dzuura/satu-lemari.git
cd satu-lemari
```

### 2. Install Dependencies

```bash
go mod download
```

### 3. Environment Configuration

Copy file environment example:

```bash
cp env.example .env
```

Edit file `.env` dan isi dengan konfigurasi yang sesuai.

### 4. Database Setup

Jalankan skema database:

```bash
# Connect to your PostgreSQL database and run:
psql -h your_host -U your_username -d your_database -f migrations/schema.sql
```

### 5. Run Application

```bash
go run cmd/main.go
```

Server akan berjalan di `http://localhost:8080`

## 🔑 Cara Mendapatkan API Keys

### Firebase Setup

1. Buat project di [Firebase Console](https://console.firebase.google.com/)
2. Enable Authentication dan Storage
3. Generate service account key di Project Settings > Service Accounts
4. Copy Project ID, Private Key, dan Client Email

### Supabase Setup

1. Buat project di [Supabase](https://supabase.com/)
2. Dapatkan URL dan API keys dari Settings > API
3. Buat storage bucket untuk upload file
4. Set RLS policies untuk storage bucket

### Google Gemini Setup

1. Dapatkan API key dari [Google AI Studio](https://makersuite.google.com/app/apikey)
2. Enable Gemini API di Google Cloud Console

## 📚 API Documentation

### Base URL

```
http://localhost:8080/api/v1
```

### Authentication

Semua endpoint yang memerlukan authentication menggunakan Bearer token dari Firebase.

```bash
Authorization: Bearer <firebase_id_token>
```

### Endpoints

#### Authentication

- `POST /auth/verify` - Verify Firebase/Google token
- `POST /auth/refresh` - Refresh access token (protected)
- `POST /auth/logout` - Logout user (protected)

#### Users

- `GET /users/me` - Get current user profile (protected)
- `PUT /users/me` - Update user profile (protected)
- `DELETE /users/me` - Delete user account with data anonymization (protected)
- `GET /users/dashboard` - Get user dashboard (protected)
- `GET /users/{user_id}/profile` - Get public user profile (public)
- `GET /users/search` - Search users (public)

#### Items

- `GET /items` - Get all items with optional category filter (public)
- `GET /items/search` - Search items with category filter (public)
- `GET /items/{item_id}` - Get specific item (public)
- `POST /items` - Create new item (protected)
- `GET /my-items` - Get my items with category filter (protected)
- `PUT /items/{item_id}` - Update item (protected)
- `PATCH /items/{item_id}/status` - Update item status (protected)
- `DELETE /items/{item_id}` - Delete item (protected)

#### Orders

- `POST /orders` - Create new order (protected)
- `GET /orders` - List orders with filters (protected - user: own orders, admin: all orders)
- `GET /orders/my` - Get current user's orders as buyer (protected - USER ROLE ONLY)
- `GET /orders/partner` - Get orders for partner's items as seller (protected - PARTNER ROLE ONLY)
- `GET /orders/{order_id}` - Get specific order details (protected)
- `DELETE /orders/{order_id}` - Cancel order (protected)

#### Requests

- `POST /requests` - Create new donation or rental request (protected)
- `GET /requests/my` - Get current user's requests with filtering (protected)
- `GET /requests/partner` - Get requests for partner's items (protected)
- `GET /requests/{request_id}` - Get specific request details (protected)
- `PUT /requests/{request_id}` - Update request status (protected)
- `DELETE /requests/{request_id}` - Delete request (protected)

#### Categories

- `GET /categories` - Get all categories (public)
- `GET /categories/{id}` - Get specific category (public)
- `POST /categories` - Create category (admin only)
- `PUT /categories/{id}` - Update category (admin only)
- `DELETE /categories/{id}` - Delete category (admin only)

#### AI Services

- `POST /ai/smart-listing` - Smart listing assistant with AI-powered item analysis
- `POST /ai/smart-listing/batch` - Batch smart listing for multiple items
- `POST /ai/intent` - Parse user intent from natural language queries
- `GET /ai/suggestions` - Get intelligent search suggestions
- `GET /ai/status` - Get AI service status and health check
- `POST /ai/analyze` - Legacy item analysis (backward compatibility)
- `POST /ai/recommendations` - Generate general recommendations
- `GET /ai/recommendations/similar/{id}` - Get similar item recommendations
- `GET /ai/recommendations/trending` - Get trending item recommendations
- `GET /ai/recommendations/personalized` - Get personalized recommendations based on user behavior (protected)

#### Chatbot

- `POST /chat/start` - Start new chat session with language preference
- `POST /chat/send` - Send message to chatbot with context
- `GET /chat/history/{sessionId}` - Get chat history for specific session
- `GET /chat/sessions` - Get user's chat sessions with pagination
- `DELETE /chat/sessions/{sessionId}` - Delete chat session and all messages
- `DELETE /chat/sessions/{sessionId}/messages` - Delete specific messages from session
- `DELETE /chat/sessions/{sessionId}/messages/all` - Delete all messages in session
- `DELETE /chat/history/all` - Delete all user chat history
- `GET /chat/suggestions` - Get chat suggestions and categories (public)
- `GET /chat/health` - Chatbot service health check

#### Notifications

**Protected Notification Endpoints (Authentication Required):**

- `GET /notifications` - Get user notifications with pagination and filters
- `GET /notifications/stats` - Get notification statistics
- `PUT /notifications/read-all` - Mark all notifications as read
- `PUT /notifications/mark-read` - Mark multiple notifications as read (bulk)
- `DELETE /notifications/delete-bulk` - Delete multiple notifications (bulk)
- `PUT /notifications/{id}/read` - Mark specific notification as read
- `DELETE /notifications/{id}` - Delete specific notification

**FCM Token Management (Authentication Required):**

- `POST /notifications/fcm-token` - Register FCM token
- `PUT /notifications/fcm-token` - Update FCM token
- `DELETE /notifications/fcm-token` - Remove FCM token
- `GET /notifications/fcm-tokens` - Get user's FCM tokens

**Admin/Testing Endpoints:**

- `POST /notifications/send-template` - Send template notification
- `POST /notifications/send-bulk` - Send bulk notifications
- `POST /notifications/fcm/test` - Send test FCM notification
- `POST /notifications/fcm/topic` - Send topic notification
- `POST /notifications/fcm/subscribe` - Subscribe to topic
- `POST /notifications/fcm/unsubscribe` - Unsubscribe from topic
- `GET /notifications/health` - Notification service health check

## 🤝 Contributing

1. Fork repository
2. Create feature branch (`git checkout -b feature/amazing-feature`)
3. Commit changes (`git commit -m 'Add amazing feature'`)
4. Push to branch (`git push origin feature/amazing-feature`)
5. Open Pull Request

## 📝 Commit Messages

Gunakan format conventional commits:

```
feat: add user authentication system
fix: resolve database connection issue
docs: update API documentation
refactor: improve error handling
test: add unit tests for user service
chore: update dependencies
```

## 📄 License

Distributed under the MIT License. See `LICENSE` for more information.

## 📞 Support

- Email: satulemariapp@gmail.com
- Documentation: [API Docs](https://documenter.getpostman.com/view/39730752/2sB34foMin)
- Issues: [GitHub Issues](https://github.com/dzuura/satu-lemari/issues)

**SatuLemari** - Platform donasi dan rental pakaian yang menghubungkan kebaikan dengan kebutuhan. 🌱👕
