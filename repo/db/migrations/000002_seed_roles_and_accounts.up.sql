-- Seed roles and development accounts.
-- Passwords are hashed using pgcrypto crypt()/gen_salt('bf') which produces
-- bcrypt $2a$ hashes compatible with Go's golang.org/x/crypto/bcrypt.
-- This migration is idempotent via ON CONFLICT DO NOTHING.

-- Roles
INSERT INTO roles (id, name, description) VALUES
  ('10000000-0000-0000-0000-000000000001', 'administrator', 'Full system access'),
  ('10000000-0000-0000-0000-000000000002', 'provider',      'Service provider access'),
  ('10000000-0000-0000-0000-000000000003', 'customer',       'Customer access')
ON CONFLICT (name) DO NOTHING;

-- Dev users (passwords: admin123, provider123, customer123)
INSERT INTO users (id, username, password_hash, email) VALUES
  ('20000000-0000-0000-0000-000000000001', 'admin',    crypt('admin123',    gen_salt('bf', 10)), 'admin@fieldserve.local'),
  ('20000000-0000-0000-0000-000000000002', 'provider', crypt('provider123', gen_salt('bf', 10)), 'provider@fieldserve.local'),
  ('20000000-0000-0000-0000-000000000003', 'customer', crypt('customer123', gen_salt('bf', 10)), 'customer@fieldserve.local')
ON CONFLICT (username) DO NOTHING;

-- Role assignments
INSERT INTO user_roles (user_id, role_id) VALUES
  ('20000000-0000-0000-0000-000000000001', '10000000-0000-0000-0000-000000000001'),
  ('20000000-0000-0000-0000-000000000002', '10000000-0000-0000-0000-000000000002'),
  ('20000000-0000-0000-0000-000000000003', '10000000-0000-0000-0000-000000000003')
ON CONFLICT DO NOTHING;

-- Profiles
INSERT INTO admin_profiles (id, user_id, display_name) VALUES
  ('30000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000001', 'Admin')
ON CONFLICT DO NOTHING;

INSERT INTO provider_profiles (id, user_id, business_name) VALUES
  ('30000000-0000-0000-0000-000000000002', '20000000-0000-0000-0000-000000000002', 'Demo Provider')
ON CONFLICT DO NOTHING;

INSERT INTO customer_profiles (id, user_id, display_name) VALUES
  ('30000000-0000-0000-0000-000000000003', '20000000-0000-0000-0000-000000000003', 'Demo Customer')
ON CONFLICT DO NOTHING;
