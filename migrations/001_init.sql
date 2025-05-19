CREATE TABLE request_logs (
    id SERIAL PRIMARY KEY,
    ip_address VARCHAR(50) NOT NULL,
    path VARCHAR(255) NOT NULL,
    status_code INT NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);