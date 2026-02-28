-- +goose Up
CREATE TABLE users(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now(),
    email TEXT NOT NULL 
);

-- +goose Down
DROP TABLE users;