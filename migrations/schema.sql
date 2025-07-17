-- SatuLemari Database Schema

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Enable RLS (Row Level Security)
ALTER DATABASE postgres SET "timezone" TO 'Asia/Jakarta';

-- Users table - using Firebase UID as primary key
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(128) PRIMARY KEY, -- Firebase UID (max 128 chars)
    email VARCHAR(255) UNIQUE NOT NULL,
    username VARCHAR(100) NOT NULL,
    full_name VARCHAR(255),
    role VARCHAR(20) CHECK (role IN ('user', 'partner', 'admin')) DEFAULT 'user',
    phone VARCHAR(20),
    address TEXT,
    city VARCHAR(100),
    latitude DECIMAL(10, 8),
    longitude DECIMAL(11, 8),
    photo TEXT,
    description TEXT,
    is_active BOOLEAN DEFAULT true,
    weekly_donation_quota INTEGER DEFAULT 3,
    weekly_donation_used INTEGER DEFAULT 0,
    quota_reset_date DATE DEFAULT CURRENT_DATE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Categories table
CREATE TABLE IF NOT EXISTS categories (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL,
    description TEXT,
    icon TEXT,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Items table (clothes)
CREATE TABLE IF NOT EXISTS items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    partner_id VARCHAR(128) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    size VARCHAR(10) NOT NULL,
    color VARCHAR(50),
    type VARCHAR(20) CHECK (type IN ('donation', 'rental')) NOT NULL,
    price DECIMAL(10, 2) DEFAULT 0, -- for rental items
    total_quantity INTEGER NOT NULL DEFAULT 1,
    available_quantity INTEGER NOT NULL DEFAULT 1,
    condition VARCHAR(20) CHECK (condition IN ('excellent', 'good', 'fair')) DEFAULT 'good',
    images TEXT[], -- array of image URLs
    status VARCHAR(20) CHECK (status IN ('active', 'inactive', 'out_of_stock')) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Requests table (for both donation and rental requests)
CREATE TABLE IF NOT EXISTS requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    item_id UUID NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    user_id VARCHAR(128) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    partner_id VARCHAR(128) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(20) CHECK (type IN ('donation', 'rental')) NOT NULL,
    quantity INTEGER NOT NULL DEFAULT 1,
    reason TEXT,
    contact_info VARCHAR(255),
    pickup_date DATE,
    return_date DATE, -- for rental only
    status VARCHAR(20) CHECK (status IN ('pending', 'approved', 'rejected', 'completed', 'returned')) DEFAULT 'pending',
    rejection_reason TEXT,
    queue_position INTEGER,
    priority_score DECIMAL(5, 2) DEFAULT 0,
    deleted_by_user BOOLEAN DEFAULT FALSE,
    deleted_by_partner BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Transactions table (for completed donations/rentals)
CREATE TABLE IF NOT EXISTS transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    request_id UUID NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
    item_id UUID NOT NULL REFERENCES items(id),
    user_id VARCHAR(128) NOT NULL REFERENCES users(id),
    partner_id VARCHAR(128) NOT NULL REFERENCES users(id),
    type VARCHAR(20) CHECK (type IN ('donation', 'rental')) NOT NULL,
    quantity INTEGER NOT NULL,
    amount DECIMAL(10, 2) DEFAULT 0, -- for rental
    status VARCHAR(20) CHECK (status IN ('active', 'completed', 'cancelled')) DEFAULT 'active',
    pickup_date TIMESTAMP WITH TIME ZONE,
    return_date TIMESTAMP WITH TIME ZONE, -- for rental
    actual_return_date TIMESTAMP WITH TIME ZONE, -- actual return for rental
    rating INTEGER CHECK (rating >= 1 AND rating <= 5),
    review TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Notifications table
CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id VARCHAR(128) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    message TEXT NOT NULL,
    type VARCHAR(50) NOT NULL, -- request_update, item_available, reminder, etc.
    related_id UUID, -- can reference request_id, item_id, etc.
    data JSONB, -- additional data
    is_read BOOLEAN DEFAULT false,
    is_sent BOOLEAN DEFAULT false,
    platform VARCHAR(20) CHECK (platform IN ('web', 'mobile', 'both')) DEFAULT 'both',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    read_at TIMESTAMP WITH TIME ZONE
);

-- FCM Tokens table for storing Firebase Cloud Messaging tokens
--
-- DESIGN DECISIONS:
-- 1. token is NOT unique because multiple users can share the same device
--    Example: Family members using the same phone will have the same FCM token
-- 2. Only user_id + platform combination is unique (one active token per user per platform)
--    Example: User A can have one Android token and one iOS token, but not two Android tokens
-- 3. FCM tokens are device-specific, not user-specific in Firebase
-- 4. When user updates token, old token is replaced (upsert behavior)
CREATE TABLE IF NOT EXISTS fcm_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id VARCHAR(128) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token TEXT NOT NULL,  -- Removed UNIQUE constraint - multiple users can have same token (shared device)
    platform VARCHAR(20) NOT NULL CHECK (platform IN ('android', 'ios', 'web')),
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_used_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT fcm_tokens_user_platform_unique UNIQUE(user_id, platform)  -- One active token per user per platform
);

