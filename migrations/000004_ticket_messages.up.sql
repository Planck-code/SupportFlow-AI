CREATE TABLE ticket_messages (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    ticket_id BIGINT UNSIGNED NOT NULL,
    author_id BIGINT UNSIGNED NOT NULL,
    content TEXT NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    FOREIGN KEY (ticket_id) REFERENCES tickets(id),
    FOREIGN KEY (author_id) REFERENCES users(id),
    CHECK (CHAR_LENGTH(content) BETWEEN 1 AND 5000),
    INDEX idx_messages_ticket (ticket_id, id)
) ENGINE=InnoDB DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
