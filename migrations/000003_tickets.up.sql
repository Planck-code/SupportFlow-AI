CREATE TABLE tickets (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    requester_id BIGINT UNSIGNED NOT NULL,
    assignee_id BIGINT UNSIGNED NULL,
    title VARCHAR(120) NOT NULL,
    description TEXT NOT NULL,
    category VARCHAR(16) COLLATE utf8mb4_0900_bin NOT NULL DEFAULT 'general',
    priority VARCHAR(16) COLLATE utf8mb4_0900_bin NOT NULL DEFAULT 'normal',
    status VARCHAR(16) COLLATE utf8mb4_0900_bin NOT NULL DEFAULT 'open',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    FOREIGN KEY (requester_id) REFERENCES users(id),
    FOREIGN KEY (assignee_id) REFERENCES users(id),
    CHECK (category IN ('general','account','billing','technical')),
    CHECK (priority IN ('normal','high')),
    CHECK (status IN ('open','in_progress','resolved','closed')),
    CHECK ((status = 'open') = (assignee_id IS NULL)),
    INDEX idx_ticket_requester (requester_id, created_at, id),
    INDEX idx_ticket_assignee (assignee_id, status, created_at, id),
    INDEX idx_ticket_queue (status, assignee_id, created_at, id)
) ENGINE=InnoDB DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;

CREATE TABLE ticket_events (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    ticket_id BIGINT UNSIGNED NOT NULL,
    actor_id BIGINT UNSIGNED NOT NULL,
    event_type VARCHAR(32) COLLATE utf8mb4_0900_bin NOT NULL,
    metadata JSON NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    FOREIGN KEY (ticket_id) REFERENCES tickets(id),
    FOREIGN KEY (actor_id) REFERENCES users(id),
    CHECK (event_type IN ('created','message_added','claimed','assigned','reassigned','priority_changed','resolved','reopened','closed')),
    INDEX idx_events_ticket (ticket_id, id)
) ENGINE=InnoDB DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