-- Cache table (for Redis-like caching in database)
CREATE TABLE IF NOT EXISTS cache (
    key VARCHAR(255) PRIMARY KEY,
    value JSONB NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Queue table (for request processing)
CREATE TABLE IF NOT EXISTS queue_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    type VARCHAR(100) NOT NULL, -- process_request, send_notification, etc.
    payload JSONB NOT NULL,
    status VARCHAR(20) CHECK (status IN ('pending', 'processing', 'completed', 'failed')) DEFAULT 'pending',
    attempts INTEGER DEFAULT 0,
    max_attempts INTEGER DEFAULT 3,
    error_message TEXT,
    scheduled_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- User recommendations table (AI-based recommendations)
CREATE TABLE IF NOT EXISTS user_recommendations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id VARCHAR(128) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    item_id UUID NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    score DECIMAL(5, 2) NOT NULL DEFAULT 0,
    reason JSONB, -- explanation for recommendation
    is_shown BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(user_id, item_id)
);

-- Search queries table (for AI intent matching)
CREATE TABLE IF NOT EXISTS search_queries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id VARCHAR(128) REFERENCES users(id) ON DELETE CASCADE,
    query TEXT NOT NULL,
    intent JSONB, -- parsed intent from AI
    results JSONB, -- search results
    session_id VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create indexes for better performance
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
CREATE INDEX IF NOT EXISTS idx_items_partner_id ON items(partner_id);
CREATE INDEX IF NOT EXISTS idx_items_category_id ON items(category_id);
CREATE INDEX IF NOT EXISTS idx_items_type ON items(type);
CREATE INDEX IF NOT EXISTS idx_items_status ON items(status);
CREATE INDEX IF NOT EXISTS idx_requests_user_id ON requests(user_id);
CREATE INDEX IF NOT EXISTS idx_requests_partner_id ON requests(partner_id);
CREATE INDEX IF NOT EXISTS idx_requests_item_id ON requests(item_id);
CREATE INDEX IF NOT EXISTS idx_requests_status ON requests(status);
CREATE INDEX IF NOT EXISTS idx_requests_type ON requests(type);
CREATE INDEX IF NOT EXISTS idx_requests_deleted_by_user ON requests(deleted_by_user);
CREATE INDEX IF NOT EXISTS idx_requests_deleted_by_partner ON requests(deleted_by_partner);
CREATE INDEX IF NOT EXISTS idx_requests_user_not_deleted ON requests(user_id, deleted_by_user);
CREATE INDEX IF NOT EXISTS idx_requests_partner_not_deleted ON requests(partner_id, deleted_by_partner);
CREATE INDEX IF NOT EXISTS idx_transactions_user_id ON transactions(user_id);
CREATE INDEX IF NOT EXISTS idx_transactions_partner_id ON transactions(partner_id);
CREATE INDEX IF NOT EXISTS idx_notifications_user_id ON notifications(user_id);
CREATE INDEX IF NOT EXISTS idx_notifications_is_read ON notifications(is_read);
-- FCM Tokens indexes
CREATE INDEX IF NOT EXISTS idx_fcm_tokens_user_id ON fcm_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_fcm_tokens_token ON fcm_tokens(token);  -- Non-unique: multiple users can share same token
CREATE INDEX IF NOT EXISTS idx_fcm_tokens_is_active ON fcm_tokens(is_active);
CREATE INDEX IF NOT EXISTS idx_fcm_tokens_platform ON fcm_tokens(platform);
CREATE INDEX IF NOT EXISTS idx_fcm_tokens_last_used ON fcm_tokens(last_used_at DESC);
CREATE INDEX IF NOT EXISTS idx_fcm_tokens_user_platform ON fcm_tokens(user_id, platform);  -- For unique constraint
CREATE INDEX IF NOT EXISTS idx_cache_expires_at ON cache(expires_at);
CREATE INDEX IF NOT EXISTS idx_queue_jobs_status ON queue_jobs(status);
CREATE INDEX IF NOT EXISTS idx_queue_jobs_scheduled_at ON queue_jobs(scheduled_at);

-- Insert default categories
INSERT INTO categories (name, description, icon) VALUES
('Pakaian Formal', 'Pakaian profesional dan formal', 'formal'),
('Pakaian Kasual', 'Pakaian kasual sehari-hari', 'kasual'),
('Pakaian Olahraga', 'Pakaian atletik dan olahraga', 'olahraga'),
('Pakaian Tradisional', 'Pakaian tradisional dan budaya', 'tradisional'),
('Aksesoris', 'Aksesoris pakaian', 'aksesoris'),
('Pakaian Luar', 'Jaket, mantel, dan pakaian luar', 'pakaian luar'),
('Alas Kaki', 'Sepatu dan sandal', 'alas kaki'),
('Celana', 'Aneka celana', 'celana');

