-- SUREBETSLIPS PRODUCTION SCHEMA — NOV 04, 2025
-- Run once:  psql -U postgres -d surebetslips -f schema.sql

DROP SCHEMA IF EXISTS public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO postgres;

-- 1. USERS
CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    username      VARCHAR(50)  UNIQUE NOT NULL,
    email         VARCHAR(100) UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role          VARCHAR(20)  DEFAULT 'user' CHECK (role IN ('user','punter','admin','ai_punter')),
    bio           TEXT,
    avatar_url    VARCHAR(255),
    total_wins    INT DEFAULT 0,
    total_losses  INT DEFAULT 0,
    win_rate      DECIMAL(5,2) GENERATED ALWAYS AS (
        CASE WHEN total_wins + total_losses = 0 THEN 0
             ELSE ROUND(100.0 * total_wins / (total_wins + total_losses), 2)
        END
    ) STORED,
    is_verified   BOOLEAN DEFAULT FALSE,
    created_at    TIMESTAMP DEFAULT NOW(),
    updated_at    TIMESTAMP DEFAULT NOW()
);
CREATE INDEX idx_users_role ON users(role);
CREATE INDEX idx_users_winrate ON users(win_rate DESC);

-- 2. BETSLIPS
CREATE TABLE betslips (
    id            BIGSERIAL PRIMARY KEY,
    user_id       BIGINT REFERENCES users(id) ON DELETE CASCADE,
    platform      VARCHAR(20) NOT NULL,
    games         JSONB NOT NULL,                 -- [{home,away,market,odds,prediction}]
    total_odds    DECIMAL(10,2),
    stake_amount  DECIMAL(10,2),
    status        VARCHAR(10) DEFAULT 'pending' CHECK (status IN ('pending','won','lost','void')),
    visibility    VARCHAR(15) DEFAULT 'public'  CHECK (visibility IN ('public','subscribers','private')),
    is_premium    BOOLEAN DEFAULT FALSE,
    created_at    TIMESTAMP DEFAULT NOW(),
    resolved_at   TIMESTAMP
);
CREATE INDEX idx_betslips_user ON betslips(user_id);
CREATE INDEX idx_betslips_status ON betslips(status);

-- 3. SUBSCRIPTIONS (punter followers)
CREATE TABLE subscriptions (
    id            BIGSERIAL PRIMARY KEY,
    subscriber_id BIGINT REFERENCES users(id),
    punter_id     BIGINT REFERENCES users(id),
    amount        DECIMAL(10,2) NOT NULL,
    sbs_commission DECIMAL(10,2) NOT NULL,
    status        VARCHAR(10) DEFAULT 'active',
    start_date    TIMESTAMP DEFAULT NOW(),
    end_date      TIMESTAMP,
    UNIQUE(subscriber_id, punter_id)
);

-- 4. PLATFORM SUBSCRIPTIONS
CREATE TABLE platform_subscriptions (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT REFERENCES users(id),
    plan       VARCHAR(10) DEFAULT 'free' CHECK (plan IN ('free','premium')),
    amount     DECIMAL(10,2),
    status     VARCHAR(10) DEFAULT 'active',
    start_date TIMESTAMP DEFAULT NOW(),
    end_date   TIMESTAMP
);

-- 5. TRANSACTIONS
CREATE TABLE transactions (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    type                VARCHAR(30) NOT NULL,
    amount              DECIMAL(10,2) NOT NULL,
    status              VARCHAR(10) DEFAULT 'completed',
    payment_gateway_ref VARCHAR(255),
    created_at          TIMESTAMP DEFAULT NOW()
);

-- 6. COMMENTS
CREATE TABLE comments (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT REFERENCES users(id),
    betslip_id BIGINT REFERENCES betslips(id) ON DELETE CASCADE,
    content    TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

-- 7. LEADERBOARDS (daily snapshot)
CREATE TABLE leaderboards (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT REFERENCES users(id),
    period     VARCHAR(10) NOT NULL CHECK (period IN ('daily','weekly','monthly')),
    wins       INT NOT NULL,
    losses     INT NOT NULL,
    rank       INT,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Create a unique index on the expression to enforce the constraint
CREATE UNIQUE INDEX leaderboards_unique_daily_entry ON leaderboards (user_id, period, (created_at::date));

-- 8. GAME RESULTS (auto-filled by your AI)
CREATE TABLE game_results (
    id         BIGSERIAL PRIMARY KEY,
    game_id    VARCHAR(100) UNIQUE NOT NULL,   -- e.g., "EPL-20251104-MANU-LIV"
    home_team  VARCHAR(100),
    away_team  VARCHAR(100),
    score      VARCHAR(20),
    winner     VARCHAR(50),
    date       DATE,
    league     VARCHAR(100),
    created_at TIMESTAMP DEFAULT NOW()
);

-- 9. BETSLIP GAMES (links slip legs to real results)
CREATE TABLE betslip_games (
    id           BIGSERIAL PRIMARY KEY,
    betslip_id   BIGINT REFERENCES betslips(id) ON DELETE CASCADE,
    game_id      VARCHAR(100) REFERENCES game_results(game_id),
    prediction   VARCHAR(50),      -- "Home Win", "Over 2.5"
    odds         DECIMAL(6,2),
    result       VARCHAR(10) DEFAULT 'pending'   -- won / lost / pending
);

-- SEED DATA
INSERT INTO users (username, email, password_hash, role)
VALUES ('Tony', 'tony@sbs.com', '$2a$10$demo', 'punter')
ON CONFLICT (username) DO NOTHING;

-- DONE
SELECT 'SUREBETSLIPS DATABASE READY!' AS status;