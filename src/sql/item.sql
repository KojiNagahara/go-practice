CREATE TABLE ITEMS (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    item_description TEXT,
    price_minor_units BIGINT UNSIGNED NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE ITEM_IMAGES (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    item_id BIGINT UNSIGNED NOT NULL,
    object_key VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (item_id) REFERENCES ITEMS(id) ON DELETE CASCADE
);

CREATE TABLE CATEGORIES (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_categories_name (name)
);

CREATE TABLE ITEM_CATEGORIES (
    item_id BIGINT UNSIGNED NOT NULL,
    category_id BIGINT UNSIGNED NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (item_id, category_id),
    FOREIGN KEY (item_id) REFERENCES ITEMS(id) ON DELETE CASCADE,
    FOREIGN KEY (category_id) REFERENCES CATEGORIES(id) ON DELETE CASCADE
);

CREATE TABLE USERS (
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

INSERT INTO USERS (username, password_hash, role) VALUES
    ('admin', '$2a$10$/tg7OSAZb/iqYCbILVu6/u1NRaSzLNNqBrS2RdBFkh0aVa7ff/BPK', 'admin'),
    ('user', '$2a$10$sUyXoJYGznJf4WAGkpmV9O.GwX1S61gt1SbMtquPZSJooV98KdU1K', 'user');

-- Test data (MySQL 8.0+)
INSERT INTO ITEMS (id, name, item_description, price_minor_units)
WITH RECURSIVE item_numbers AS (
    SELECT 1 AS item_id
    UNION ALL
    SELECT item_id + 1
    FROM item_numbers
    WHERE item_id < 50
)
SELECT
    item_id,
    CONCAT('Test Item ', item_id),
    CONCAT('Description for test item ', item_id),
    100000 + (item_id * 25000)
FROM item_numbers;

INSERT INTO CATEGORIES (id, name)
WITH RECURSIVE category_numbers AS (
    SELECT 1 AS category_id
    UNION ALL
    SELECT category_id + 1
    FROM category_numbers
    WHERE category_id < 10
)
SELECT
    category_id,
    CONCAT('Category ', category_id)
FROM category_numbers;

INSERT INTO ITEM_CATEGORIES (item_id, category_id)
WITH RECURSIVE item_numbers AS (
    SELECT 1 AS item_id
    UNION ALL
    SELECT item_id + 1
    FROM item_numbers
    WHERE item_id < 50
)
SELECT
    item_id,
    1 + MOD(item_id - 1, 10)
FROM item_numbers;

INSERT INTO ITEM_IMAGES (item_id, object_key)
WITH RECURSIVE item_numbers AS (
    SELECT 1 AS item_id
    UNION ALL
    SELECT item_id + 1
    FROM item_numbers
    WHERE item_id < 50
),
image_numbers AS (
    SELECT 1 AS image_number
    UNION ALL
    SELECT image_number + 1
    FROM image_numbers
    WHERE image_number < 4
)
SELECT
    item_numbers.item_id,
    CONCAT(
        'https://www.kjnghr.com/images/items/',
        item_numbers.item_id,
        '/image-',
        image_numbers.image_number,
        '.jpg'
    )
FROM item_numbers
JOIN image_numbers
    ON image_numbers.image_number <= 2 + MOD(item_numbers.item_id, 3);