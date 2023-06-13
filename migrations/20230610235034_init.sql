-- +goose Up
-- +goose StatementBegin
CREATE TABLE contract (
    id INT AUTO_INCREMENT PRIMARY KEY,
    guid VARCHAR(50),
    type TEXT,
    date DATETIME,
    number VARCHAR(50),
    contract TEXT,
    lessor TEXT,
    lessee TEXT,
    ogrn VARCHAR(50),
    inn VARCHAR(50),
    stop_reason TEXT,
    user_comment TEXT,
    list_item_raw JSON,
    item_raw JSON,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    enriched BOOLEAN DEFAULT FALSE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS contract;
-- +goose StatementEnd