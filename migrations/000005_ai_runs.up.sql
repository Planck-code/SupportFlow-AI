CREATE TABLE ai_runs (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    ticket_id BIGINT UNSIGNED NOT NULL,
    requested_by BIGINT UNSIGNED NOT NULL,
    status ENUM('running','succeeded','failed') NOT NULL,
    provider VARCHAR(64) NOT NULL,
    model VARCHAR(255) NOT NULL,
    input_snapshot_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    input_truncated BOOLEAN NOT NULL,
    result JSON NULL,
    error_category ENUM('timeout','canceled','network','upstream','invalid_response','invalid_output','unavailable') NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    finished_at DATETIME(6) NULL,
    FOREIGN KEY (ticket_id) REFERENCES tickets(id),
    FOREIGN KEY (requested_by) REFERENCES users(id),
    INDEX idx_ai_runs_ticket (ticket_id, id),
    CHECK ((status = 'running' AND finished_at IS NULL AND result IS NULL AND error_category IS NULL)
        OR (status = 'succeeded' AND finished_at IS NOT NULL AND result IS NOT NULL AND JSON_TYPE(result) = 'OBJECT' AND error_category IS NULL)
        OR (status = 'failed' AND finished_at IS NOT NULL AND result IS NULL AND error_category IS NOT NULL))
) ENGINE=InnoDB DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
