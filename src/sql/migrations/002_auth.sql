CREATE TABLE IF NOT EXISTS USERS (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(100) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role ENUM('admin', 'user') NOT NULL,
    oauth_provider VARCHAR(32) NULL,
    oauth_subject VARCHAR(255) NULL,
    email VARCHAR(255) NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_users_oauth (oauth_provider, oauth_subject)
);

ALTER TABLE USERS
    ADD COLUMN oauth_provider VARCHAR(32) NULL,
    ADD COLUMN oauth_subject VARCHAR(255) NULL,
    ADD COLUMN email VARCHAR(255) NULL,
    ADD UNIQUE KEY uq_users_oauth (oauth_provider, oauth_subject);

INSERT IGNORE INTO USERS (username, password_hash, role) VALUES
    ('admin', '$2a$10$/tg7OSAZb/iqYCbILVu6/u1NRaSzLNNqBrS2RdBFkh0aVa7ff/BPK', 'admin'),
    ('user', '$2a$10$sUyXoJYGznJf4WAGkpmV9O.GwX1S61gt1SbMtquPZSJooV98KdU1K', 'user');
