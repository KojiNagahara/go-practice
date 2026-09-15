-- Run once against databases created with the pre-DDD schema.
-- Existing prices are decimal major units and are converted to minor units.
ALTER TABLE ITEMS ADD COLUMN price_minor_units BIGINT UNSIGNED NULL;
UPDATE ITEMS SET price_minor_units = ROUND(price * 100);
ALTER TABLE ITEMS DROP COLUMN price;
ALTER TABLE ITEMS MODIFY price_minor_units BIGINT UNSIGNED NOT NULL;

ALTER TABLE ITEM_IMAGES ADD COLUMN object_key VARCHAR(255) NULL;
UPDATE ITEM_IMAGES SET object_key = image_url;
ALTER TABLE ITEM_IMAGES DROP COLUMN image_url;
ALTER TABLE ITEM_IMAGES MODIFY object_key VARCHAR(255) NOT NULL;