-- Functions for automatic updates
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create triggers for updated_at
CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_categories_updated_at BEFORE UPDATE ON categories FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_items_updated_at BEFORE UPDATE ON items FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_requests_updated_at BEFORE UPDATE ON requests FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_transactions_updated_at BEFORE UPDATE ON transactions FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Function to reset weekly donation quota
CREATE OR REPLACE FUNCTION reset_weekly_donation_quota()
RETURNS void AS $$
BEGIN
    UPDATE users 
    SET weekly_donation_used = 0, quota_reset_date = CURRENT_DATE
    WHERE quota_reset_date <= CURRENT_DATE - INTERVAL '7 days'
    AND role = 'user';
END;
$$ LANGUAGE plpgsql;

-- Function to update item availability
CREATE OR REPLACE FUNCTION update_item_availability()
RETURNS TRIGGER AS $$
BEGIN
    -- Update item status based on available quantity
    IF NEW.available_quantity <= 0 THEN
        UPDATE items SET status = 'out_of_stock' WHERE id = NEW.id;
    ELSIF OLD.status = 'out_of_stock' AND NEW.available_quantity > 0 THEN
        UPDATE items SET status = 'active' WHERE id = NEW.id;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_item_availability 
    AFTER UPDATE OF available_quantity ON items 
    FOR EACH ROW EXECUTE FUNCTION update_item_availability();

-- Enable RLS on all tables
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE items ENABLE ROW LEVEL SECURITY;
ALTER TABLE requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE transactions ENABLE ROW LEVEL SECURITY;
ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE fcm_tokens ENABLE ROW LEVEL SECURITY;

-- RLS Policies - with proper type casting for Firebase UID
-- Note: auth.uid() returns UUID, but we store Firebase UID as VARCHAR(128)
-- We need to cast auth.uid() to VARCHAR for comparison

-- Users policies
CREATE POLICY "Users can view own data" ON users FOR SELECT USING (auth.uid()::VARCHAR = id);
CREATE POLICY "Users can update own data" ON users FOR UPDATE USING (auth.uid()::VARCHAR = id);
CREATE POLICY "Allow user creation during auth" ON users FOR INSERT WITH CHECK (true);

-- Items policies
CREATE POLICY "Anyone can view active items" ON items FOR SELECT USING (status = 'active');
CREATE POLICY "Partners can manage own items" ON items FOR ALL USING (auth.uid()::VARCHAR = partner_id);

-- Requests policies
CREATE POLICY "Users can view own requests" ON requests FOR SELECT USING (auth.uid()::VARCHAR = user_id OR auth.uid()::VARCHAR = partner_id);
CREATE POLICY "Users can create requests" ON requests FOR INSERT WITH CHECK (auth.uid()::VARCHAR = user_id);
CREATE POLICY "Partners can manage requests for their items" ON requests FOR ALL USING (auth.uid()::VARCHAR = partner_id);

-- Notifications policies
CREATE POLICY "Users can view own notifications" ON notifications FOR SELECT USING (auth.uid()::VARCHAR = user_id);
CREATE POLICY "Users can update own notifications" ON notifications FOR UPDATE USING (auth.uid()::VARCHAR = user_id);
CREATE POLICY "Users can delete own notifications" ON notifications FOR DELETE USING (auth.uid()::VARCHAR = user_id);
CREATE POLICY "System can insert notifications" ON notifications FOR INSERT WITH CHECK (true); -- Allow system to insert notifications for any user

-- FCM Tokens policies
CREATE POLICY "Users can view own FCM tokens" ON fcm_tokens FOR SELECT USING (auth.uid()::VARCHAR = user_id);
CREATE POLICY "Users can insert own FCM tokens" ON fcm_tokens FOR INSERT WITH CHECK (auth.uid()::VARCHAR = user_id);
CREATE POLICY "Users can update own FCM tokens" ON fcm_tokens FOR UPDATE USING (auth.uid()::VARCHAR = user_id);
CREATE POLICY "Users can delete own FCM tokens" ON fcm_tokens FOR DELETE USING (auth.uid()::VARCHAR = user_id);

-- Function to automatically update updated_at timestamp for FCM tokens
CREATE OR REPLACE FUNCTION update_fcm_tokens_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to automatically update updated_at for FCM tokens
CREATE TRIGGER trigger_fcm_tokens_updated_at
    BEFORE UPDATE ON fcm_tokens
    FOR EACH ROW
    EXECUTE FUNCTION update_fcm_tokens_updated_at();

-- Function to clean up inactive FCM tokens (for maintenance)
-- Note: Multiple users can have the same token (shared device scenario)
-- This function only removes inactive tokens that haven't been used for specified days
CREATE OR REPLACE FUNCTION cleanup_inactive_fcm_tokens(days_inactive INTEGER DEFAULT 30)
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM fcm_tokens
    WHERE is_active = false
    AND updated_at < NOW() - INTERVAL '1 day' * days_inactive;

    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;